package internal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gocker/internal/config"
	"gocker/internal/container"
	"gocker/pkg"

	"gocker/internal/network"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
)

// RunContainer 負責完整的父行程邏輯：建立容器元數據、設定資源並啟動子行程
func RunContainer(req *types.RunRequest) error {
	rootCgroupProcs := "/sys/fs/cgroup/cgroup.procs"
	if err := os.WriteFile(rootCgroupProcs, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("無法將父行程移至 cgroup root: %w", err)
	}

	// 1. 產生一個隨機的容器 ID
	randBytes := make([]byte, 12)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Errorf("無法產生容器 ID: %w", err)
	}
	containerID := hex.EncodeToString(randBytes)

	if req.ContainerName == "" {
		req.ContainerName = containerID
	}

	log := logrus.WithFields(logrus.Fields{
		"containerID": containerID,
		"image":       fmt.Sprintf("%s:%s", req.ImageName, req.ImageTag),
		"name":        req.ContainerName,
	})

	log.Info("父行程: 準備啟動容器...")

	// 2. 建立容器的工作目錄
	containerDir := filepath.Join(config.ContainerStoragePath, containerID)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		return fmt.Errorf("建立容器目錄失敗: %w", err)
	}
	mountPoint := filepath.Join(containerDir, "rootfs")
	req.MountPoint = mountPoint

	// 3. 建立並寫入初始的 config.json
	info := &types.ContainerInfo{
		ID:         containerID,
		Name:       req.ContainerName,
		Command:    req.ContainerCommand,
		Status:     types.Created,
		CreatedAt:  time.Now(),
		Image:      fmt.Sprintf("%s:%s", req.ImageName, req.ImageTag),
		MountPoint: mountPoint,
		Limits:     req.ContainerLimits,
	}
	if err := pkg.WriteContainerInfo(containerDir, info); err != nil {
		return fmt.Errorf("寫入容器設定檔失敗: %w", err)
	}

	// 4. 建立匿名管道用於父子行程通信
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("建立管道失敗: %w", err)
	}
	defer readPipe.Close()

	// 5. 準備啟動子行程的命令
	cmd := exec.Command("/proc/self/exe", "init")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{readPipe}

	// 6. 啟動子行程
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("啟動子行程失敗: %w", err)
	}
	childPid := cmd.Process.Pid
	log.Infof("父行程: 子行程已啟動，PID 為 %d", childPid)

	// 7. 父行程為子行程設定外部環境
	// 7.1 設定 cgroup
	manager := container.NewManager()
	log.Info("父行程: 設定 cgroup 資源限制...")
	cgroupPath, err := manager.SetupCgroup(req.ContainerLimits, childPid, containerID)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("設定 cgroup 失敗: %w", err)
	}

	// 7.2 設定網路，並得到 peerName
	log.Info("父行程: 設定容器網路...")
	peerName, err := network.SetupVeth(childPid)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("設定網路失敗: %w", err)
	}
	req.VethPeerName = peerName

	// 8. 將 req 物件寫入管道，通知子行程繼續
	defer writePipe.Close()
	encoder := json.NewEncoder(writePipe)
	if err := encoder.Encode(req); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("父行程: 向管道寫入配置失敗: %v", err)
	}

	// 9. 更新 config.json，寫入 PID 並將狀態改為 Running
	info.PID = childPid
	info.Status = types.Running
	if err := pkg.WriteContainerInfo(containerDir, info); err != nil {
		log.Warnf("更新容器狀態為 Running 失敗: %v", err)
	}

	// 10. 等待容器行程結束
	log.Info("父行程: 等待容器行程結束...")
	if err := cmd.Wait(); err != nil {
		log.Warnf("父行程: 容器行程結束時發生錯誤: %v", err)
	}

	// 11. 容器結束後，再次更新狀態
	log.Info("父行程: 容器已退出，更新狀態為 Stopped")
	info.PID = 0 // 清理 PID
	info.Status = types.Stopped
	if err := pkg.WriteContainerInfo(containerDir, info); err != nil {
		log.Warnf("更新容器狀態為 Stopped 失敗: %v", err)
	}
	// 12. 清理 cgroup
	log.Info("父行程: 清理 cgroup...")
	_ = manager.CleanupCgroup(cgroupPath)

	return nil
}

// RunChildProcess 執行子行程的邏輯
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
	if err := container.SetupRootfs(req.MountPoint, req.ImageName, req.ImageTag); err != nil {
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
