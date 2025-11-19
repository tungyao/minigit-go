package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// remote 实现的功能包括添加远程仓库、删除远程仓库、修改远程仓库名称、修改远程仓库URL

func CmdRemote(args []string) {
	manager := &LocalManager{}
	if !manager.IsInit() {
		log.Println("未初始化仓库，请先执行 init")
		return
	}

	if len(args) == 0 {
		// 直接打印所有远程仓库
		listRemotes(manager)
		return
	}

	switch args[0] {
	case "add":
		if len(args) < 3 {
			log.Println("用法: remote add <name> <url> [password]")
			return
		}
		password := ""
		if len(args) >= 4 {
			password = args[3]
		}
		addRemote(manager, args[1], args[2], password)
	case "remove", "rm":
		if len(args) < 2 {
			log.Println("用法: remote remove <name>")
			return
		}
		removeRemote(manager, args[1])
	case "rename":
		if len(args) < 3 {
			log.Println("用法: remote rename <old> <new>")
			return
		}
		renameRemote(manager, args[1], args[2])
	case "set-url":
		if len(args) < 3 {
			log.Println("用法: remote set-url <name> <url>")
			return
		}
		setRemoteUrl(manager, args[1], args[2])
	case "set-password":
		if len(args) < 3 {
			log.Println("用法: remote set-password <name> <password>")
			return
		}
		setRemotePassword(manager, args[1], args[2])
	default:
		log.Println("未知命令:", args[0])
	}
}

// getConfigPath 获取config文件路径
func getConfigPath(manager *LocalManager) (string, error) {
	root, err := manager.GetRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, Mark, "config"), nil
}

// loadConfig 加载config文件
func loadConfig(manager *LocalManager) (*Conf, error) {
	configPath, err := getConfigPath(manager)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，返回空配置
			return &Conf{
				Remote:   make(map[string]string),
				Password: make(map[string]string),
			}, nil
		}
		return nil, err
	}

	var conf Conf
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, err
	}

	if conf.Remote == nil {
		conf.Remote = make(map[string]string)
	}
	if conf.Password == nil {
		conf.Password = make(map[string]string)
	}

	return &conf, nil
}

// saveConfig 保存config文件
func saveConfig(manager *LocalManager, conf *Conf) error {
	configPath, err := getConfigPath(manager)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// listRemotes 列出所有远程仓库
func listRemotes(manager *LocalManager) {
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	if len(conf.Remote) == 0 {
		log.Println("没有配置远程仓库")
		return
	}

	for name, url := range conf.Remote {
		fmt.Printf("%s\t%s\n", name, url)
	}
}

// addRemote 添加远程仓库
func addRemote(manager *LocalManager, name, url, password string) {
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	if _, exists := conf.Remote[name]; exists {
		log.Printf("远程仓库 '%s' 已存在", name)
		return
	}

	conf.Remote[name] = url
	if password != "" {
		conf.Password[name] = password
	}

	if err := saveConfig(manager, conf); err != nil {
		log.Printf("保存配置失败: %v", err)
		return
	}

	log.Printf("添加远程仓库 '%s' -> %s", name, url)
}

// removeRemote 删除远程仓库
func removeRemote(manager *LocalManager, name string) {
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	if _, exists := conf.Remote[name]; !exists {
		log.Printf("远程仓库 '%s' 不存在", name)
		return
	}

	delete(conf.Remote, name)

	if err := saveConfig(manager, conf); err != nil {
		log.Printf("保存配置失败: %v", err)
		return
	}

	log.Printf("删除远程仓库 '%s'", name)
}

// renameRemote 重命名远程仓库
func renameRemote(manager *LocalManager, oldName, newName string) {
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	url, exists := conf.Remote[oldName]
	if !exists {
		log.Printf("远程仓库 '%s' 不存在", oldName)
		return
	}

	if _, exists := conf.Remote[newName]; exists {
		log.Printf("远程仓库 '%s' 已存在", newName)
		return
	}

	delete(conf.Remote, oldName)
	conf.Remote[newName] = url

	if err := saveConfig(manager, conf); err != nil {
		log.Printf("保存配置失败: %v", err)
		return
	}

	log.Printf("重命名远程仓库: '%s' -> '%s'", oldName, newName)
}

// setRemoteUrl 修改远程仓库URL
func setRemoteUrl(manager *LocalManager, name, url string) {
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	if _, exists := conf.Remote[name]; !exists {
		log.Printf("远程仓库 '%s' 不存在", name)
		return
	}

	conf.Remote[name] = url

	if err := saveConfig(manager, conf); err != nil {
		log.Printf("保存配置失败: %v", err)
		return
	}

	log.Printf("修改远程仓库 '%s' URL -> %s", name, url)
}

// setRemotePassword 修改远程仓库密码
func setRemotePassword(manager *LocalManager, name, password string) {
	conf, err := loadConfig(manager)
	if err != nil {
		log.Printf("加载配置失败: %v", err)
		return
	}

	if _, exists := conf.Remote[name]; !exists {
		log.Printf("远程仓库 '%s' 不存在", name)
		return
	}

	conf.Password[name] = password

	if err := saveConfig(manager, conf); err != nil {
		log.Printf("保存配置失败: %v", err)
		return
	}

	log.Printf("修改远程仓库 '%s' 密码", name)
}
