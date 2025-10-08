// cmd/run.go
package cmd

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"gocker/internal"
	"gocker/internal/config"
	"gocker/internal/types"
	"gocker/pkg"
)

var request types.RunRequest

var runCommand = &cobra.Command{
	Use:   "run [OPTIONS] IMAGE COMMAND [ARG...]",
	Short: "Run a command in a new container",
	Long: `
Run a command in a new container with specified image and command.
Use double dashes (--) if you want to pass arguments to the command. like 'gocker run --<flags>... -- /bin/sh'."`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := os.Stat("eBPF/ebpf-sched-monitor"); os.IsNotExist(err) {
			logrus.Fatal(
				`
eBPF/ebpf-sched-monitor 執行檔不存在，
請先在 Gocker 目錄中執行
1. 進入 eBPF 目錄，然後執行 make 指令
2. 透過 go generate 指令來生成 eBPF/ebpf-sched-monitor。`)
		}

		imageName, imageTag := pkg.Parse(args[0])
		logrus.Infof("Image: %s, Tag: %s", imageName, imageTag)

		request.ImageName = imageName
		request.ImageTag = imageTag

		if len(args) == 1 {
			logrus.Info("沒有指定容器命令，預設使用 /bin/sh")
			request.ContainerCommand = config.DefaultCommand
			request.ContainerArgs = []string{}
		} else {
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

func init() {
	runCommand.Flags().StringVarP(&request.ContainerName, "name", "", "", "Assign a name to the container")
	runCommand.Flags().IntVar(&request.PidsLimit, "pids-limit", config.DefaultPidsLimit, "Limit the number of container tasks")
	runCommand.Flags().IntVarP(&request.MemoryLimit, "memory", "m", config.DefaultMemoryLimit, "Limit the memory")
	runCommand.Flags().IntVar(&request.CPULimit, "cpus", config.DefaultCPULimit, "Limit the number of CPUs")
	rootCmd.AddCommand(runCommand)
}
