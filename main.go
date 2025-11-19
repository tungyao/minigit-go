package main

import (
	"log"
	"os"
)

func main() {
	log.SetPrefix("[minigit] ")
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	args := os.Args
	if len(args) < 2 {
		log.Println("Usage: minigit <command> [<args>]")
		return
	}
	switch args[1] {
	case "init":
		CmdInit(args[2:])
	case "add":
		CmdAdd(args[2:])
	case "commit":
		CmdCommit(args[2:])
	case "status":
		CmdStatus(args[2:])
	case "log":
		CmdLog(args[2:])
	case "reset":
		CmdReset(args[2:])
	case "clone":
		CmdClone(args[2:])
	case "server":
		CmdServer(args[2:])
	case "push":
		CmdPush(args[2:])
	case "pull":
		CmdPull(args[2:])
	case "remote":
		CmdRemote(args[2:])
	default:
		log.Println("unknown command")
	}
}
