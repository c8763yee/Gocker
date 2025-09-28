// internal/container/cgroup.go
package container

import (
	"fmt"
	"gocker/internal/config"
	"os"
	"path/filepath"
	"strconv"
)

// SetupCgroup 設定 Cgroup 資源限制
func SetupCgroup() {
	gockerCgroupPath := filepath.Join(config.CgroupRoot, config.CgroupName)

	// 建立 gocker cgroup 目錄
	if err := os.MkdirAll(gockerCgroupPath, 0755); err != nil {
		fmt.Printf("警告: 建立 cgroup 目錄失敗: %v\n", err)
		return
	}

	// 啟用控制器
	if err := enableControllers(); err != nil {
		fmt.Printf("警告: 啟用 cgroup 控制器失敗: %v\n", err)
	}

	// 設定資源限制
	setResourceLimits(gockerCgroupPath)

	// 將目前程序加入 cgroup
	pid := os.Getpid()
	procsPath := filepath.Join(gockerCgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		panic(fmt.Sprintf("將 PID 加入 cgroup 失敗: %v", err))
	}

	fmt.Printf("子行程 PID %d 已加入 gocker cgroup\n", pid)
}

// enableControllers 啟用 cgroup 控制器
func enableControllers() error {
	controllerPath := filepath.Join(config.CgroupRoot, "cgroup.subtree_control")
	return os.WriteFile(controllerPath, []byte("+cpu +memory"), 0644)
}

// setResourceLimits 設定資源限制
func setResourceLimits(cgroupPath string) {
	// 設定 CPU 限制
	cpuMaxPath := filepath.Join(cgroupPath, "cpu.max")
	if err := os.WriteFile(cpuMaxPath, []byte(config.CPULimit), 0644); err != nil {
		fmt.Printf("警告: 設定 CPU 限制失敗: %v\n", err)
	}

	// 設定記憶體限制
	memMaxPath := filepath.Join(cgroupPath, "memory.max")
	if err := os.WriteFile(memMaxPath, []byte(config.MemoryLimit), 0644); err != nil {
		fmt.Printf("警告: 設定記憶體限制失敗: %v\n", err)
	}
}
