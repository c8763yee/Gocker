// cmd/stop.go
package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"gocker/internal/container"
)

var stopCommand = &cobra.Command{
	Use:   "stop CONTAINER",
	Short: "Stop a running container",
	Long:  "Stop a running container by sending a termination signal.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		containerID := args[0]
		logrus.Infof("Stopping container: %s", containerID)
		manager := container.NewManager()

		if err := manager.StopContainer(containerID); err != nil {
			logrus.Fatalf("Failed to stop container: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(stopCommand)
}
