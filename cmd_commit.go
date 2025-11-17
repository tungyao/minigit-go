package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// CmdCommit 处理commit命令,创建新的提交
func CmdCommit(args []string) {
	if len(args) == 0 {
		log.Println("Usage: minigit commit -m <message>")
		return
	}

	// 解析提交信息
	var message string
	for i := 0; i < len(args); i++ {
		if args[i] == "-m" && i+1 < len(args) {
			message = args[i+1]
			break
		}
	}

	if message == "" {
		log.Println("Error: commit message is required. Use -m flag.")
		return
	}

	manager := LocalManager{}

	// 检查是否已初始化
	if !manager.IsInit() {
		log.Println("Error: not a minigit repository. Please run 'minigit init' first.")
		return
	}

	// 读取当前index
	indexContent, err := manager.GetIndexContent()
	if err != nil {
		log.Printf("Error reading index: %v", err)
		return
	}

	if indexContent == "" {
		log.Println("Nothing to commit. Use 'minigit add' to add files first.")
		return
	}

	var trees []Tree
	err = json.Unmarshal([]byte(indexContent), &trees)
	if err != nil {
		log.Printf("Error parsing index: %v", err)
		return
	}

	if len(trees) == 0 {
		log.Println("Nothing to commit.")
		return
	}

	// 获取父提交hash
	parentHash, _ := manager.GetHeadContent()

	// 检查是否有变更(对比当前index和上次commit)
	if parentHash != "" {
		hasChanges, err := checkIfHasChanges(&manager, parentHash, trees)
		if err != nil {
			log.Printf("Error checking changes: %v", err)
			return
		}
		if !hasChanges {
			log.Println("Nothing to commit, working tree clean.")
			return
		}
	}

	// 创建commit对象
	commit := HeadObj{
		ParentHash: parentHash,
		Message:    message,
		Timestamp:  time.Now().Format(time.RFC3339),
		Trees:      trees,
	}

	// 计算commit hash
	commitJSON, err := json.Marshal(commit)
	if err != nil {
		log.Printf("Error marshaling commit: %v", err)
		return
	}

	commitHash := fmt.Sprintf("%x", sha1.Sum(commitJSON))

	// 保存commit对象到objects目录
	err = saveCommitObject(&manager, commitHash, commit)
	if err != nil {
		log.Printf("Error saving commit object: %v", err)
		return
	}

	// 更新HEAD
	err = manager.FlushHead(commitHash)
	if err != nil {
		log.Printf("Error updating HEAD: %v", err)
		return
	}

	// 输出结果
	log.Printf("Committed successfully!")
	log.Printf("Commit hash: %s", commitHash)
	log.Printf("Message: %s", message)
	log.Printf("Files: %d", len(trees))
}

// saveCommitObject 保存commit对象到objects目录
func saveCommitObject(manager *LocalManager, hash string, commit HeadObj) error {
	root, err := manager.GetRoot()
	if err != nil {
		return err
	}

	// 使用hash的前2个字符作为子目录名
	objectDir := root + "/" + Mark + "/objects/" + hash[:2]
	err = os.MkdirAll(objectDir, 0755)
	if err != nil {
		return err
	}

	// 保存commit对象
	objectPath := objectDir + "/" + hash[2:]
	commitJSON, err := json.MarshalIndent(commit, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(objectPath, commitJSON, 0644)
}

// checkIfHasChanges 检查当前index是否相对于上次commit有变更
func checkIfHasChanges(manager *LocalManager, parentHash string, currentTrees []Tree) (bool, error) {
	// 读取父commit对象
	parentCommit, err := loadCommitObject(manager, parentHash)
	if err != nil {
		return true, nil // 如果读取失败,假设有变更
	}

	// 对比文件数量
	if len(currentTrees) != len(parentCommit.Trees) {
		return true, nil
	}

	// 创建map便于对比
	parentMap := make(map[string]Tree)
	for _, tree := range parentCommit.Trees {
		parentMap[tree.Path] = tree
	}

	// 检查每个文件
	for _, currentTree := range currentTrees {
		parentTree, exists := parentMap[currentTree.Path]
		if !exists {
			// 新文件
			return true, nil
		}
		if currentTree.Hash != parentTree.Hash {
			// 文件内容变更
			return true, nil
		}
	}

	// 检查是否有文件被删除
	for path := range parentMap {
		found := false
		for _, currentTree := range currentTrees {
			if currentTree.Path == path {
				found = true
				break
			}
		}
		if !found {
			// 有文件被删除
			return true, nil
		}
	}

	// 没有任何变更
	return false, nil
}
