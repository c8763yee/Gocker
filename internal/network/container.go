// internal/network/container.go
package network

import (
	"fmt"
	"net"

	"gocker/internal/config"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// ConfigureContainerNetwork 設定容器內的網路
func ConfigureContainerNetwork(peerName string) error {
	// 1. 找到容器內的 veth peer
	peer, err := netlink.LinkByName(peerName)
	if err != nil {
		return fmt.Errorf("在容器內找不到 veth peer '%s': %v", peerName, err)
	}

	// 2. 將 veth peer 重新命名為 eth0，這是容器內網卡的標準名稱
	if err := netlink.LinkSetName(peer, "eth0"); err != nil {
		return fmt.Errorf("重新命名 veth peer 為 eth0 失敗: %v", err)
	}

	// 3. 為 eth0 設定 IP 位址
	addr, err := netlink.ParseAddr(config.ContainerIP)
	if err != nil {
		return fmt.Errorf("解析容器 IP 位址 '%s' 失敗: %v", config.ContainerIP, err)
	}
	if err := netlink.AddrAdd(peer, addr); err != nil {
		return fmt.Errorf("為 eth0 設定 IP 失敗: %v", err)
	}

	// 4. 啟動 eth0 網卡
	if err := netlink.LinkSetUp(peer); err != nil {
		return fmt.Errorf("啟動 eth0 失敗: %v", err)
	}

	// 5. 設定預設路由，將所有流量指向網橋的 IP (Gateway)
	gatewayIP := net.ParseIP(config.GatewayIP)
	if gatewayIP == nil {
		return fmt.Errorf("解析閘道 IP '%s' 失敗", config.GatewayIP)
	}
	route := &netlink.Route{
		Scope: netlink.SCOPE_UNIVERSE,
		Gw:    gatewayIP,
	}
	if err := netlink.RouteAdd(route); err != nil {
		return fmt.Errorf("設定預設路由失敗: %v", err)
	}

	// (可選但推薦) 啟動 lo 本地環回介面
	lo, _ := netlink.LinkByName("lo")
	_ = netlink.LinkSetUp(lo)

	logrus.Infof("容器內網路設定完成，IP: %s", config.ContainerIP)
	return nil
}

/*
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

*/
