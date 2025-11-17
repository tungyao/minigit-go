package main

// remote 实现的功能包括添加远程仓库、删除远程仓库、修改远程仓库名称、修改远程仓库URL

func CmdRemote(args []string) {
	if len(args) > 1 {
		switch args[0] {
		case "add":
			// 添加远程仓库
		case "remove":
			// 删除远程仓库
		case "rename":
			// 修改远程仓库名称
		case "update":
			// 修改远程仓库URL
		}
	} else {
		// 直接打印吧
	}

}
