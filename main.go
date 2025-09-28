// main.go
package main

import (
	"fmt"
	"os"

	"gocker/cmd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: gocker <command> [args...]")
		fmt.Println("命令:")
		fmt.Println("  run <image> <command>  - 運行容器")
		fmt.Println("  pull <image>           - 拉取映像")
		fmt.Println("  images                 - 列出本地映像")
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "run":
		cmd.RunContainer(args)
	case "child":
		cmd.RunChildProcess(args)
	case "pull":
		cmd.PullImage(args)
	case "images":
		cmd.ListImages()
	default:
		fmt.Printf("未知的命令: %s\n", command)
		os.Exit(1)
	}
}
