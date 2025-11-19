package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// CommitLog commit日志信息（已废弃，不再使用）
type CommitLog struct {
	Hash       string   `json:"hash"`
	ParentHash string   `json:"parent_hash"`
	Message    string   `json:"message"`
	Timestamp  string   `json:"timestamp"`
	Objects    []string `json:"objects"` // 该commit关联的所有对象hash
}

// ProjectIndex 项目索引结构（已废弃，不再使用）
type ProjectIndex struct {
	Commits map[string]*CommitLog // hash -> commit log
	Heads   []string              // 所有commit hash列表
	Latest  string                // 最新的head
}

// ProjectData 项目数据结构（不再使用）
type ProjectData struct {
	Heads map[string][]byte // hash -> commit object data
}

// ServerData 服务器存储的数据结构（支持多项目）
type ServerData struct {
	Projects map[string]*ProjectData // project_name -> project_data
}

// PushRequest 推送请求
type PushRequest struct {
	Project string   // 项目名称
	Heads   []string // 要推送的commit hash列表
	Data    []byte   // 加密后的tar包数据
}

// PullRequest 拉取请求
type PullRequest struct {
	Project    string   // 项目名称
	LocalHeads []string // 本地已有的commit hash列表
}

// PullResponse 拉取响应
type PullResponse struct {
	Heads  []string // 需要拉取的commit hash列表
	Data   []byte   // 加密后的tar包数据
	Latest string   // 服务器端最新的head
}

// HeadsResponse 获取heads时的响应
type HeadsResponse struct {
	Heads  []string `json:"heads"`
	Latest string   `json:"latest"`
}

// Server 服务器实例
type Server struct {
	password string
	port     string
	dataDir  string // 数据根目录
	cipher   cipher.AEAD
}

// 创建服务器 用来方便推送和拉取
// server -port 8000 -password 123123
// 加密协议使用 aes-256 推送和拉取都只处理不存在的head 统一打包然后发送
func CmdServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	port := fs.String("port", "8000", "服务器端口")
	password := fs.String("password", "", "访问密码")
	fs.Parse(args)

	if *password == "" {
		log.Fatal("必须提供密码 -password")
	}

	// 创建服务器
	server, err := NewServer(*port, *password)
	if err != nil {
		log.Fatalf("创建服务器失败: %v", err)
	}

	log.Printf("服务器启动在端口 %s", *port)
	server.Start()
}

// NewServer 创建新的服务器实例
func NewServer(port, password string) (*Server, error) {
	// 使用密码生成AES密钥
	key := deriveKey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// 创建数据目录
	dataDir := ".minigit-server"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	server := &Server{
		password: password,
		port:     port,
		dataDir:  dataDir,
		cipher:   aesgcm,
	}

	return server, nil
}

// encrypt 加密数据
func (s *Server) encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := s.cipher.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt 解密数据
func (s *Server) decrypt(data []byte) ([]byte, error) {
	nonceSize := s.cipher.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("密文太短")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := s.cipher.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// loadData 加载服务器数据（新版本不需要，直接从文件系统读取）
func (s *Server) loadData() {
	// 不再需要，数据直接存储在文件系统中
}

// saveData 保存服务器数据（新版本不需要）
func (s *Server) saveData() error {
	// 不再需要，数据直接存储在文件系统中
	return nil
}

// getProjectDir 获取项目目录
func (s *Server) getProjectDir(project string) string {
	return filepath.Join(s.dataDir, project)
}

// getProjectObjectsDir 获取项目对象目录
func (s *Server) getProjectObjectsDir(project string) string {
	return filepath.Join(s.getProjectDir(project), "objects")
}

// getProjectHeadFile 获取项目HEAD文件路径
func (s *Server) getProjectHeadFile(project string) string {
	return filepath.Join(s.getProjectDir(project), "head")
}

// getProjectIndexFile 获取项目索引文件（已废弃）
func (s *Server) getProjectIndexFile(project string) string {
	return filepath.Join(s.getProjectDir(project), "index.json")
}

// loadProjectHead 加载项目的HEAD
func (s *Server) loadProjectHead(project string) (string, error) {
	headFile := s.getProjectHeadFile(project)
	data, err := os.ReadFile(headFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // HEAD不存在，返回空
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// saveProjectHead 保存项目的HEAD
func (s *Server) saveProjectHead(project, headHash string) error {
	// 确保项目目录存在
	projectDir := s.getProjectDir(project)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return err
	}

	headFile := s.getProjectHeadFile(project)
	return os.WriteFile(headFile, []byte(headHash), 0644)
}

// collectCommitHistory 从指定commit开始向前收集所有提交历史
func (s *Server) collectCommitHistory(project, startHash string) ([]string, error) {
	if startHash == "" {
		return []string{}, nil
	}

	history := make([]string, 0)
	visited := make(map[string]bool)
	currentHash := startHash

	for currentHash != "" && !visited[currentHash] {
		visited[currentHash] = true
		history = append(history, currentHash)

		// 加载commit对象
		commitData, err := s.loadObject(project, currentHash)
		if err != nil {
			log.Printf("无法加载commit %s: %v", currentHash, err)
			break
		}

		var headObj HeadObj
		if err := json.Unmarshal(commitData, &headObj); err != nil {
			log.Printf("解析commit %s 失败: %v", currentHash, err)
			break
		}

		// 继续追踪父提交
		currentHash = headObj.ParentHash
	}

	return history, nil
}

// getCommitObjects 获取指定commit关联的所有对象hash
func (s *Server) getCommitObjects(project, commitHash string) ([]string, error) {
	commitData, err := s.loadObject(project, commitHash)
	if err != nil {
		return nil, err
	}

	var headObj HeadObj
	if err := json.Unmarshal(commitData, &headObj); err != nil {
		return nil, err
	}

	objects := make([]string, 0)
	for _, tree := range headObj.Trees {
		objects = append(objects, tree.Hash)
	}

	return objects, nil
}

// loadProjectIndex 加载项目索引（已废弃，保留用于兼容）
func (s *Server) loadProjectIndex(project string) (*ProjectIndex, error) {
	// 尝试从旧的index.json迁移
	indexFile := s.getProjectIndexFile(project)
	if data, err := os.ReadFile(indexFile); err == nil {
		var index ProjectIndex
		if err := json.Unmarshal(data, &index); err == nil {
			// 迁移：将Latest写入head文件
			if index.Latest != "" {
				s.saveProjectHead(project, index.Latest)
				log.Printf("已迁移项目 %s 的索引到新格式", project)
			}
			// 删除旧的index.json
			os.Remove(indexFile)
			return &index, nil
		}
	}

	// 新格式：从head文件读取
	return &ProjectIndex{
		Commits: make(map[string]*CommitLog),
		Heads:   make([]string, 0),
	}, nil
}

// saveProjectIndex 保存项目索引（已废弃）
func (s *Server) saveProjectIndex(project string, index *ProjectIndex) error {
	// 不再保存index.json，只保存head
	return nil
}

// getLatestHead 计算索引中的最新head（已废弃）
func getLatestHead(index *ProjectIndex) string {
	// 已废弃，现在直接从head文件读取
	return index.Latest
}

// saveObject 保存对象到项目目录
func (s *Server) saveObject(project, hash string, data []byte) error {
	objectsDir := s.getProjectObjectsDir(project)
	// 使用hash的前2个字符作为子目录
	subDir := filepath.Join(objectsDir, hash[:2])
	if err := os.MkdirAll(subDir, 0755); err != nil {
		return err
	}

	objectFile := filepath.Join(subDir, hash[2:])
	return os.WriteFile(objectFile, data, 0644)
}

// loadObject 加载对象数据
func (s *Server) loadObject(project, hash string) ([]byte, error) {
	objectsDir := s.getProjectObjectsDir(project)
	objectFile := filepath.Join(objectsDir, hash[:2], hash[2:])
	return os.ReadFile(objectFile)
}

// Start 启动服务器
func (s *Server) Start() {
	http.HandleFunc("/push", s.handlePush)
	http.HandleFunc("/pull", s.handlePull)
	http.HandleFunc("/heads", s.handleHeads)
	// 支持形如 /{project}/push 等带项目前缀的路径
	http.HandleFunc("/", s.handleRoute)

	log.Fatal(http.ListenAndServe(":"+s.port, nil))
}

// handlePush 处理推送请求
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	// 验证密码
	password := r.Header.Get("X-Password")
	if password != s.password {
		http.Error(w, "密码错误", http.StatusUnauthorized)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}

	var req PushRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "解析请求失败", http.StatusBadRequest)
		return
	}
	// 如果请求体未包含项目名，尝试从URL路径解析
	if req.Project == "" {
		if p := extractProjectFromPath(r.URL.Path, "push"); p != "" {
			req.Project = p
		}
	}

	// 解密数据
	decryptedData, err := s.decrypt(req.Data)
	if err != nil {
		http.Error(w, "解密失败", http.StatusBadRequest)
		return
	}

	// 解压tar包
	if err := s.extractTar(req.Project, decryptedData); err != nil {
		http.Error(w, fmt.Sprintf("解压失败: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("成功接收推送，%d个commit", len(req.Heads))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("推送成功"))
}

// handlePull 处理拉取请求
func (s *Server) handlePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	// 验证密码
	password := r.Header.Get("X-Password")
	if password != s.password {
		http.Error(w, "密码错误", http.StatusUnauthorized)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}

	var req PullRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "解析请求失败", http.StatusBadRequest)
		return
	}
	// 如果请求体未包含项目名，尝试从URL路径解析
	if req.Project == "" {
		if p := extractProjectFromPath(r.URL.Path, "pull"); p != "" {
			req.Project = p
		}
	}

	// 加载项目HEAD
	latestHead, err := s.loadProjectHead(req.Project)
	if err != nil {
		http.Error(w, fmt.Sprintf("加载项目HEAD失败: %v", err), http.StatusInternalServerError)
		return
	}

	if latestHead == "" {
		// 项目为空，返回空
		resp := PullResponse{
			Heads:  []string{},
			Data:   []byte{},
			Latest: "",
		}
		jsonData, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
		return
	}

	// 从最新HEAD开始收集所有提交历史
	allCommits, err := s.collectCommitHistory(req.Project, latestHead)
	if err != nil {
		http.Error(w, fmt.Sprintf("收集提交历史失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 找出客户端没有的commits
	localHeadsMap := make(map[string]bool)
	for _, h := range req.LocalHeads {
		localHeadsMap[h] = true
	}

	newCommits := make([]string, 0)
	needObjects := make(map[string]bool) // 需要的所有对象

	for _, commitHash := range allCommits {
		if !localHeadsMap[commitHash] {
			newCommits = append(newCommits, commitHash)
			// 添加该commit及其关联的所有对象
			needObjects[commitHash] = true // commit对象本身

			// 获取commit关联的所有blob对象
			objects, err := s.getCommitObjects(req.Project, commitHash)
			if err != nil {
				log.Printf("获取commit %s 的对象失败: %v", commitHash, err)
			} else {
				for _, objHash := range objects {
					needObjects[objHash] = true
				}
			}
		}
	}

	if len(newCommits) == 0 {
		// 没有新的commit
		resp := PullResponse{
			Heads:  []string{},
			Data:   []byte{},
			Latest: latestHead,
		}
		jsonData, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
		return
	}

	// 创建tar包，包含所有需要的对象
	objectHashes := make([]string, 0, len(needObjects))
	for hash := range needObjects {
		objectHashes = append(objectHashes, hash)
	}

	tarData, err := s.createTar(req.Project, objectHashes)
	if err != nil {
		http.Error(w, fmt.Sprintf("创建tar包失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 加密数据
	encryptedData, err := s.encrypt(tarData)
	if err != nil {
		http.Error(w, "加密失败", http.StatusInternalServerError)
		return
	}

	resp := PullResponse{
		Heads:  newCommits,
		Data:   encryptedData,
		Latest: latestHead,
	}

	jsonData, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "生成响应失败", http.StatusInternalServerError)
		return
	}

	log.Printf("发送拉取响应，%d个新commit，%d个对象", len(newCommits), len(objectHashes))
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

// handleHeads 处理获取所有heads的请求
func (s *Server) handleHeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持GET方法", http.StatusMethodNotAllowed)
		return
	}

	// 验证密码
	password := r.Header.Get("X-Password")
	if password != s.password {
		http.Error(w, "密码错误", http.StatusUnauthorized)
		return
	}

	// 获取项目名
	project := r.URL.Query().Get("project")
	if project == "" {
		if p := extractProjectFromPath(r.URL.Path, "heads"); p != "" {
			project = p
		}
	}

	// 加载项目HEAD
	latestHead, err := s.loadProjectHead(project)
	if err != nil {
		http.Error(w, fmt.Sprintf("加载项目HEAD失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 从最新HEAD开始收集所有提交历史
	allCommits := []string{}
	if latestHead != "" {
		allCommits, err = s.collectCommitHistory(project, latestHead)
		if err != nil {
			log.Printf("收集提交历史失败: %v", err)
		}
	}

	resp := HeadsResponse{
		Heads:  allCommits,
		Latest: latestHead,
	}

	jsonData, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "生成响应失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

// createTar 创建tar包，包含指定的对象
func (s *Server) createTar(project string, hashes []string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for _, hash := range hashes {
		// 从文件系统读取对象
		data, err := s.loadObject(project, hash)
		if err != nil {
			log.Printf("读取对象 %s 失败: %v", hash, err)
			continue
		}

		// 添加commit对象到tar包
		header := &tar.Header{
			Name: hash,
			Mode: 0644,
			Size: int64(len(data)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, err
		}

		if _, err := tarWriter.Write(data); err != nil {
			return nil, err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, err
	}

	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// extractTar 解压tar包并保存对象
func (s *Server) extractTar(project string, data []byte) error {
	buf := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(buf)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	newCommitHashes := make([]string, 0)
	var latestHash string
	latestTimestamp := ""

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// 读取对象数据
		objectData := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, objectData); err != nil {
			return err
		}

		hash := strings.TrimSpace(header.Name)

		// 保存对象到文件系统
		if err := s.saveObject(project, hash, objectData); err != nil {
			log.Printf("保存对象 %s 失败: %v", hash, err)
			continue
		}

		// 尝试解析为commit对象
		var headObj HeadObj
		if err := json.Unmarshal(objectData, &headObj); err == nil {
			// 这是一个commit对象
			newCommitHashes = append(newCommitHashes, hash)

			// 记录最新的commit（按时间戳）
			if headObj.Timestamp > latestTimestamp {
				latestTimestamp = headObj.Timestamp
				latestHash = hash
			}
		}
	}

	// 更新HEAD为最新的commit
	if latestHash != "" {
		if err := s.saveProjectHead(project, latestHash); err != nil {
			log.Printf("更新HEAD失败: %v", err)
			return err
		}
		log.Printf("已更新项目 %s 的HEAD为 %s", project, latestHash)
	}

	log.Printf("已接收 %d 个新commit", len(newCommitHashes))
	return nil
}

// handleRoute 路由分发，支持 /{project}/push 等形式
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(path, "/push") || path == "push" {
		s.handlePush(w, r)
		return
	}
	if strings.HasSuffix(path, "/pull") || path == "pull" {
		s.handlePull(w, r)
		return
	}
	if strings.HasSuffix(path, "/heads") || path == "heads" {
		s.handleHeads(w, r)
		return
	}
	http.NotFound(w, r)
}

// extractProjectFromPath 从URL路径中解析项目名，依据末尾操作名
func extractProjectFromPath(p string, suffix string) string {
	p = strings.Trim(p, "/")
	segs := strings.Split(p, "/")
	if len(segs) >= 2 && segs[len(segs)-1] == suffix {
		return segs[len(segs)-2]
	}
	return ""
}
