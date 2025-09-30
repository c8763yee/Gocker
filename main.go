// main.go
package main

import (
	"os"

	"gocker/cmd"
	"gocker/internal"

	"github.com/sirupsen/logrus"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		logrus.Info("Executing internal 'init' command for container setup")
		if err := internal.InitContainer(); err != nil {
			logrus.Fatalf("容器初始化失敗: %v", err)
		}
		// 子行程的生命週期到此為止
		return
	}
	cmd.Execute()
}
