// cmd/start.go
package cmd

import (
	"encoding/json"
	"gocker/internal/api"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type StartRequest struct {
	ContainerID string `json:"container_id"`
}

var startCommand = &cobra.Command{
	Use:   "start CONTAINER",
	Short: "啟動一個已停止的容器",
	Long:  "透過容器的 ID 或名稱來啟動一個已停止的容器。",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		containerIdentifier := args[0]

		payload, err := json.Marshal(StartRequest{ContainerID: containerIdentifier})
		if err != nil {
			logrus.Fatalf("序列化 start 請求失敗: %v", err)
		}

		req := types.Request{
			Command: "start",
			Payload: payload,
		}

		res, err := api.SendRequest(req)
		if err != nil {
			logrus.Fatalf("與 gocker-daemon 通訊失敗: %v", err)
		}

		if res.Status == "success" {
			logrus.Info(res.Message)
		} else {
			logrus.Fatalf("來自 Daemon 的錯誤: %s", res.Message)
		}
	},
}

func init() {
	rootCmd.AddCommand(startCommand)
}
