package main

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
)

// CmdLog 处理log命令,显示提交历史
func CmdLog(args []string) {
	// 解析参数
	limit := -1      // -1表示显示所有
	showAll := false // 是否显示所有提交（包括不在当前分支的）
	for i := 0; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				log.Printf("Error: invalid number for -n flag: %s", args[i+1])
				return
			}
			limit = n
			i++ // 跳过数字参数
		} else if args[i] == "--all" {
			showAll = true
		}
	}

	manager := LocalManager{}

	// 检查是否已初始化
	if !manager.IsInit() {
		log.Println("Error: not a minigit repository. Please run 'minigit init' first.")
		return
	}

	if showAll {
		// 显示所有提交对象
		showAllCommits(&manager, limit)
		return
	}

	// 获取当前HEAD
	headHash, err := manager.GetHeadContent()
	if err != nil || headHash == "" {
		log.Println("No commits yet.")
		return
	}

	// 遍历提交历史
	count := 0
	currentHash := headHash

	for currentHash != "" {
		// 检查是否达到限制
		if limit > 0 && count >= limit {
			break
		}

		// 读取commit对象
		commit, err := loadCommitObject(&manager, currentHash)
		if err != nil {
			log.Printf("Error loading commit %s: %v", currentHash, err)
			break
		}

		// 打印提交信息
		printCommit(currentHash, commit, count == 0)
		count++

		// 移动到父提交
		currentHash = commit.ParentHash
	}

	if count == 0 {
		log.Println("No commits yet.")
	} else {
		log.Printf("\nTotal: %d commit(s)", count)
	}
}

// loadCommitObject 从objects目录加载commit对象
func loadCommitObject(manager *LocalManager, hash string) (*HeadObj, error) {
	root, err := manager.GetRoot()
	if err != nil {
		return nil, err
	}

	// 构造对象路径
	objectPath := root + "/" + Mark + "/objects/" + hash[:2] + "/" + hash[2:]

	// 读取文件内容
	content, err := os.ReadFile(objectPath)
	if err != nil {
		return nil, err
	}

	// 解析JSON
	var commit HeadObj
	err = json.Unmarshal(content, &commit)
	if err != nil {
		return nil, err
	}

	return &commit, nil
}

// printCommit 打印提交信息
func printCommit(hash string, commit *HeadObj, isLatest bool) {
	log.Println("\n" + strings.Repeat("=", 60))
	if isLatest {
		log.Printf("commit %s (HEAD)", hash)
	} else {
		log.Printf("commit %s", hash)
	}
	log.Println(strings.Repeat("-", 60))
	log.Printf("Date:    %s", commit.Timestamp)
	log.Printf("Message: %s", commit.Message)
	log.Printf("Files:   %d", len(commit.Trees))

	// 统计文件变更
	added := 0
	modified := 0
	deleted := 0

	for _, tree := range commit.Trees {
		switch tree.Status {
		case StatusAdd:
			added++
		case StatusModify:
			modified++
		case StatusDelete:
			deleted++
		}
	}

	log.Printf("Changes: +%d ~%d -%d", added, modified, deleted)
	log.Println(strings.Repeat("=", 60))
}

// showAllCommits 显示所有提交对象（从 commits 文件读取）
func showAllCommits(manager *LocalManager, limit int) {
	// 从 commits 文件读取所有提交
	commitRefs, err := manager.GetAllCommits()
	if err != nil {
		log.Printf("Error reading commits: %v", err)
		return
	}

	if len(commitRefs) == 0 {
		log.Println("No commits found.")
		return
	}

	// 获取当前HEAD
	headHash, _ := manager.GetHeadContent()

	// 打印提交
	count := 0
	for _, commitRef := range commitRefs {
		if limit > 0 && count >= limit {
			break
		}

		// 加载完整的commit对象
		commit, err := loadCommitObject(manager, commitRef.Hash)
		if err != nil {
			log.Printf("Warning: failed to load commit %s: %v", commitRef.Hash, err)
			continue
		}

		isLatest := commitRef.Hash == headHash
		printCommit(commitRef.Hash, commit, isLatest)
		count++
	}

	log.Printf("\nTotal: %d commit(s)", count)
}
