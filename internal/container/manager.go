// internal/container/manager.go
package container

import (
	"encoding/json"
	"fmt"
	"gocker/internal/config"
	"gocker/internal/types"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// Manager 負責所有容器的生命週期管理
type Manager struct {
	StoragePath string // 依賴的設定，例如容器儲存路徑
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
	// syscall.Kill 可以發送任何信號給指定的 PID
	// SIGTERM 是一個標準的、溫和的終止信號
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		// 如果行程已經不存在，也會回傳錯誤，但我們的目標是停止它，所以可以視為成功
		// "no such process" 是一個可以接受的錯誤
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

// writeContainerInfo 是一個輔助函式，用來將容器資訊寫回檔案
func WriteContainerInfo(containerDir string, info *types.ContainerInfo) error {
	configFilePath := filepath.Join(containerDir, "config.json")
	file, err := os.Create(configFilePath)
	if err != nil {
		return fmt.Errorf("建立 config.json 失敗: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ") // 格式化 JSON，方便閱讀
	return encoder.Encode(info)
}
