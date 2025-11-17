package main

import (
	"log"
	"os"
	"strings"
)

// CmdStatus 处理status命令,显示工作区文件状态
func CmdStatus(args []string) {
	manager := LocalManager{}

	// 检查是否已初始化
	if !manager.IsInit() {
		log.Println("Error: not a minigit repository. Please run 'minigit init' first.")
		return
	}

	// 创建工作区管理器
	workTree := NewWorkTree(&manager)

	// 获取状态
	log.Println("Checking status...")
	trees, err := workTree.GetStatus()
	if err != nil {
		log.Printf("Error getting status: %v", err)
		return
	}

	if len(trees) == 0 {
		log.Println("No changes detected")
		return
	}

	// 分类统计
	var added, modified, deleted, untrackedDirs []Tree
	for _, tree := range trees {
		switch tree.Status {
		case StatusAdd:
			// 检查是否是目录(路径以分隔符结尾)
			if strings.HasSuffix(tree.Path, string(os.PathSeparator)) {
				untrackedDirs = append(untrackedDirs, tree)
			} else {
				added = append(added, tree)
			}
		case StatusModify:
			modified = append(modified, tree)
		case StatusDelete:
			deleted = append(deleted, tree)
		}
	}

	// 输出结果
	if len(added) > 0 {
		log.Println("\nNew files:")
		for _, tree := range added {
			log.Printf("  + %s", tree.Path)
		}
	}

	if len(untrackedDirs) > 0 {
		log.Println("\nUntracked directories:")
		for _, tree := range untrackedDirs {
			log.Printf("  ? %s", tree.Path)
		}
	}

	if len(modified) > 0 {
		log.Println("\nModified files:")
		for _, tree := range modified {
			log.Printf("  M %s", tree.Path)
		}
	}

	if len(deleted) > 0 {
		log.Println("\nDeleted files:")
		for _, tree := range deleted {
			log.Printf("  - %s", tree.Path)
		}
	}

	log.Printf("\nTotal: %d added, %d modified, %d deleted, %d untracked dirs",
		len(added), len(modified), len(deleted), len(untrackedDirs))
}
