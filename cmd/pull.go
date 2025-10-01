// cmd/pull.go
package cmd

import (
	"gocker/internal/image"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var pullCommand = &cobra.Command{
	Use:   "pull [IMAGE_NAME]",
	Short: "Pull an image from a remote repository",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		imageName := args[0]
		logrus.Infof("正在拉取映像: %s", imageName)
		manager := image.NewManager()
		if err := manager.PullImage(imageName); err != nil {
			logrus.Fatalf("拉取映像 %s 失敗: %v", imageName, err)
		}

		logrus.Infof("成功拉取並儲存映像: %s", imageName)
	},
}

func init() {
	rootCmd.AddCommand(pullCommand)
}
