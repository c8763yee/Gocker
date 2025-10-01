// internal/image/manager.go
package image

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gocker/internal/config"
	"gocker/internal/types"

	"github.com/google/go-containerregistry/pkg/crane"
)

// Manager 映像管理器
type Manager struct {
	storageDir string
}

// NewManager 建立新的映像管理器
func NewManager() *Manager {
	manager := &Manager{
		storageDir: config.ImagesDir,
	}

	// 確保儲存目錄存在
	os.MkdirAll(manager.storageDir, 0755)

	return manager
}

// Pull 拉取映像
func (m *Manager) PullImage(imageName string) error {
	// 使用 crane 拉取映像
	img, err := crane.Pull(imageName)
	if err != nil {
		return err
	}

	// 準備儲存路徑
	imageTarPath := filepath.Join(m.storageDir, m.sanitizeImageName(imageName)+".tar")

	// 建立目標檔案
	f, err := os.Create(imageTarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// 匯出映像
	if err := crane.Export(img, f); err != nil {
		return err
	}

	return nil
}

// List 列出本地映像
func (m *Manager) ListImages() ([]string, error) {
	files, err := os.ReadDir(m.storageDir)
	if err != nil {
		return nil, err
	}

	var images []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".tar") {
			// 移除 .tar 後綴並還原映像名稱
			imageName := strings.TrimSuffix(file.Name(), ".tar")
			imageName = strings.Replace(imageName, "_", ":", 1)
			images = append(images, imageName)
		}
	}

	return images, nil
}

// GetImagePath 取得映像的檔案路徑
func (m *Manager) GetImagePath(imageName string) string {
	return filepath.Join(m.storageDir, m.sanitizeImageName(imageName)+".tar")
}

// Exists 檢查映像是否存在
func (m *Manager) Exists(imageName string) bool {
	path := m.GetImagePath(imageName)
	_, err := os.Stat(path)
	return err == nil
}

// sanitizeImageName 清理映像名稱作為檔案名稱
func (m *Manager) sanitizeImageName(imageName string) string {
	return strings.Replace(imageName, ":", "_", 1)
}

// ListContainers 讀取所有容器的設定檔並回傳
func ListContainers() ([]*types.ContainerInfo, error) {
	files, err := os.ReadDir(config.ContainerStoragePath)
	if err != nil {
		return nil, fmt.Errorf("讀取容器儲存目錄失敗: %w", err)
	}

	var containers []*types.ContainerInfo
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		containerID := file.Name()
		configFilePath := filepath.Join(config.ContainerStoragePath, containerID, "config.json")

		data, err := os.ReadFile(configFilePath)
		if err != nil {
			// 如果 config.json 讀取失敗，就跳過這個目錄
			continue
		}

		var info types.ContainerInfo
		if err := json.Unmarshal(data, &info); err != nil {
			// 如果 json 解析失敗，也跳過
			continue
		}
		containers = append(containers, &info)
	}
	return containers, nil
}
