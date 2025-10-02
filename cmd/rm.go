// gocker/cmd/rm.go
package cmd

import (
	"fmt"

	"gocker/internal"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rmCommand = &cobra.Command{
	Use:   "rm [CONTAINER_ID]",
	Short: "Remove a container",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		containerID := args[0]
		if err := internal.RemoveContainer(containerID); err != nil {
			logrus.Fatalf("Failed to remove container %s: %v", containerID, err)
		}
		fmt.Println(containerID)
	},
}

func init() {
	rootCmd.AddCommand(rmCommand)
}
