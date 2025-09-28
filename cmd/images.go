// cmd/images.go
package cmd

import (
	"fmt"

	"gocker/internal/image"
)

// ListImages 列出本地映像
func ListImages() {
	fmt.Println("本地映像列表:")

	manager := image.NewManager()
	images, err := manager.List()
	if err != nil {
		panic(fmt.Sprintf("讀取映像列表失敗: %v", err))
	}

	if len(images) == 0 {
		fmt.Println("沒有找到本地映像")
		return
	}

	for _, img := range images {
		fmt.Println(img)
	}
}
