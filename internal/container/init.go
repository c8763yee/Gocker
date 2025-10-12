package container

import (
	"encoding/json"
	"fmt"
	"gocker/internal/config"
	"gocker/internal/network"
	"gocker/internal/types"
	"os"
	"os/exec"
	"syscall"

	"github.com/sirupsen/logrus"
)

// InitContainer 負責所有子行程在新的 Namespace 中的初始化工作
func InitContainer() error {

	log := logrus.WithFields(logrus.Fields{
		"pid": os.Getpid(),
	})
	log.Info("子行程: 在新的 Namespace 中啟動...")

	// 1. 從傳入的管道 (fd 3) 中讀取並解析 RunRequest 配置
	pipe := os.NewFile(uintptr(3), "pipe")
	defer pipe.Close()

	var req types.RunRequest
	decoder := json.NewDecoder(pipe)
	if err := decoder.Decode(&req); err != nil {
		return fmt.Errorf("子行程: 從管道讀取配置失敗: %w", err)
	}
	log.Infof("子行程: 成功解析配置，準備執行命令 '%s'", req.ContainerCommand)

	// 2. 設定容器的主機名稱
	if err := syscall.Sethostname([]byte(req.ContainerName)); err != nil {
		return fmt.Errorf("子行程: 設定主機名稱失敗: %w", err)
	}

	// 3. 設定根檔案系統 (Rootfs)
	if err := SetupRootfs(req.MountPoint, req.ImageName, req.ImageTag); err != nil {
		return fmt.Errorf("子行程: 設定 rootfs 失敗: %w", err)
	}
	log.Info("子行程: Rootfs 掛載成功")

	// 4. 設定 DNS
	if err := network.SetupDNS(); err != nil {
		return fmt.Errorf("子行程: 設定 DNS 失敗: %w", err)
	}

	// 5. 在容器內部設定網路
	if err := network.ConfigureContainerNetwork(req.VethPeerName); err != nil {
		return fmt.Errorf("子行程: 設定容器網路失敗: %w", err)
	}
	log.Info("子行程: 容器內網路設定完成")

	// 6. 使用 syscall.Exec 執行使用者指定的命令
	cmdPath, err := exec.LookPath(req.ContainerCommand)
	if err != nil {
		return fmt.Errorf("子行程: 找不到命令 '%s': %w", req.ContainerCommand, err)
	}

	// 8. 啟用 eBPF 監控服務
	stdoutFile, err := os.OpenFile(config.BPFServiceOutputLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("子行程: 開啟 eBPF 監控服務輸出檔失敗: %w", err)
	}

	cmd := exec.Command(config.BPFServiceExeContainer)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdoutFile
	cmd.Stderr = stdoutFile

	if err := cmd.Start(); err != nil {
		// return fmt.Errorf("子行程: 啟動 eBPF 監控服務失敗: %w", err)
		log.Errorf("子行程: 啟動 eBPF 監控服務失敗: %v", err)
	}

	// 9. 執行使用者命令
	log.Infof("子行程: 執行 exec syscall: %s", cmdPath)
	args := append([]string{req.ContainerCommand}, req.ContainerArgs...)
	if err := syscall.Exec(cmdPath, args, os.Environ()); err != nil {
		return fmt.Errorf("子行程: exec 失敗: %w", err)
	}

	return nil
}
