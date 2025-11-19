package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CmdReset 处理reset命令
// 使用方式: minigit reset [--hard|--soft] <target>
// --soft: 只更新HEAD和index，不改变工作区（默认模式）
// --hard: 更新HEAD、index和工作区
// <target>: HEAD、HEAD~n 或 commit hash
func CmdReset(args []string) {
	manager := LocalManager{}

	// 检查是否已初始化
	if !manager.IsInit() {
		log.Println("Error: not a minigit repository. Please run 'minigit init' first.")
		return
	}

	// 解析参数
	if len(args) < 1 {
		log.Println("Usage: minigit reset [--hard|--soft] <target>")
		log.Println("  Modes:")
		log.Println("    --soft   - only update HEAD and index (default)")
		log.Println("    --hard   - update HEAD, index and working directory")
		log.Println("  <target> can be:")
		log.Println("    HEAD     - reset to latest commit")
		log.Println("    HEAD~1   - reset to previous commit")
		log.Println("    HEAD~n   - reset to n commits before")
		log.Println("    <hash>   - reset to specific commit hash")
		return
	}

	// 解析模式和目标
	mode := "soft" // 默认为soft模式
	targetIndex := 0

	if args[0] == "--hard" {
		mode = "hard"
		targetIndex = 1
	} else if args[0] == "--soft" {
		mode = "soft"
		targetIndex = 1
	} else {
		// 没有指定模式，使用默认soft模式，参数就是目标
		mode = "soft"
		targetIndex = 0
	}

	// 检查是否有目标参数
	if targetIndex >= len(args) {
		log.Println("Error: target is required")
		log.Println("Usage: minigit reset [--hard|--soft] <target>")
		return
	}

	// 解析目标提交
	target := args[targetIndex]
	var targetHash string

	// 解析 HEAD~n 格式或直接使用 hash
	if strings.HasPrefix(target, "HEAD") {
		// 获取当前HEAD
		headHash, err := manager.GetHeadContent()
		if err != nil || headHash == "" {
			log.Println("Error: no commits yet, cannot reset.")
			return
		}

		offset := 0
		if target == "HEAD" {
			offset = 0
		} else if strings.HasPrefix(target, "HEAD~") {
			// 提取数字
			numStr := strings.TrimPrefix(target, "HEAD~")
			n, err := strconv.Atoi(numStr)
			if err != nil {
				log.Printf("Error: invalid format '%s', expected HEAD~n where n is a number", target)
				return
			}
			offset = n
		} else {
			log.Printf("Error: invalid format '%s', expected HEAD or HEAD~n", target)
			return
		}

		// 从HEAD开始向前查找offset个提交
		targetHash = headHash
		currentHash := headHash

		for i := 0; i < offset; i++ {
			commit, err := loadCommitObject(&manager, currentHash)
			if err != nil {
				log.Printf("Error: cannot find commit at HEAD~%d", i+1)
				return
			}

			if commit.ParentHash == "" {
				log.Printf("Error: cannot go back %d commits, only %d commits available", offset, i)
				return
			}

			currentHash = commit.ParentHash
		}

		targetHash = currentHash
	} else {
		// 直接使用提供的hash值
		targetHash = target
		// 验证hash对应的commit是否存在
		_, err := loadCommitObject(&manager, targetHash)
		if err != nil {
			log.Printf("Error: commit '%s' not found: %v", targetHash, err)
			return
		}
	}

	// 加载目标提交
	targetCommit, err := loadCommitObject(&manager, targetHash)
	if err != nil {
		log.Printf("Error loading target commit: %v", err)
		return
	}

	log.Printf("%s mode: Resetting to commit %s", mode, targetHash)
	log.Printf("Message: %s", targetCommit.Message)
	log.Printf("Date: %s", targetCommit.Timestamp)

	// 执行reset操作
	if mode == "hard" {
		err = performHardReset(&manager, targetHash, targetCommit)
	} else {
		err = performSoftReset(&manager, targetHash, targetCommit)
	}
	if err != nil {
		log.Printf("Error performing reset: %v", err)
		return
	}

	log.Println("Reset successful!")
}

// performHardReset 执行hard reset操作
// 1. 先读取旧索引（在更新前）
// 2. 更新HEAD指向目标提交
// 3. 更新index为目标提交的trees
// 4. 恢复工作区文件到目标提交的状态
func performHardReset(manager *LocalManager, targetHash string, targetCommit *HeadObj) error {
	// 1. 先读取旧的暂存区索引（在更新前），用于删除在目标提交中不存在的文件
	workTree := NewWorkTree(manager)
	oldIndexMap, _ := workTree.GetIndexMap()
	log.Printf("Old index has %d files", len(oldIndexMap))

	// 2. 更新HEAD
	if err := manager.FlushHead(targetHash); err != nil {
		return err
	}

	// 3. 构建完整快照（遍历历史提交，收集所有未删除的文件）
	completeSnapshot, err := buildCompleteSnapshot(manager, targetHash, targetCommit)
	if err != nil {
		return err
	}
	log.Printf("Target commit complete snapshot has %d files", len(completeSnapshot))
	if err := manager.FlushIndex(completeSnapshot); err != nil {
		return err
	}

	// 4. 恢复工作区：删除旧索引中存在但目标提交不存在的文件，并写入目标提交的文件
	if err := restoreWorkingDirectoryWithDiff(manager, completeSnapshot, oldIndexMap); err != nil {
		return err
	}

	return nil
}

// performSoftReset 执行soft reset操作
// 1. 更新HEAD指向目标提交
// 2. 更新index为目标提交的trees
// 3. 不改变工作区文件
func performSoftReset(manager *LocalManager, targetHash string, targetCommit *HeadObj) error {
	// 1. 更新HEAD
	err := manager.FlushHead(targetHash)
	if err != nil {
		return err
	}

	// 2. 构建完整快照并更新index
	completeSnapshot, err := buildCompleteSnapshot(manager, targetHash, targetCommit)
	if err != nil {
		return err
	}
	err = manager.FlushIndex(completeSnapshot)
	log.Println("Working directory unchanged (soft mode)")
	return nil
}

// buildCompleteSnapshot 构建目标提交的完整快照
// 从目标提交开始，向前遍历所有父提交，收集所有未被删除的文件
// 这样可以还原出该提交时刻的完整文件状态
func buildCompleteSnapshot(manager *LocalManager, targetHash string, targetCommit *HeadObj) ([]Tree, error) {
	// 使用 map 存储文件状态，key 是文件路径
	fileMap := make(map[string]*Tree)

	// 从目标提交开始向前遍历
	currentCommit := targetCommit

	for {
		// 处理当前提交的所有 trees
		for _, tree := range currentCommit.Trees {
			// 只处理尚未记录的文件（因为我们是从新到旧遍历，新的状态优先）
			if _, exists := fileMap[tree.Path]; !exists {
				if tree.Status == StatusDelete {
					// 如果是删除操作，标记为已删除（不加入最终快照）
					fileMap[tree.Path] = nil
				} else {
					// 添加或修改操作，记录文件信息
					treeCopy := tree
					fileMap[tree.Path] = &treeCopy
				}
			}
		}

		// 如果没有父提交，遍历结束
		if currentCommit.ParentHash == "" {
			break
		}

		// 加载父提交
		parentCommit, err := loadCommitObject(manager, currentCommit.ParentHash)
		if err != nil {
			log.Printf("Warning: failed to load parent commit %s: %v", currentCommit.ParentHash, err)
			break
		}

		currentCommit = parentCommit
	}

	// 构建最终快照（只包含未删除的文件）
	result := make([]Tree, 0, len(fileMap))
	for _, tree := range fileMap {
		if tree != nil {
			// 归一化状态为 StatusAdd（表示这是快照中的文件）
			result = append(result, Tree{
				Hash:      tree.Hash,
				Path:      tree.Path,
				Name:      tree.Name,
				Timestamp: tree.Timestamp,
				Status:    StatusAdd,
			})
		}
	}

	log.Printf("Built complete snapshot with %d files from commit history", len(result))
	return result, nil
}

// normalizeTreesForSnapshot 将提交中的Trees归一为快照写入index
// - 统一将 Status 设为 StatusAdd，避免旧提交中的 Modify/Delete 干扰当前index
// - 保留 Path/Hash/Name/Timestamp 信息
func normalizeTreesForSnapshot(trees []Tree) []Tree {
	result := make([]Tree, 0, len(trees))
	for _, t := range trees {
		result = append(result, Tree{
			Hash:      t.Hash,
			Path:      t.Path,
			Name:      t.Name,
			Timestamp: t.Timestamp,
			Status:    StatusAdd,
		})
	}
	return result
}

// restoreWorkingDirectoryWithDiff 根据目标提交与旧索引的差异恢复工作区
// - 扫描工作区，删除所有不在目标提交中的文件（包括未跟踪的文件）
// - 写入目标提交中的所有文件（无论本地是否存在）
func restoreWorkingDirectoryWithDiff(manager *LocalManager, targetTrees []Tree, oldIndexMap map[string]Tree) error {
	root, err := manager.GetRoot()
	if err != nil {
		return err
	}

	// 目标集合
	targetSet := make(map[string]bool)
	for _, t := range targetTrees {
		targetSet[t.Path] = true
	}

	// 1. 扫描工作区，删除所有不在目标提交中的文件
	deletedCount := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略错误，继续
		}

		// 跳过.minigit目录
		if info.IsDir() && info.Name() == Mark {
			return filepath.SkipDir
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 获取相对路径
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		// 如果文件不在目标集合中，删除它
		if !targetSet[relPath] {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				log.Printf("Warning: failed to remove file %s: %v", relPath, err)
			} else if err == nil {
				log.Printf("Deleted: %s", relPath)
				deletedCount++
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("Warning: error walking directory: %v", err)
	}

	if deletedCount > 0 {
		log.Printf("Deleted %d file(s) not in target commit", deletedCount)
	}

	// 2. 写入目标提交中的所有文件（无论本地是否存在都要恢复）
	restoredCount := 0
	for _, tree := range targetTrees {
		content, err := manager.GetBlob(tree.Hash)
		if err != nil {
			log.Printf("Error: failed to read blob for %s: %v", tree.Path, err)
			return err
		}
		fullPath := filepath.Join(root, tree.Path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Error: failed to create directory for %s: %v", tree.Path, err)
			return err
		}
		// 强制写入文件，恢复到目标提交的状态
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			log.Printf("Error: failed to restore file %s: %v", tree.Path, err)
			return err
		}
		log.Printf("Restored: %s", tree.Path)
		restoredCount++
	}
	log.Printf("Restored %d file(s) from target commit", restoredCount)

	return nil
}

// restoreWorkingDirectoryFromSnapshot 从快照恢复工作区（用于clone/pull后恢复文件）
// 直接写入所有文件，不删除任何文件
func restoreWorkingDirectoryFromSnapshot(manager *LocalManager, snapshot []Tree) error {
	root, err := manager.GetRoot()
	if err != nil {
		return err
	}

	// 写入快照中的所有文件
	for _, tree := range snapshot {
		content, err := manager.GetBlob(tree.Hash)
		if err != nil {
			log.Printf("Error: failed to read blob for %s: %v", tree.Path, err)
			return err
		}
		fullPath := filepath.Join(root, tree.Path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Error: failed to create directory for %s: %v", tree.Path, err)
			return err
		}
		// 写入文件
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			log.Printf("Error: failed to restore file %s: %v", tree.Path, err)
			return err
		}
		log.Printf("Restored: %s", tree.Path)
	}

	return nil
}

func restoreWorkingDirectory(manager *LocalManager, trees []Tree) error {
	root, err := manager.GetRoot()
	if err != nil {
		return err
	}

	// 获取当前工作区所有已跟踪的文件
	workTree := NewWorkTree(manager)
	indexMap, err := workTree.GetIndexMap()
	if err != nil {
		return err
	}

	// 删除当前工作区中所有已跟踪的文件
	for path := range indexMap {
		fullPath := root + "/" + path
		err := os.Remove(fullPath)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: failed to remove file %s: %v", path, err)
		}
	}

	// 恢复目标提交中的所有文件
	for _, tree := range trees {
		// 读取文件内容
		content, err := manager.GetBlob(tree.Hash)
		if err != nil {
			log.Printf("Warning: failed to read blob for %s: %v", tree.Path, err)
			continue
		}

		// 确保目录存在
		fullPath := root + "/" + tree.Path
		dir := filepath.Dir(fullPath)
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			log.Printf("Warning: failed to create directory for %s: %v", tree.Path, err)
			continue
		}

		// 写入文件
		err = os.WriteFile(fullPath, content, 0644)
		if err != nil {
			log.Printf("Warning: failed to restore file %s: %v", tree.Path, err)
			continue
		}

		log.Printf("Restored: %s", tree.Path)
	}

	return nil
}
