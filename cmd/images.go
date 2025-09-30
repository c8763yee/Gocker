// cmd/images.go
package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"gocker/internal/image"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// imagesCmd 代表 'gocker images' 指令
var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "List all locally stored images",
	Long:  `Lists all images that have been pulled and are stored locally in the gocker storage path.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. 建立一個 Image Manager 的實例
		mgr := image.NewManager()

		// 2. 在 mgr 這個實例上呼叫 ListImages 方法
		localImages, err := mgr.ListImages()
		if err != nil {
			logrus.Fatalf("Failed to list images: %v", err)
		}

		// 3. 使用 text/tabwriter 來建立一個格式化的表格輸出
		w := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)

		// 4. 打印表頭
		// 因為後端回傳的是 string，我們先只顯示 REPOSITORY 和 TAG
		fmt.Fprint(w, "REPOSITORY\tTAG\n")

		// 5. 遍歷映像列表並打印每一行
		for _, imgName := range localImages {
			// 從 "repository:tag" 格式的字串中分離出兩部分
			parts := strings.Split(imgName, ":")
			repo := parts[0]
			tag := "latest" // 預設 tag
			if len(parts) > 1 {
				tag = parts[1]
			}
			fmt.Fprintf(w, "%s\t%s\n", repo, tag)
		}

		// 6. 確保所有內容都已寫入標準輸出
		if err := w.Flush(); err != nil {
			logrus.Errorf("Failed to flush output: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(imagesCmd)
}
