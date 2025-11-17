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
// 1. 更新HEAD指向目标提交
// 2. 更新index为目标提交的trees
// 3. 恢复工作区文件到目标提交的状态
func performHardReset(manager *LocalManager, targetHash string, targetCommit *HeadObj) error {
	// 1. 更新HEAD
	err := manager.FlushHead(targetHash)
	if err != nil {
		return err
	}

	// 2. 更新index
	err = manager.FlushIndex(targetCommit.Trees)
	if err != nil {
		return err
	}

	// 3. 恢复工作区文件
	err = restoreWorkingDirectory(manager, targetCommit.Trees)
	if err != nil {
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

	// 2. 更新index
	err = manager.FlushIndex(targetCommit.Trees)
	if err != nil {
		return err
	}

	// soft模式不改变工作区
	log.Println("Working directory unchanged (soft mode)")
	return nil
}

// restoreWorkingDirectory 恢复工作区到指定的trees状态
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
