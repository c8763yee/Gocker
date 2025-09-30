// cmd/run.go
package cmd

import (
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"gocker/internal"
	"gocker/internal/config"
	"gocker/internal/types"
)

var request types.RunRequest

var runCommand = &cobra.Command{
	Use:   "run [OPTIONS] IMAGE COMMAND [ARG...]",
	Short: "Run a command in a new container",
	Long:  "Run a command in a new container with specified image and command.",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		imageName, imageTag := input_parse(args[0])
		logrus.Infof("Image: %s, Tag: %s", imageName, imageTag)

		request.ImageName = imageName
		request.ImageTag = imageTag

		if len(args) > 1 {
			request.ContainerCommand = args[1]
			if len(args) > 2 {
				request.ContainerArgs = args[2:]
			}
		}

		if err := internal.RunContainer(&request); err != nil {
			logrus.Fatalf("Failed to run container: %v", err)
		}
	},
}

func input_parse(input string) (string, string) {
	s := strings.Split(input, ":")
	if len(s) == 1 {
		return s[0], "latest"
	}
	if len(s) == 2 {
		return s[0], s[1]
	}
	return "", ""
}

func init() {
	runCommand.Flags().StringVarP(&request.ContainerName, "name", "", "", "Assign a name to the container")
	runCommand.Flags().IntVar(&request.PidsLimit, "pids-limit", config.DefaultPidsLimit, "Limit the number of container tasks")
	runCommand.Flags().IntVarP(&request.MemoryLimit, "memory", "m", config.DefaultMemoryLimit, "Limit the memory")
	runCommand.Flags().IntVar(&request.CPULimit, "cpus", config.DefaultCPULimit, "Limit the number of CPUs")
	rootCmd.AddCommand(runCommand)
}
