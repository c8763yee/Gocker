package types

import "time"

type RunRequest struct {
	ImageName        string
	ImageTag         string
	ContainerName    string
	ContainerCommand string
	ContainerID      string
	ContainerArgs    []string
	MountPoint       string
	VethPeerName     string
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

// ContainerInfo 用於儲存容器的metadata
type ContainerInfo struct {
	ID         string          `json:"id"`
	PID        int             `json:"pid"`
	Name       string          `json:"name"`
	Command    string          `json:"command"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"createdAt"`
	Image      string          `json:"image"`
	MountPoint string          `json:"mountPoint"`
	FinishedAt time.Time       `json:"finishedAt,omitempty"`
	Limits     ContainerLimits `json:"limits,omitempty"`
}

// ImageManifest Image 的結構
type ImageManifest struct {
	ImageID string `json:"imageID"` // 映像的唯一 ID
	RepoTag string `json:"repoTag"` // 映像的標籤
}
