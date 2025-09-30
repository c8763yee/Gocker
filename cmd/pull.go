// cmd/pull.go
package cmd

import (
	"fmt"
	"os"

	// "github.com/spf13/cobra"

	"gocker/internal/image"
)

func PullImage(args []string) {
	if len(args) < 1 {
		fmt.Println("用法: gocker pull <image_name>")
		os.Exit(1)
	}

	imageName := args[0]
	fmt.Printf("正在拉取映像: %s\n", imageName)

	manager := image.NewManager()
	if err := manager.Pull(imageName); err != nil {
		panic(fmt.Sprintf("拉取映像 '%s' 失敗: %v", imageName, err))
	}

	fmt.Printf("成功拉取映像: %s\n", imageName)
}
