// internal/container/manager.go
package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"gocker/internal/config"
	"gocker/internal/types"
	"gocker/pkg"
)

// Manager 負責所有容器的生命週期管理
type Manager struct {
	StoragePath string // 依賴的設定，容器儲存路徑
}

// NewManager 建立一個新的容器管理器
func NewManager() *Manager {
	return &Manager{
		StoragePath: config.ContainerStoragePath,
	}
}

func (m *Manager) StopContainer(identifier string) error {
	// 1. 查找容器資訊
	info, err := findContainerInfo(identifier)
	if err != nil {
		return err
	}

	// 2. 檢查容器狀態
	if info.Status != types.Running {
		return fmt.Errorf("容器 %s 不在運行狀態，目前狀態為: %s", identifier, info.Status)
	}

	pid := info.PID
	log := logrus.WithFields(logrus.Fields{
		"containerID": info.ID,
		"pid":         pid,
	})

	log.Infof("正在停止容器...")

	// 3. 發送 SIGTERM 信號
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if err.Error() != "no such process" {
			return fmt.Errorf("向容器行程 %d 發送 SIGTERM 信號失敗: %w", pid, err)
		}
		log.Warnf("行程 %d 已不存在，但仍將狀態標記為 Stopped", pid)
	}

	// 4. 更新容器狀態
	info.Status = types.Stopped
	info.PID = 0 // 清理 PID
	info.FinishedAt = time.Now()

	containerDir := filepath.Join(config.ContainerStoragePath, info.ID)
	if err := WriteContainerInfo(containerDir, info); err != nil {
		return fmt.Errorf("更新容器狀態為 Stopped 失敗: %w", err)
	}

	log.Infof("容器 %s 已成功停止", identifier)
	return nil
}
func findContainerInfo(identifier string) (*types.ContainerInfo, error) {
	files, err := os.ReadDir(config.ContainerStoragePath)
	if err != nil {
		return nil, fmt.Errorf("讀取容器儲存目錄失敗: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		containerID := file.Name()
		configPath := filepath.Join(config.ContainerStoragePath, containerID, "config.json")

		data, err := os.ReadFile(configPath)
		if err != nil {
			logrus.Warnf("讀取 config.json 失敗 (%s): %v", configPath, err)
			continue
		}

		var info types.ContainerInfo
		if err := json.Unmarshal(data, &info); err != nil {
			logrus.Warnf("解析 config.json 失敗 (%s): %v", configPath, err)
			continue
		}

		// 比對完整 ID -> 名稱 -> ID 前綴
		if info.ID == identifier || info.Name == identifier || (len(identifier) <= len(info.ID) && info.ID[:len(identifier)] == identifier) {
			return &info, nil
		}
	}

	return nil, fmt.Errorf("找不到容器: %s", identifier)
}

func (m *Manager) Start(identifier string) error {
	// 1. 查找容器並獲取其資訊
	info, err := findContainerInfo(identifier)
	if err != nil {
		return err
	}

	// 2. 檢查容器狀態
	if info.Status == types.Running {
		return fmt.Errorf("容器 %s 已經在運行中", identifier)
	}
	if info.Status != types.Stopped && info.Status != types.Created {
		return fmt.Errorf("無法啟動狀態為 %s 的容器", info.Status)
	}

	// 3. 準備重新啟動子行程
	log := logrus.WithField("containerID", info.ID)

	// 準備管道用於通信
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("建立管道失敗: %w", err)
	}
	defer readPipe.Close()

	// 準備命令
	selfPath, _ := os.Executable()
	cmd := exec.Command(selfPath, "init")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{readPipe}

	go func() {
		defer writePipe.Close()
		imageName, imageTag := pkg.Parse(info.Image)

		req := &types.RunRequest{
			ImageName:        imageName,
			ImageTag:         imageTag,
			ContainerCommand: info.Command,
			ContainerLimits:  info.Limits,
		}

		encoder := json.NewEncoder(writePipe)
		if err := encoder.Encode(req); err != nil {
			log.Errorf("向管道寫入配置失敗: %v", err)
		}
	}()

	// 4. 啟動子行程
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("啟動子行程失敗: %w", err)
	}
	newPid := cmd.Process.Pid
	log.Infof("容器已重新啟動，新的 PID 為 %d", newPid)

	// 5. 更新 config.json 狀態
	info.PID = newPid
	info.Status = types.Running
	info.FinishedAt = time.Time{} // 清除上次的結束時間
	containerDir := filepath.Join(m.StoragePath, info.ID)
	if err := WriteContainerInfo(containerDir, info); err != nil {
		log.Warnf("更新容器狀態為 Running 失敗: %v", err)
	}

	// 6. 設定資源限制
	cgroupPath, err := m.SetupCgroup(types.ContainerLimits{}, newPid)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("設定 cgroup 失敗: %w", err)
	}

	// 7. 等待容器結束
	_ = cmd.Wait()

	// 8. 容器再次停止後，更新最終狀態
	log.Info("容器已退出")
	info.PID = 0
	info.Status = types.Stopped
	info.FinishedAt = time.Now()
	_ = WriteContainerInfo(containerDir, info)

	// 9. 清理資源
	_ = m.CleanupCgroup(cgroupPath)

	return nil
}

// writeContainerInfo 將容器資訊寫回檔案
func WriteContainerInfo(containerDir string, info *types.ContainerInfo) error {
	configFilePath := filepath.Join(containerDir, "config.json")
	file, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("建立 config.json 失敗: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ") // 格式化 JSON
	return encoder.Encode(info)
}
