// cmd/run.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"gocker/internal/container"
	"gocker/internal/network"

	"github.com/spf13/cobra"
)

var runCommand = &cobra.Command{
	Use:   "run [OPTIONS] IMAGE COMMAND [ARG...]",
	Short: "Run a command in a new container",
	Long:  "Run a command in a new container with specified image and command.",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		imageName, imageTag := input_parse(args[0])
		RunContainer(args)
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
	rootCmd.AddCommand(runCommand)
}

// RunContainer 執行父行程的邏輯
func RunContainer(args []string) {
	if len(args) < 2 {
		fmt.Println("用法: gocker run <image> <command>")
		os.Exit(1)
	}

	fmt.Printf("父行程: 準備啟動容器... PID: %d\n", os.Getpid())

	// 設定網橋
	if err := network.SetupBridge(); err != nil {
		panic(fmt.Sprintf("設定主機網橋失敗: %v", err))
	}

	// 準備子行程參數
	childArgs := append([]string{"child"}, args...)
	cmd := exec.Command("/proc/self/exe", childArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 啟動子行程
	if err := cmd.Start(); err != nil {
		fmt.Printf("錯誤: 執行 cmd.Start() 失敗: %v", err)
		os.Exit(1)
	}

	childPid := cmd.Process.Pid
	fmt.Printf("子行程 PID: %d\n", childPid)

	// 等待子行程完成初始化
	time.Sleep(100 * time.Millisecond)

	// 設定容器網路
	if err := network.SetupContainerNetwork(childPid); err != nil {
		fmt.Printf("警告: 設定容器網路失敗: %v\n", err)
	}

	// 等待子行程結束
	cmd.Wait()
}

// RunChildProcess 執行子行程的邏輯
func RunChildProcess(args []string) {
	if len(args) < 2 {
		fmt.Println("子行程參數不足")
		os.Exit(1)
	}

	fmt.Printf("子行程: 在新的 Namespace 中執行... PID: %d\n", os.Getpid())

	imageName := args[0]
	userCmd := args[1]
	userArgs := args[1:]

	// 1. 設定容器的主機名稱
	syscall.Sethostname([]byte("gocker-container"))

	// 2. 準備並切換根目錄
	mergedDir, err := container.SetupOverlayFS(imageName)
	if err != nil {
		panic(fmt.Sprintf("準備 OverlayFS 失敗: %v", err))
	}

	container.PivotRoot(mergedDir)

	// 3. 掛載 /proc
	if err := syscall.Mount("proc", "/proc", "proc", 0, ""); err != nil {
		panic(fmt.Sprintf("掛載 /proc 失敗: %v", err))
	}

	// 4. 設定 Cgroup
	container.SetupCgroup()

	// 5. 設定容器內的網路
	network.ConfigureContainerNetwork()

	// 6. 設定 DNS
	network.SetupDNS()

	// 7. 執行使用者指定的命令
	if err := syscall.Exec(userCmd, userArgs, os.Environ()); err != nil {
		panic(fmt.Sprintf("syscall.Exec 在 '%s' 上失敗: %v", userCmd, err))
	}
}
