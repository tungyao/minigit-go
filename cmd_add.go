package main

import (
	"log"
)

// CmdAdd 处理add命令，将指定文件添加到暂存区
func CmdAdd(args []string) {
	if len(args) == 0 {
		log.Println("Usage: minigit add <file>...")
		return
	}

	manager := LocalManager{}

	// 检查是否已初始化
	if !manager.IsInit() {
		log.Println("Error: not a minigit repository. Please run 'minigit init' first.")
		return
	}

	// 创建工作区管理器
	workTree := NewWorkTree(&manager)

	// 扫描指定的文件/目录
	log.Printf("Scanning files: %v", args)
	newTrees, err := workTree.ScanFiles(args)
	if err != nil {
		log.Printf("Error scanning files: %v", err)
		return
	}

	if len(newTrees) == 0 {
		log.Println("No changes to add")
		return
	}

	// 合并到index
	mergedTrees, err := workTree.MergeIndexWithTrees(newTrees)
	if err != nil {
		log.Printf("Error merging index: %v", err)
		return
	}

	// 刷新index文件
	err = manager.FlushIndex(mergedTrees)
	if err != nil {
		log.Printf("Error flushing index: %v", err)
		return
	}

	// 输出结果
	for _, tree := range newTrees {
		var statusStr string
		switch tree.Status {
		case StatusAdd:
			statusStr = "Added"
		case StatusModify:
			statusStr = "Modified"
		case StatusDelete:
			statusStr = "Deleted"
		}
		log.Printf("%s: %s", statusStr, tree.Path)
	}

	log.Printf("Successfully added %d file(s) to index", len(newTrees))
}
