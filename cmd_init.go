package main

import "os"

func CmdInit(args []string) error {
	manager := LocalManager{}
	root, err := manager.GetRoot()
	if err != nil {
		return err
	}
	err = os.MkdirAll(root+"/"+Mark+"/objects", 0755)
	if err != nil {
		return err
	}
	err = manager.FlushHead("")
	if err != nil {
		return err
	}
	err = manager.FlushIndex(nil)
	if err != nil {
		return err
	}
	// 初始化提交索引文件（commits），写入空数组，便于后续快速读取
	commitsFile, err := manager.GetCommitsFile()
	if err == nil {
		_ = os.WriteFile(commitsFile, []byte("[]"), 0644)
	}
	return nil
}
