// internal/container/rootfs.go
package container

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// PivotRoot 切換根目錄
func PivotRoot(newRoot string) {
	// 將根目錄的掛載傳播類型設為 private
	if err := syscall.Mount("", "/", "", syscall.MS_PRIVATE|syscall.MS_REC, ""); err != nil {
		panic(fmt.Sprintf("將根掛載設為 private 失敗: %v", err))
	}

	putold := filepath.Join(newRoot, ".old_root")

	// 確保 newRoot 是掛載點
	if err := syscall.Mount(newRoot, newRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		panic(fmt.Sprintf("重新掛載 newRoot 失敗: %v", err))
	}

	// 建立 .old_root 目錄
	if err := os.MkdirAll(putold, 0700); err != nil {
		panic(fmt.Sprintf("建立 .old_root 失敗: %v", err))
	}

	// 執行 pivot_root
	if err := syscall.PivotRoot(newRoot, putold); err != nil {
		panic(fmt.Sprintf("pivot_root 失敗: %v", err))
	}

	// 切換到新的根目錄
	if err := os.Chdir("/"); err != nil {
		panic(fmt.Sprintf("chdir 到 / 失敗: %v", err))
	}

	// 卸載並移除舊的根目錄
	if err := syscall.Unmount("/.old_root", syscall.MNT_DETACH); err != nil {
		panic(fmt.Sprintf("卸載 .old_root 失敗: %v", err))
	}

	if err := os.RemoveAll("/.old_root"); err != nil {
		fmt.Printf("警告: 移除 .old_root 失敗: %v\n", err)
	}
}
