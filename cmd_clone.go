package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

// CmdClone 克隆远程仓库到本地目录
// 用法: minigit clone <remote_url> <dir>
func CmdClone(args []string) {
	if len(args) < 1 {
		log.Println("用法: minigit clone <remote_url> [dir]")
		return
	}

	remoteUrl := args[0]
	targetDir := ""
	if len(args) >= 2 {
		targetDir = args[1]
	} else {
		// 未提供目录时，使用远程URL中的项目名
		projectFromURL := parseProjectFromURL(remoteUrl)
		if projectFromURL == "" {
			projectFromURL = "project"
		}
		targetDir = projectFromURL
	}

	// 密码
	password := readPassword("请输入远程仓库密码: ")

	// 创建目标目录
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Printf("创建目录失败: %v", err)
		return
	}

	// 切换到目标目录，方便复用 LocalManager
	cwd, _ := os.Getwd()
	if err := os.Chdir(targetDir); err != nil {
		log.Printf("切换目录失败: %v", err)
		return
	}
	defer os.Chdir(cwd)

	// 初始化本地仓库结构
	if err := CmdInit(nil); err != nil {
		log.Printf("初始化仓库失败: %v", err)
		return
	}

	// 项目名称使用远程URL解析
	projectName := parseProjectFromURL(remoteUrl)

	// 构建拉取请求（本地没有任何对象）
	req := PullRequest{
		Project:    projectName,
		LocalHeads: []string{},
	}
	data, err := json.Marshal(req)
	if err != nil {
		log.Printf("构建请求失败: %v", err)
		return
	}

	// 发送请求到远程
	httpReq, err := http.NewRequest("POST", remoteUrl+"/pull", bytes.NewReader(data))
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Password", password)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("克隆失败: %s", string(body))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应失败: %v", err)
		return
	}

	var pullResp PullResponse
	if err := json.Unmarshal(body, &pullResp); err != nil {
		log.Printf("解析响应失败: %v", err)
		return
	}

	if len(pullResp.Heads) == 0 || len(pullResp.Data) == 0 {
		log.Println("远程仓库为空或无可拉取对象。")
		// 仍然写入远程配置
		manager := &LocalManager{}
		addRemote(manager, "origin", remoteUrl, password)
		return
	}

	// 解密数据
	decrypted, err := decryptData(pullResp.Data, password)
	if err != nil {
		log.Printf("解密失败: %v", err)
		return
	}

	// 解压到本地 objects
	manager := &LocalManager{}
	if err := extractLocalTar(manager, decrypted); err != nil {
		log.Printf("解压失败: %v", err)
		return
	}
	log.Println("收到heads:", pullResp.Heads)
	// 设置 HEAD 为最新提交，并构建完整快照恢复工作区
	latest := pullResp.Latest
	if latest == "" && len(pullResp.Heads) > 0 {
		latest = pullResp.Heads[len(pullResp.Heads)-1]
	}
	if latest != "" {
		// 写入 HEAD
		if err := manager.FlushHead(latest); err != nil {
			log.Printf("写入HEAD失败: %v", err)
		} else {
			// 加载最新提交
			commit, err := LoadCommit(manager, latest)
			if err != nil {
				log.Printf("加载最新提交失败: %v", err)
			} else {
				// 构建完整快照（遍历历史提交）
				completeSnapshot, err := buildCompleteSnapshot(manager, latest, commit)
				if err != nil {
					log.Printf("构建完整快照失败: %v", err)
				} else {
					log.Printf("构建完整快照: %d 个文件", len(completeSnapshot))
					// 写入索引
					if err := manager.FlushIndex(completeSnapshot); err != nil {
						log.Printf("刷新索引失败: %v", err)
					} else {
						// 恢复工作区文件
						if err := restoreWorkingDirectoryFromSnapshot(manager, completeSnapshot); err != nil {
							log.Printf("恢复工作区失败: %v", err)
						} else {
							log.Printf("已恢复 %d 个文件到工作区", len(completeSnapshot))
						}
					}
				}
			}
		}
	}

	// 写入远程配置 origin
	addRemote(manager, "origin", remoteUrl, password)

	log.Printf("克隆完成: %s -> %s", remoteUrl, targetDir)
}
