package main

import (
	"log"
	"os"

	"gocker/internal/container"
	"gocker/internal/daemon"
	"gocker/internal/image" // 為未來預留
)

func main() {
	// 關鍵：檢查程式是如何被呼叫的
	if len(os.Args) > 1 && os.Args[1] == "init" {
		// 如果是以 "init" 參數啟動，則執行子行程的初始化邏輯
		// 這是容器的「創世紀」
		log.Println("--- Daemon in init mode ---")
		if err := container.InitContainer(); err != nil {
			log.Fatalf("子行程初始化失敗: %v", err)
		}
		// InitContainer 內部會呼叫 syscall.Exec，永遠不會返回
		return
	}

	// 如果沒有 "init" 參數，則正常啟動 Daemon 服務
	log.Println("--- Daemon in server mode ---")

	// 1. 建立所有業務邏輯的管理器 (依賴)
	containerManager := container.NewManager()
	imageManager := image.NewManager() // 為未來預留

	// 2. 將管理器實例「注入」到 Daemon Server 中
	server := daemon.NewServer(containerManager, imageManager)

	// 3. 啟動 Daemon 伺服器的主迴圈
	if err := server.Run(); err != nil {
		log.Fatalf("Daemon 啟動失敗: %v", err)
	}
}
