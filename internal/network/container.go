// internal/network/container.go
package network

import (
	"fmt"
	"net"
	"time"

	"gocker/internal/config"

	"github.com/vishvananda/netlink"
)

// ConfigureContainerNetwork 設定容器內的網路
func ConfigureContainerNetwork() {
	// 等待 veth peer 設定完成
	time.Sleep(200 * time.Millisecond)

	// 設定網路介面
	if err := setupNetworkInterface(); err != nil {
		fmt.Printf("警告: 設定網路介面失敗: %v\n", err)
	}

	// 啟動 loopback 介面
	if err := setupLoopback(); err != nil {
		fmt.Printf("警告: 設定 loopback 失敗: %v\n", err)
	}

	fmt.Println("容器內網路設定完成")
}

// setupNetworkInterface 設定容器的網路介面
func setupNetworkInterface() error {
	// 找到可用的網路介面 (除了 lo)
	targetLink, err := findNetworkInterface()
	if err != nil {
		return fmt.Errorf("找不到可用的網路介面: %v", err)
	}

	// 重新命名為 eth0
	if err := netlink.LinkSetName(targetLink, "eth0"); err != nil {
		return fmt.Errorf("重新命名網路介面失敗: %v", err)
	}

	// 重新獲取重新命名後的介面
	iface, err := netlink.LinkByName("eth0")
	if err != nil {
		return fmt.Errorf("找不到 eth0 介面: %v", err)
	}

	// 設定 IP 位址
	if err := setInterfaceIP(iface, config.ContainerIP); err != nil {
		return fmt.Errorf("設定介面 IP 失敗: %v", err)
	}

	// 啟動介面
	if err := netlink.LinkSetUp(iface); err != nil {
		return fmt.Errorf("啟動 eth0 失敗: %v", err)
	}

	// 設定預設路由
	if err := setDefaultRoute(config.GatewayIP); err != nil {
		return fmt.Errorf("設定預設路由失敗: %v", err)
	}

	return nil
}

// findNetworkInterface 找到可用的網路介面
func findNetworkInterface() (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	for _, link := range links {
		if link.Attrs().Name != "lo" {
			return link, nil
		}
	}

	return nil, fmt.Errorf("沒有找到可用的網路介面")
}

// setInterfaceIP 設定介面的 IP 位址
func setInterfaceIP(iface netlink.Link, ipAddr string) error {
	addr, err := netlink.ParseAddr(ipAddr)
	if err != nil {
		return err
	}

	return netlink.AddrAdd(iface, addr)
}

// setDefaultRoute 設定預設路由
func setDefaultRoute(gatewayIP string) error {
	route := &netlink.Route{
		Scope: netlink.SCOPE_UNIVERSE,
		Gw:    net.ParseIP(gatewayIP),
	}

	return netlink.RouteAdd(route)
}

// setupLoopback 設定 loopback 介面
func setupLoopback() error {
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("找不到 loopback 介面: %v", err)
	}

	return netlink.LinkSetUp(lo)
}
