// internal/network/dns.go
package network

import (
	"fmt"
	"os"

	"gocker/internal/config"
)

// SetupDNS 設定容器的 DNS
func SetupDNS() {
	if err := os.WriteFile("/etc/resolv.conf", []byte(config.DNSServers), 0644); err != nil {
		fmt.Printf("警告: 設定 DNS 失敗: %v\n", err)
		return
	}

	fmt.Println("DNS 設定完成")
}
