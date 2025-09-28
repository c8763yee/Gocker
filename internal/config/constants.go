// internal/config/constants.go
package config

const (
	// 儲存路徑
	GockerStorage = "/var/lib/gocker"
	ImagesDir     = GockerStorage + "/images"
	ContainersDir = GockerStorage + "/containers"

	// 網路設定
	BridgeName  = "gocker0"
	BridgeIP    = "10.20.0.1/24"
	NetworkCIDR = "10.20.0.0/24"
	ContainerIP = "10.20.0.2/24"
	GatewayIP   = "10.20.0.1"

	// DNS 設定
	DNSServers = `nameserver 8.8.8.8
	              nameserver 1.1.1.1
	              nameserver 8.8.4.4
				 `

	// Cgroup 設定
	CgroupRoot = "/sys/fs/cgroup"
	CgroupName = "gocker"

	// 資源限制 (可在未來改為可配置)
	CPULimit    = "50000 100000" // 50% CPU
	MemoryLimit = "100M"         // 100MB Memory
)
