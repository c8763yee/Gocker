// cmd/build.go
package cmd

import (
	"fmt"
	"gocker/internal/builder"
	"gocker/pkg"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	buildTag  string
	buildFile string
)

var buildCommand = &cobra.Command{
	Use:   "build [OPTIONS] PATH",
	Short: "Build an image from a Gockerfile",
	Long:  "Build an image from a Gockerfile in the specified path.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildContext := args[0]

		// 預設使用當前目錄的 Gockerfile
		if buildFile == "" {
			buildFile = buildContext + "/Gockerfile"
		}

		// 解析映像名稱和標籤
		imageName, imageTag := pkg.Parse(buildTag)
		if imageName == "" {
			logrus.Fatal("請使用 --tag 或 -t 指定映像名稱")
		}

		logrus.Infof("正在建構映像 %s:%s", imageName, imageTag)
		logrus.Infof("使用 Gockerfile: %s", buildFile)

		// 建立 builder 並執行建構
		b, err := builder.NewBuilder(buildFile, imageName, imageTag)
		if err != nil {
			logrus.Fatalf("建立 builder 失敗: %v", err)
		}

		if err := b.Build(); err != nil {
			logrus.Fatalf("建構映像失敗: %v", err)
		}

		fmt.Printf("成功建構映像: %s:%s\n", imageName, imageTag)
	},
}

func init() {
	buildCommand.Flags().StringVarP(&buildTag, "tag", "t", "", "映像名稱和標籤 (格式: name:tag)")
	buildCommand.Flags().StringVarP(&buildFile, "file", "f", "", "Gockerfile 路徑 (預設: PATH/Gockerfile)")
	buildCommand.MarkFlagRequired("tag")
	rootCmd.AddCommand(buildCommand)
}
