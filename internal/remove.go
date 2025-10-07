// gocker/internal/remove.go
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gocker/internal/config"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
)

// RemoveContainer 根據容器 ID 執行解除掛載和刪除操作
func RemoveContainer(containerID string) error {
	// 1. 直接定位到指定容器的目錄
	containerDir := filepath.Join(config.ContainerStoragePath, containerID)
	configFilePath := filepath.Join(containerDir, "config.json")

	// 2. 讀取該容器的設定檔
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return fmt.Errorf("找不到容器 %s 的設定檔: %w", containerID, err)
	}

	var info types.ContainerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("解析容器 %s 的設定檔失敗: %w", containerID, err)
	}

	// 3. 安全檢查：不允許刪除正在運行的容器
	if info.Status == types.Running {
		return fmt.Errorf("無法刪除正在運行的容器 %s，請先停止它", containerID)
	}

	// 4. 解除掛載 (Unmount)
	if info.MountPoint != "" {
		logrus.Infof("正在解除掛載 %s", info.MountPoint)
		if err := syscall.Unmount(info.MountPoint, 0); err != nil {
			logrus.Warnf("解除掛載 %s 失敗: %v", info.MountPoint, err)
		}
	}

	// 5. 刪除cgroup
	cgroupPath := filepath.Join(config.CgroupRoot, config.CgroupName, info.ID)
	logrus.Infof("正在清理 Cgroup %s", cgroupPath)

	if err := os.RemoveAll(cgroupPath); err != nil {
		return fmt.Errorf("移除 cgroup 目錄 %s 失敗: %w", cgroupPath, err)
	}
	logrus.Info("成功清理 cgroup")

	// 6. 刪除整個容器目錄
	logrus.Infof("正在刪除容器目錄 %s", containerDir)
	if err := os.RemoveAll(containerDir); err != nil {
		return fmt.Errorf("刪除容器目錄 %s 失敗: %w", containerDir, err)
	}

	logrus.Infof("成功刪除容器 %s", containerID)
	return nil
}
