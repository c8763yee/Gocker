package internal

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"gocker/internal/config"
	"gocker/internal/container"
	"gocker/internal/network"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
)

/*
RunContainer 負責完整的父行程邏輯：建立容器元數據、設定資源並啟動子行程
*/
func RunContainer(req *types.RunRequest) error {
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

	// 3. 建立並寫入初始的 config.json
	info := &types.ContainerInfo{
		ID:         containerID,
		Name:       req.ContainerName,
		Command:    req.ContainerCommand,
		Status:     types.Created,
		CreatedAt:  time.Now(),
		Image:      fmt.Sprintf("%s:%s", req.ImageName, req.ImageTag),
		MountPoint: filepath.Join(containerDir, "rootfs"), // 預先定義掛載點路徑
	}
	if err := container.WriteContainerInfo(containerDir, info); err != nil {
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

	// 6. 使用 goroutine 將完整的請求資訊寫入管道
	go func() {
		defer writePipe.Close()
		encoder := json.NewEncoder(writePipe)
		if err := encoder.Encode(req); err != nil {
			log.Errorf("父行程: 向管道寫入配置失敗: %v", err)
		}
	}()

	// 7. 啟動子行程
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("啟動子行程失敗: %w", err)
	}
	childPid := cmd.Process.Pid
	log.Infof("父行程: 子行程已啟動，PID 為 %d", childPid)

	// 8. 更新 config.json，寫入 PID 並將狀態改為 Running
	info.PID = childPid
	info.Status = types.Running
	if err := container.WriteContainerInfo(containerDir, info); err != nil {
		log.Warnf("更新容器狀態為 Running 失敗: %v", err)
	}
	manager := container.NewManager()

	// 9. 從外部為子行程設定資源
	log.Info("父行程: 設定 cgroup 資源限制...")
	cgroupPath, err := manager.SetupCgroup(req.ContainerLimits, childPid)
	if err != nil {
		// 如果設定失敗，殺死子行程並回傳錯誤
		_ = cmd.Process.Kill()
		return fmt.Errorf("設定 cgroup 失敗: %w", err)
	}

	log.Info("父行程: 設定容器網路...")
	if err := network.SetupVeth(childPid); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("設定網路失敗: %w", err)
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
	if err := container.WriteContainerInfo(containerDir, info); err != nil {
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
	if err := container.SetupRootfs(req.ImageName, req.ImageTag); err != nil {
		return fmt.Errorf("子行程: 設定 rootfs 失敗: %w", err)
	}
	log.Info("子行程: Rootfs 掛載成功")

	// 4. 掛載必要的核心虛擬檔案系統
	syscall.Mount("proc", "/proc", "proc", 0, "")
	syscall.Mount("sysfs", "/sys", "sysfs", 0, "")

	// 5. 在容器內部設定網路
	if err := network.ConfigureContainerNetwork(); err != nil {
		return fmt.Errorf("子行程: 設定容器網路失敗: %w", err)
	}
	log.Info("子行程: 容器內網路設定完成")

	// 6. 使用 syscall.Exec 執行使用者指定的命令
	cmdPath, err := exec.LookPath(req.ContainerCommand)
	if err != nil {
		return fmt.Errorf("子行程: 找不到命令 '%s': %w", req.ContainerCommand, err)
	}

	log.Infof("子行程: 執行 exec syscall: %s", cmdPath)
	args := append([]string{req.ContainerCommand}, req.ContainerArgs...)
	if err := syscall.Exec(cmdPath, args, os.Environ()); err != nil {
		return fmt.Errorf("子行程: exec 失敗: %w", err)
	}

	return nil
}
