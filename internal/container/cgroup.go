package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gocker/internal/config"
	"gocker/internal/types"

	"github.com/sirupsen/logrus"
)

// SetupCgroup 由父行程呼叫，為指定的 PID 建立一個獨立的 cgroup 並設定資源限制
// 成功時回傳 cgroup 的路徑和 nil
func SetupCgroup(limits types.ContainerLimits, pid int) (string, error) {
	// 2. 為每個容器建立一個獨立的 cgroup 路徑，使用 PID 來確保唯一性
	containerCgroupPath := filepath.Join(config.CgroupRoot, config.CgroupName, strconv.Itoa(pid))
	log := logrus.WithField("cgroupPath", containerCgroupPath)

	log.Info("正在建立容器的 cgroup...")
	if err := os.MkdirAll(containerCgroupPath, 0755); err != nil {
		return "", fmt.Errorf("建立 cgroup 目錄失敗: %w", err)
	}

	// 啟用必要的控制器 (假設 enableControllers 在此套件中)
	// 這一步通常只需要在父 cgroup (/sys/fs/cgroup/gocker) 做一次
	// 此處省略以簡化
	// if err := enableControllers(); err != nil { ... }

	// 3. 將從 RunRequest 傳入的具體限制寫入檔案
	log.Info("正在設定容器的資源限制...")
	if err := setResourceLimits(containerCgroupPath, limits); err != nil {
		return "", fmt.Errorf("設定資源限制失敗: %w", err)
	}

	// 4. 使用傳入的子行程 PID，而不是 os.Getpid()
	procsPath := filepath.Join(containerCgroupPath, "cgroup.procs")
	log.Infof("正在將 PID %d 加入 cgroup...", pid)
	if err := os.WriteFile(procsPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return "", fmt.Errorf("將 PID 加入 cgroup.procs 失敗: %w", err)
	}

	log.Info("Cgroup 設定成功")
	// 5. 回傳建立的 cgroup 路徑，以便後續清理
	return containerCgroupPath, nil
}

func CleanupCgroup(cgroupPath string) error {
	logrus.Infof("TODO: Cleaning up cgroup at %s", cgroupPath)
	// 這裡未來要加入刪除 cgroup 目錄的邏輯
	return nil
}

func setResourceLimits(cgroupPath string, limits types.ContainerLimits) error {
	// 設定 CPU 限制
	if limits.CPULimit > 0 {
		cpuQuota := limits.CPULimit * 100000
		cpuPeriod := 100000
		cpuLimitString := fmt.Sprintf("%d %d", cpuQuota, cpuPeriod)
		cpuMaxPath := filepath.Join(cgroupPath, "cpu.max")
		if err := os.WriteFile(cpuMaxPath, []byte(cpuLimitString), 0644); err != nil {
			// logrus.Warnf("設定 CPU 限制失敗: %v", err)
			return fmt.Errorf("寫入 cpu.max 失敗: %w", err)
		}
	}

	// 設定記憶體限制
	if limits.MemoryLimit > 0 {
		memMaxPath := filepath.Join(cgroupPath, "memory.max")
		memLimitString := strconv.Itoa(limits.MemoryLimit)
		if err := os.WriteFile(memMaxPath, []byte(memLimitString), 0644); err != nil {
			// logrus.Warnf("設定記憶體限制失敗: %v", err)
			return fmt.Errorf("寫入 memory.max 失敗: %w", err)
		}
	}

	// 設定 PID 限制 (pids.max)
	if limits.PidsLimit > 0 {
		pidsMaxPath := filepath.Join(cgroupPath, "pids.max")
		pidsLimitString := strconv.Itoa(limits.PidsLimit)
		if err := os.WriteFile(pidsMaxPath, []byte(pidsLimitString), 0644); err != nil {
			// logrus.Warnf("設定 PID 限制失敗: %v", err)
			return fmt.Errorf("寫入 pids.max 失敗: %w", err)
		}
	}

	return nil
}
