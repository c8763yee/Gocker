// internal/network/veth.go
package network

import (
	"fmt"

	"gocker/internal/config"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

// SetupContainerNetwork 為容器設定網路
func SetupContainerNetwork(childPid int) error {
	vethName := fmt.Sprintf("veth-%d", childPid)
	peerName := fmt.Sprintf("peer-%d", childPid)

	// 建立 veth pair
	if err := createVethPair(vethName, peerName); err != nil {
		return fmt.Errorf("建立 veth pair 失敗: %v", err)
	}

	// 連接主機端 veth 到網橋
	if err := connectVethToBridge(vethName); err != nil {
		return fmt.Errorf("連接 veth 到網橋失敗: %v", err)
	}

	// 將容器端 veth 移入容器的網路 namespace
	if err := moveVethToContainer(peerName, childPid); err != nil {
		return fmt.Errorf("移動 veth 到容器失敗: %v", err)
	}

	return nil
}

// createVethPair 建立 veth pair
func createVethPair(vethName, peerName string) error {
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: vethName, MTU: 1500},
		PeerName:  peerName,
	}

	return netlink.LinkAdd(veth)
}

// connectVethToBridge 將主機端 veth 連接到網橋
func connectVethToBridge(vethName string) error {
	// 獲取網橋
	bridge, err := netlink.LinkByName(config.BridgeName)
	if err != nil {
		return fmt.Errorf("找不到網橋: %v", err)
	}

	// 獲取主機端 veth
	hostVeth, err := netlink.LinkByName(vethName)
	if err != nil {
		return fmt.Errorf("找不到主機端 veth: %v", err)
	}

	// 將 veth 連接到網橋
	if err := netlink.LinkSetMaster(hostVeth, bridge); err != nil {
		return fmt.Errorf("設定 veth master 失敗: %v", err)
	}

	// 啟動主機端 veth
	if err := netlink.LinkSetUp(hostVeth); err != nil {
		return fmt.Errorf("啟動主機端 veth 失敗: %v", err)
	}

	return nil
}

// moveVethToContainer 將容器端 veth 移入容器的網路 namespace
func moveVethToContainer(peerName string, childPid int) error {
	peer, err := netlink.LinkByName(peerName)
	if err != nil {
		return fmt.Errorf("找不到容器端 veth: %v", err)
	}

	return netlink.LinkSetNsPid(peer, childPid)
}

func SetupVeth(pid int) error {
	logrus.Infof("TODO: Setting up veth for PID %d", pid)
	// 這裡未來要加入建立 veth、設定 bridge、將一端移入 network namespace 的邏輯
	return nil // 暫時回傳 nil 讓編譯通過
}
