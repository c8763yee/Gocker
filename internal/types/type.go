package types

import "time"

type RunRequest struct {
	ImageName        string
	ImageTag         string
	ContainerName    string
	ContainerCommand string
	ContainerID      string
	ContainerArgs    []string
	ContainerLimits
}

type ContainerLimits struct {
	MemoryLimit int
	PidsLimit   int
	CPULimit    int
}

// ContainerStatus 容器的狀態
const (
	Running = "running"
	Stopped = "stopped"
	Created = "created"
)

// ContainerInfo 用於儲存容器的元數據
type ContainerInfo struct {
	ID         string    `json:"id"`
	PID        int       `json:"pid"`
	Name       string    `json:"name"`
	Command    string    `json:"command"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	Image      string    `json:"image"`
	MountPoint string    `json:"mountPoint"`
}

type Info struct {
	Repository string
	Tag        string
	ID         string
	Size       string // 或是 int64 型別，然後再格式化
}
