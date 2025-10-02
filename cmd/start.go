// cmd/start.go
package cmd

import (
	"gocker/internal/container"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var startCommand = &cobra.Command{
	Use:   "start CONTAINER",
	Short: "Start a stopped container",
	Long:  "Start a stopped container by its ID or name.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		containerIdentifier := args[0]

		manager := container.NewManager()
		if err := manager.Start(containerIdentifier); err != nil {
			logrus.Fatalf("啟動容器 %s 失敗: %v", containerIdentifier, err)
		}

		logrus.Infof("成功啟動容器: %s", containerIdentifier)
	},
}

func init() {
	rootCmd.AddCommand(startCommand)
}
