package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// CmdPull 拉取命令
func CmdPull(args []string) {
	manager := &LocalManager{}
	if !manager.IsInit() {
		log.Println("未初始化仓库，请先执行 init")
		return
	}

	// 默认从 origin 拉取
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

	// 获取本地所有commits
	localHeads, err := getLocalHeads(manager)
	if err != nil {
		log.Printf("获取本地commits失败: %v", err)
		return
	}

	// 从远程URL解析项目名称
	projectName := parseProjectFromURL(remoteUrl)

	// 构建拉取请求
	req := PullRequest{
		Project:    projectName,
		LocalHeads: localHeads,
	}

	reqData, err := json.Marshal(req)
	if err != nil {
		log.Printf("构建请求失败: %v", err)
		return
	}

	// 发送拉取请求
	httpReq, err := http.NewRequest("POST", remoteUrl+"/pull", bytes.NewReader(reqData))
	if err != nil {
		log.Printf("创建请求失败: %v", err)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Password", password) // 使用配置的密码

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("拉取失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("拉取失败: %s", string(body))
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

	if len(pullResp.Heads) == 0 {
		log.Println("已是最新，无需拉取")
		return
	}

	log.Printf("准备拉取 %d 个新commit", len(pullResp.Heads))

	// 解密数据
	decryptedData, err := decryptData(pullResp.Data, password)
	if err != nil {
		log.Printf("解密失败: %v", err)
		return
	}

	// 解压tar包
	if err := extractLocalTar(manager, decryptedData); err != nil {
		log.Printf("解压失败: %v", err)
		return
	}

	// 根据最新HEAD构建完整快照并更新索引与工作区
	latest := pullResp.Latest
	if latest == "" && len(pullResp.Heads) > 0 {
		latest = pullResp.Heads[len(pullResp.Heads)-1]
	}
	if latest != "" {
		// 写入 HEAD
		if err := manager.FlushHead(latest); err != nil {
			log.Printf("刷新HEAD失败: %v", err)
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

	log.Printf("成功拉取 %d 个commit从 %s", len(pullResp.Heads), remoteName)
}

// extractLocalTar 解压tar包到本地仓库
func extractLocalTar(manager *LocalManager, data []byte) error {
	buf := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(buf)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		// 读取commit数据
		commitData := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, commitData); err != nil {
			return err
		}

		// 保存到本地objects目录
		hash := header.Name
		if err := manager.SaveBlob(hash, commitData); err != nil {
			log.Printf("保存对象 %s 失败: %v", hash, err)
		}
	}

	return nil
}
