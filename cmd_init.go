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
	return nil
}
