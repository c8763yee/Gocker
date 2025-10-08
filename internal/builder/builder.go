// internal/builder/builder.go
package builder

import (
	"archive/tar"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"gocker/internal/config"
	"gocker/internal/image"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
)

// Builder 負責建構映像
type Builder struct {
	gockerfile *Gockerfile
	buildDir   string
	imageID    string
	imageName  string
	imageTag   string
}

// NewBuilder 建立新的 Builder
func NewBuilder(gockerfilePath, imageName, imageTag string) (*Builder, error) {
	gf, err := ParseGockerfile(gockerfilePath)
	if err != nil {
		return nil, err
	}

	// 產生映像 ID
	randBytes := make([]byte, 12)
	if _, err := rand.Read(randBytes); err != nil {
		return nil, fmt.Errorf("產生映像 ID 失敗: %w", err)
	}
	imageID := hex.EncodeToString(randBytes)

	return &Builder{
		gockerfile: gf,
		imageID:    imageID,
		imageName:  imageName,
		imageTag:   imageTag,
	}, nil
}

// Build 執行映像建構
func (b *Builder) Build() error {
	log := logrus.WithFields(logrus.Fields{
		"imageID":   b.imageID,
		"imageName": b.imageName,
		"imageTag":  b.imageTag,
	})

	log.Info("開始建構映像...")

	// 1. 確保基礎映像存在
	manager := image.NewManager()
	if !manager.Exists(b.gockerfile.BaseImage) {
		log.Infof("基礎映像 %s 不存在，開始拉取...", b.gockerfile.BaseImage)
		if err := manager.PullImage(b.gockerfile.BaseImage); err != nil {
			return fmt.Errorf("拉取基礎映像失敗: %w", err)
		}
	}

	// 2. 建立建構目錄
	b.buildDir = filepath.Join(config.ImagesDir, b.imageID)
	if err := os.MkdirAll(b.buildDir, 0755); err != nil {
		return fmt.Errorf("建立建構目錄失敗: %w", err)
	}

	// 3. 建立工作目錄並複製基礎映像
	workDir := filepath.Join(b.buildDir, "rootfs")
	if err := b.setupBaseImage(workDir); err != nil {
		return fmt.Errorf("設定基礎映像失敗: %w", err)
	}

	// 4. 執行 RUN 指令
	for i, runCmd := range b.gockerfile.RunCommands {
		log.Infof("執行 RUN 指令 [%d/%d]: %s", i+1, len(b.gockerfile.RunCommands), runCmd)
		if err := b.executeRun(workDir, runCmd); err != nil {
			return fmt.Errorf("執行 RUN 指令失敗: %w", err)
		}
	}

	// 5. 處理 COPY 指令
	for i, copyInst := range b.gockerfile.Copies {
		log.Infof("執行 COPY 指令 [%d/%d]: %s -> %s", i+1, len(b.gockerfile.Copies), copyInst.Source, copyInst.Dest)
		if err := b.executeCopy(workDir, copyInst); err != nil {
			return fmt.Errorf("執行 COPY 指令失敗: %w", err)
		}
	}

	// 6. 處理 ADD 指令
	for i, addInst := range b.gockerfile.Adds {
		log.Infof("執行 ADD 指令 [%d/%d]: %s -> %s", i+1, len(b.gockerfile.Adds), addInst.Source, addInst.Dest)
		if err := b.executeAdd(workDir, addInst); err != nil {
			return fmt.Errorf("執行 ADD 指令失敗: %w", err)
		}
	}

	// 7. 建立映像 tarball
	imageTarPath := filepath.Join(b.buildDir, "image.tar")
	log.Infof("正在建立映像 tarball: %s", imageTarPath)
	if err := b.createTarball(workDir, imageTarPath); err != nil {
		return fmt.Errorf("建立 tarball 失敗: %w", err)
	}

	// 8. 更新 manifest
	log.Info("正在更新 manifest.json...")
	repoTag := fmt.Sprintf("%s:%s", b.imageName, b.imageTag)
	if err := b.updateManifest(repoTag); err != nil {
		return fmt.Errorf("更新 manifest 失敗: %w", err)
	}

	// 9. 儲存映像元數據（CMD、ENTRYPOINT 等）
	if err := b.saveImageMetadata(); err != nil {
		return fmt.Errorf("儲存映像元數據失敗: %w", err)
	}

	log.Info("映像建構完成")
	return nil
}

// setupBaseImage 設定基礎映像
func (b *Builder) setupBaseImage(workDir string) error {
	// 找到基礎映像的 imageID
	manifestPath := filepath.Join(config.ImagesDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("讀取 manifest.json 失敗: %w", err)
	}

	var manifests []types.ImageManifest
	if err := json.Unmarshal(data, &manifests); err != nil {
		return fmt.Errorf("解析 manifest.json 失敗: %w", err)
	}

	var baseImageID string
	for _, m := range manifests {
		if m.RepoTag == b.gockerfile.BaseImage {
			baseImageID = m.ImageID
			break
		}
	}

	if baseImageID == "" {
		return fmt.Errorf("找不到基礎映像 %s", b.gockerfile.BaseImage)
	}

	// 複製基礎映像的 rootfs
	baseRootfs := filepath.Join(config.ImagesDir, baseImageID, "rootfs")
	if err := copyDir(baseRootfs, workDir); err != nil {
		return fmt.Errorf("複製基礎映像失敗: %w", err)
	}

	return nil
}

// executeRun 在 chroot 環境中執行命令
func (b *Builder) executeRun(workDir, command string) error {
	// 使用 chroot 執行命令
	cmd := exec.Command("chroot", workDir, "/bin/sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 掛載必要的檔案系統
	procPath := filepath.Join(workDir, "proc")
	sysPath := filepath.Join(workDir, "sys")

	os.MkdirAll(procPath, 0755)
	os.MkdirAll(sysPath, 0755)

	// 掛載 proc 和 sys
	syscall.Mount("proc", procPath, "proc", 0, "")
	syscall.Mount("sysfs", sysPath, "sysfs", 0, "")

	defer func() {
		syscall.Unmount(procPath, 0)
		syscall.Unmount(sysPath, 0)
	}()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("執行命令 '%s' 失敗: %w", command, err)
	}

	return nil
}

// executeCopy 執行 COPY 指令
func (b *Builder) executeCopy(workDir string, copyInst CopyInstruction) error {
	srcPath := copyInst.Source
	destPath := filepath.Join(workDir, copyInst.Dest)

	return copyFile(srcPath, destPath)
}

// executeAdd 執行 ADD 指令
func (b *Builder) executeAdd(workDir string, addInst AddInstruction) error {
	// ADD 與 COPY 類似，但支援 URL 和自動解壓縮
	// 簡化實現，這裡只實現基本的檔案複製
	srcPath := addInst.Source
	destPath := filepath.Join(workDir, addInst.Dest)

	return copyFile(srcPath, destPath)
}

// createTarball 建立 tarball
func (b *Builder) createTarball(sourceDir, tarPath string) error {
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	tarWriter := tar.NewWriter(tarFile)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})
}

// updateManifest 更新 manifest.json
func (b *Builder) updateManifest(repoTag string) error {
	manifestPath := filepath.Join(config.ImagesDir, "manifest.json")

	var manifests []types.ImageManifest
	data, err := os.ReadFile(manifestPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &manifests); err != nil {
			return err
		}
	}

	// 檢查是否已存在
	found := false
	for i := range manifests {
		if manifests[i].RepoTag == repoTag {
			manifests[i].ImageID = b.imageID
			found = true
			break
		}
	}

	if !found {
		manifests = append(manifests, types.ImageManifest{
			RepoTag: repoTag,
			ImageID: b.imageID,
		})
	}

	newData, err := json.MarshalIndent(manifests, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(manifestPath, newData, 0644)
}

// saveImageMetadata 儲存映像元數據
func (b *Builder) saveImageMetadata() error {
	metadata := map[string]interface{}{
		"cmd":        b.gockerfile.Cmd,
		"entrypoint": b.gockerfile.Entrypoint,
	}

	metadataPath := filepath.Join(b.buildDir, "metadata.json")
	data, err := json.MarshalIndent(metadata, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(metadataPath, data, 0644)
}

// 輔助函數：複製目錄
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

// 輔助函數：複製檔案
func copyFile(src, dst string) error {
	// 確保目標目錄存在
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// 複製檔案權限
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}
