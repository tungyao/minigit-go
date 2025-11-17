package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkTree 工作区管理器
type WorkTree struct {
	manager *LocalManager
}

// NewWorkTree 创建工作区管理器
func NewWorkTree(manager *LocalManager) *WorkTree {
	return &WorkTree{manager: manager}
}

// GetIndexMap 将index中的Tree数组转换为map，使用Path作为key，快速查找
func (w *WorkTree) GetIndexMap() (map[string]Tree, error) {
	indexContent, err := w.manager.GetIndexContent()
	if err != nil {
		return nil, err
	}

	if indexContent == "" {
		return make(map[string]Tree), nil
	}

	var trees []Tree
	err = json.Unmarshal([]byte(indexContent), &trees)
	if err != nil {
		return nil, err
	}

	treeMap := make(map[string]Tree)
	for _, tree := range trees {
		treeMap[tree.Path] = tree
	}

	return treeMap, nil
}

// ScanFiles 扫描指定的文件/目录，返回所有文件的Tree列表
// 优化策略:只扫描index中已存在的目录,避免全量遍历
func (w *WorkTree) ScanFiles(paths []string) ([]Tree, error) {
	indexMap, err := w.GetIndexMap()
	if err != nil {
		log.Printf("Error getting index map: %v", err)
		return nil, err
	}

	result := make([]Tree, 0)
	processed := make(map[string]bool) // 防止重复处理

	for _, path := range paths {
		// 获取相对路径
		relPath, err := GetRelativePath(path)
		if err != nil {
			log.Printf("Error getting relative path for %s: %v", path, err)
			continue
		}

		// 判断是文件还是目录
		pathType := PathType(relPath)

		switch pathType {
		case "file":
			if !processed[relPath] {
				tree, err := w.processFile(relPath, indexMap)
				if err != nil {
					log.Printf("Error processing file %s: %v", relPath, err)
					continue
				}
				if tree != nil {
					result = append(result, *tree)
					processed[relPath] = true
				}
			}
		case "dir":
			// 检查index中是否有该目录下的文件
			hasFilesInIndex := w.hasFilesInDirectory(relPath, indexMap)
			if hasFilesInIndex {
				// 扫描目录中所有文件(包括已跟踪和新文件)
				trees := w.scanDirectoryFull(relPath, indexMap, processed)
				result = append(result, trees...)
			} else {
				// 目录不在index中,添加整个目录的所有文件
				log.Printf("Adding new directory %s", relPath)
				trees := w.scanNewDirectory(relPath, indexMap, processed)
				result = append(result, trees...)
			}
		case "":
			// 路径不存在,可能是被删除的文件,检查index中是否存在
			if oldTree, exists := indexMap[relPath]; exists {
				oldTree.Status = StatusDelete
				result = append(result, oldTree)
				processed[relPath] = true
			} else {
				log.Printf("Path %s does not exist and not in index", relPath)
			}
		default:
			log.Printf("Path %s is of unknown type", relPath)
		}
	}

	return result, nil
}

// processFile 处理单个文件，计算其状态并保存文件内容
func (w *WorkTree) processFile(filePath string, indexMap map[string]Tree) (*Tree, error) {
	// 计算文件hash
	hash, err := GetFileHash(filePath)
	if err != nil {
		return nil, err
	}

	// 创建 Tree 对象
	tree := &Tree{
		Hash:      hash,
		Path:      filePath,
		Name:      filepath.Base(filePath),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// 判断状态
	if oldTree, exists := indexMap[filePath]; exists {
		// 文件在index中存在
		if oldTree.Hash != hash {
			// Hash不同，说明文件被修改
			tree.Status = StatusModify
			// 保存新的文件内容
			if err := w.saveFileBlob(filePath, hash); err != nil {
				log.Printf("Error saving blob for %s: %v", filePath, err)
			}
		} else {
			// Hash相同，文件未变化，不需要返回
			return nil, nil
		}
	} else {
		// 文件不在index中，是新增文件
		tree.Status = StatusAdd
		// 保存文件内容
		if err := w.saveFileBlob(filePath, hash); err != nil {
			log.Printf("Error saving blob for %s: %v", filePath, err)
		}
	}

	return tree, nil
}

// saveFileBlob 保存文件内容到objects目录
func (w *WorkTree) saveFileBlob(filePath string, hash string) error {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// 保存blob对象
	return w.manager.SaveBlob(hash, content)
}

// hasFilesInDirectory 检查index中是否有该目录下的文件
func (w *WorkTree) hasFilesInDirectory(dirPath string, indexMap map[string]Tree) bool {
	for path := range indexMap {
		// 检查path是否在dirPath目录下
		if filepath.Dir(path) == dirPath || path == dirPath {
			return true
		}
		// 检查是否是子目录
		relPath, err := filepath.Rel(dirPath, path)
		if err == nil && !filepath.IsAbs(relPath) && !strings.HasPrefix(relPath, "..") {
			return true
		}
	}
	return false
}

// scanTrackedFilesInDirectory 扫描目录中已跟踪的文件的状态(修改/删除)
func (w *WorkTree) scanTrackedFilesInDirectory(dirPath string, indexMap map[string]Tree, processed map[string]bool) []Tree {
	result := make([]Tree, 0)

	// 只检查index中在该目录下的文件
	for path, oldTree := range indexMap {
		// 检查文件是否在指定目录下
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "..") {
			continue
		}

		if processed[path] {
			continue
		}

		// 检查文件是否还存在
		pathType := PathType(path)
		if pathType == "" {
			// 文件已被删除
			oldTree.Status = StatusDelete
			result = append(result, oldTree)
			processed[path] = true
			continue
		}

		if pathType != "file" {
			continue
		}

		// 检查文件是否被修改
		hash, err := GetFileHash(path)
		if err != nil {
			log.Printf("Error getting hash for %s: %v", path, err)
			continue
		}

		if hash != oldTree.Hash {
			// 文件被修改
			tree := Tree{
				Hash:      hash,
				Path:      path,
				Name:      filepath.Base(path),
				Timestamp: time.Now().Format(time.RFC3339),
				Status:    StatusModify,
			}
			result = append(result, tree)
			processed[path] = true
		}
	}

	return result
}

// scanDirectoryFull 完整扫描目录,处理所有文件(已跟踪和新文件)
func (w *WorkTree) scanDirectoryFull(dirPath string, indexMap map[string]Tree, processed map[string]bool) []Tree {
	result := make([]Tree, 0)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// 跳过.minigit目录
		if info.IsDir() && info.Name() == Mark {
			return filepath.SkipDir
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 获取相对路径
		relPath, err := GetRelativePath(path)
		if err != nil {
			return nil
		}

		// 如果已经处理过,跳过
		if processed[relPath] {
			return nil
		}

		// 处理文件
		tree, err := w.processFile(relPath, indexMap)
		if err != nil {
			log.Printf("Error processing file %s: %v", relPath, err)
			return nil
		}

		if tree != nil {
			result = append(result, *tree)
			processed[relPath] = true
		}

		return nil
	})

	if err != nil {
		log.Printf("Error walking directory %s: %v", dirPath, err)
	}

	return result
}

// scanNewDirectory 扫描新目录,添加所有文件
func (w *WorkTree) scanNewDirectory(dirPath string, indexMap map[string]Tree, processed map[string]bool) []Tree {
	result := make([]Tree, 0)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误,继续
		}

		// 跳过.minigit目录
		if info.IsDir() && info.Name() == Mark {
			return filepath.SkipDir
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 获取相对路径
		relPath, err := GetRelativePath(path)
		if err != nil {
			return nil
		}

		// 如果已经处理过,跳过
		if processed[relPath] {
			return nil
		}

		// 处理文件
		tree, err := w.processFile(relPath, indexMap)
		if err != nil {
			log.Printf("Error processing file %s: %v", relPath, err)
			return nil
		}

		if tree != nil {
			result = append(result, *tree)
			processed[relPath] = true
		}

		return nil
	})

	if err != nil {
		log.Printf("Error walking directory %s: %v", dirPath, err)
	}

	return result
}

// GetStatus 获取当前工作区状态(智能扫描,避免深度遍历)
// 优化策略:
// 1. 检查index中已跟踪的文件状态(删除/修改)
// 2. 浅层扫描工作区根目录,查找新文件
// 3. 对于子目录,只有在index中存在该目录下的文件时才深度遍历
func (w *WorkTree) GetStatus() ([]Tree, error) {
	indexMap, err := w.GetIndexMap()
	if err != nil {
		return nil, err
	}

	result := make([]Tree, 0)
	processed := make(map[string]bool)

	// 1. 遍历index中的文件,检查是否被删除或修改
	for path, oldTree := range indexMap {
		processed[path] = true
		// 检查文件是否还存在
		pathType := PathType(path)
		if pathType == "" {
			// 文件已被删除
			oldTree.Status = StatusDelete
			result = append(result, oldTree)
			continue
		}

		if pathType != "file" {
			continue
		}

		// 文件存在,检查是否被修改
		hash, err := GetFileHash(path)
		if err != nil {
			log.Printf("Error getting hash for %s: %v", path, err)
			continue
		}

		if hash != oldTree.Hash {
			// 文件被修改
			tree := Tree{
				Hash:      hash,
				Path:      path,
				Name:      filepath.Base(path),
				Timestamp: time.Now().Format(time.RFC3339),
				Status:    StatusModify,
			}
			result = append(result, tree)
		}
	}

	// 2. 智能扫描工作区,避免深度遍历未跟踪的目录
	newFiles, err := w.smartScanWorkspace(indexMap, processed)
	if err != nil {
		log.Printf("Error scanning workspace: %v", err)
	} else {
		result = append(result, newFiles...)
	}

	return result, nil
}

// smartScanWorkspace 智能扫描工作区,避免深度遍历
// 策略:只浅层扫描根目录,对于子目录只有在index中存在时才深度遍历
// 返回: newFiles - 新文件列表, untrackedDirs - 未跟踪的目录列表
func (w *WorkTree) smartScanWorkspace(indexMap map[string]Tree, processed map[string]bool) ([]Tree, error) {
	root, err := w.manager.GetRoot()
	if err != nil {
		return nil, err
	}

	result := make([]Tree, 0)

	// 读取根目录下的文件和文件夹
	entries, err := filepath.Glob(filepath.Join(root, "*"))
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		relPath, err := GetRelativePath(entry)
		if err != nil {
			continue
		}

		// 跳过.minigit目录
		if filepath.Base(relPath) == Mark {
			continue
		}

		pathType := PathType(relPath)
		if pathType == "file" {
			// 根目录下的文件,检查是否已处理
			if processed[relPath] {
				continue
			}

			// 这是一个新文件
			hash, err := GetFileHash(relPath)
			if err != nil {
				log.Printf("Error getting hash for %s: %v", relPath, err)
				continue
			}

			tree := Tree{
				Hash:      hash,
				Path:      relPath,
				Name:      filepath.Base(relPath),
				Timestamp: time.Now().Format(time.RFC3339),
				Status:    StatusAdd,
			}
			result = append(result, tree)
			processed[relPath] = true

		} else if pathType == "dir" {
			// 目录:只有在index中存在该目录下的文件时才深度遍历
			hasTrackedFiles := w.hasFilesInDirectory(relPath, indexMap)
			if hasTrackedFiles {
				// 该目录在历史记录中存在,进行深度遍历
				newFilesInDir := w.deepScanDirectory(relPath, indexMap, processed)
				result = append(result, newFilesInDir...)
			} else {
				// 未跟踪的目录,标记为新增目录
				tree := Tree{
					Hash:      "",
					Path:      relPath + string(filepath.Separator),
					Name:      filepath.Base(relPath) + string(filepath.Separator),
					Timestamp: time.Now().Format(time.RFC3339),
					Status:    StatusAdd, // 未跟踪目录也标记为新增
				}
				result = append(result, tree)
			}
		}
	}

	return result, nil
}

// deepScanDirectory 深度扫描目录,查找新文件
func (w *WorkTree) deepScanDirectory(dirPath string, indexMap map[string]Tree, processed map[string]bool) []Tree {
	result := make([]Tree, 0)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误,继续
		}

		// 跳过.minigit目录
		if info.IsDir() && info.Name() == Mark {
			return filepath.SkipDir
		}

		// 跳过目录本身
		if info.IsDir() {
			return nil
		}

		// 获取相对路径
		relPath, err := GetRelativePath(path)
		if err != nil {
			return nil
		}

		// 如果已经处理过,跳过
		if processed[relPath] {
			return nil
		}

		// 这是一个新文件
		hash, err := GetFileHash(relPath)
		if err != nil {
			log.Printf("Error getting hash for %s: %v", relPath, err)
			return nil
		}

		tree := Tree{
			Hash:      hash,
			Path:      relPath,
			Name:      info.Name(),
			Timestamp: time.Now().Format(time.RFC3339),
			Status:    StatusAdd,
		}
		result = append(result, tree)
		processed[relPath] = true

		return nil
	})

	if err != nil {
		log.Printf("Error walking directory %s: %v", dirPath, err)
	}

	return result
}

// MergeIndexWithTrees 将新的trees合并到index中
func (w *WorkTree) MergeIndexWithTrees(newTrees []Tree) ([]Tree, error) {
	indexMap, err := w.GetIndexMap()
	if err != nil {
		return nil, err
	}

	// 合并新的trees到indexMap
	for _, tree := range newTrees {
		if tree.Status == StatusDelete {
			// 删除文件
			delete(indexMap, tree.Path)
		} else {
			// 新增或修改文件
			indexMap[tree.Path] = tree
		}
	}

	// 转换为数组
	result := make([]Tree, 0, len(indexMap))
	for _, tree := range indexMap {
		result = append(result, tree)
	}

	return result, nil
}
