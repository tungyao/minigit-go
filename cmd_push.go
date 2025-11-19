package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// CmdPush 推送命令
func CmdPush(args []string) {
	manager := &LocalManager{}
	if !manager.IsInit() {
		log.Println("未初始化仓库，请先执行 init")
		return
	}

	// 默认推送到 origin
	remoteName := "origin"
	if len(args) > 0 {
		remoteName = args[0]
	}

	// 读取配置
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	remoteUrl, exists := conf.Remote[remoteName]
	if !exists {
		log.Printf("远程仓库 '%s' 不存在", remoteName)
		return
	}

	// 获取密码
	password := ""
	if conf.Password != nil {
		password = conf.Password[remoteName]
	}
	// 如果配置中没有密码，提示用户输入
	if password == "" {
		password = readPassword("请输入远程仓库密码: ")
	}

	// 从远程URL解析项目名称
	projectName := parseProjectFromURL(remoteUrl)
	log.Printf("项目名称: %s", projectName)
	// 获取本地所有commits
	localHeads, err := getLocalHeads(manager)
	if err != nil {
		log.Printf("获取本地commits失败: %v", err)
		return
	}

	if len(localHeads) == 0 {
		log.Println("没有可推送的commit")
		return
	}

	// 获取远程已有的heads
	remoteHeads, remoteLatest, err := fetchRemoteHeads(remoteUrl, password, projectName)
	if err != nil {
		log.Printf("获取远程heads失败: %v", err)
		return
	}
	// 如果远程不为空，校验本地HEAD是否与远程最新一致
	if len(remoteHeads) > 0 {
		localHead, _ := manager.GetHeadContent()
		if localHead != remoteLatest {
			log.Printf("推送被拒绝：本地HEAD(%s)不是远程最新(%s)，请先pull或reset到最新再推送", localHead, remoteLatest)
			return
		}
	}

	// 找出需要推送的heads
	remoteHeadsMap := make(map[string]bool)
	for _, h := range remoteHeads {
		remoteHeadsMap[h] = true
	}

	newHeads := make([]string, 0)
	for _, h := range localHeads {
		if !remoteHeadsMap[h] {
			newHeads = append(newHeads, h)
		}
	}

	if len(newHeads) == 0 {
		log.Println("所有commit已存在于远程，无需推送")
		return
	}

	log.Printf("准备推送 %d 个新commit", len(newHeads))

	// 创建tar包
	tarData, err := createLocalTar(manager, newHeads)
	if err != nil {
		log.Printf("创建tar包失败: %v", err)
		return
	}

	// 加密数据
	encryptedData, err := encryptData(tarData, password)
	if err != nil {
		log.Printf("加密失败: %v", err)
		return
	}

	// 构建推送请求
	req := PushRequest{
		Project: projectName,
		Heads:   newHeads,
		Data:    encryptedData,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		log.Printf("构建请求失败: %v", err)
		return
	}

	// 发送推送请求
	httpReq, err := http.NewRequest("POST", remoteUrl+"/push", bytes.NewReader(reqData))
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Password", password) // 使用配置的密码

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("推送失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("推送失败: %s", string(body))
		return
	}

	log.Printf("成功推送 %d 个heads到 %s", len(newHeads), remoteName)
}

// getLocalHeads 获取本地所有commit hashes
func getLocalHeads(manager *LocalManager) ([]string, error) {
	root, err := manager.GetRoot()
	if err != nil {
		return nil, err
	}

	objectsDir := filepath.Join(root, Mark, "objects")
	heads := make([]string, 0)

	// 遍历objects目录
	err = filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// 构建完整hash
		relPath, _ := filepath.Rel(objectsDir, path)
		hash := filepath.Dir(relPath) + filepath.Base(relPath)
		heads = append(heads, hash)

		return nil
	})

	return heads, err
}

// fetchRemoteHeads 获取远程仓库的heads
func fetchRemoteHeads(remoteUrl, password, projectName string) ([]string, string, error) {
	req, err := http.NewRequest("GET", remoteUrl+"/heads?project="+projectName, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("X-Password", password)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 如果远程项目不存在，返回404，则视为无任何heads，后续push将创建项目
		if resp.StatusCode == http.StatusNotFound {
			return []string{}, "", nil
		}
		return nil, "", fmt.Errorf("获取远程heads失败: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// 兼容两种响应格式：数组 或 对象{heads, latest}
	var obj struct {
		Heads  []string `json:"heads"`
		Latest string   `json:"latest"`
	}
	if err := json.Unmarshal(body, &obj); err == nil {
		return obj.Heads, obj.Latest, nil
	}

	var heads []string
	if err := json.Unmarshal(body, &heads); err != nil {
		return nil, "", err
	}
	latest := ""
	if len(heads) > 0 {
		latest = heads[len(heads)-1]
	}
	return heads, latest, nil
}

// createLocalTar 创建本地tar包
func createLocalTar(manager *LocalManager, hashes []string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for _, hash := range hashes {
		// 读取对象数据
		data, err := manager.GetBlob(hash)
		if err != nil {
			log.Printf("读取对象 %s 失败: %v", hash, err)
			continue
		}

		// 添加到tar包
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
