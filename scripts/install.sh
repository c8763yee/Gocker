#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
INSTALL_PATH="/usr/local/bin"
STORAGE_PATH="/var/lib/gocker"
SERVICE_FILE="gocker-daemon.service"
SCRIPTS_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPTS_DIR")"
SERVICE_FILE_PATH="$SCRIPTS_DIR/$SERVICE_FILE"

CLI_BIN="gocker"
DAEMON_BIN="gocker-daemon"
EBPF_BIN="ebpf-sched-monitor"

# --- Check for root privileges ---
if [ "$(id -u)" -ne 0 ]; then
    echo "錯誤：此腳本需要 root 權限來執行。請使用 'sudo'。"
    exit 1
fi

echo "--- Gocker 安裝程序 ---"

# --- 1. 停止現有服務 ---
echo "--> 正在停止現有的 gocker-daemon 服務 (如果存在)..."
systemctl stop $SERVICE_FILE || true

# --- 2. 複製執行檔 ---
echo "--> 正在複製執行檔到 $INSTALL_PATH..."
install -m 0755 "$PROJECT_ROOT/$CLI_BIN" "$INSTALL_PATH/"
install -m 0755 "$PROJECT_ROOT/$DAEMON_BIN" "$INSTALL_PATH/"
install -m 0755 "$PROJECT_ROOT/eBPF/$EBPF_BIN" "$INSTALL_PATH/"

# --- 3. 建立儲存目錄 ---
echo "--> 正在建立儲存目錄 $STORAGE_PATH..."
mkdir -p "$STORAGE_PATH/containers"

# --- 4. 安裝 systemd 服務檔案 ---
if [ ! -f "$SERVICE_FILE_PATH" ]; then
    echo "錯誤：找不到服務檔案 $SERVICE_FILE_PATH"
    exit 1
fi
echo "--> 正在安裝 systemd 服務..."
cp "$SERVICE_FILE_PATH" /etc/systemd/system/

# --- 5. 重載、啟用並啟動服務 ---
echo "--> 正在重載 systemd 並啟動 gocker-daemon..."
systemctl daemon-reload
systemctl enable $SERVICE_FILE
systemctl start $SERVICE_FILE

# --- 6. 驗證服務狀態 ---
sleep 2
if systemctl is-active --quiet $SERVICE_FILE; then
    echo "✅ Gocker Daemon 正在運行！"
else
    echo "❌ Gocker Daemon 啟動失敗。請執行 'sudo journalctl -u $SERVICE_FILE' 查看日誌。"
    exit 1
fi

echo ""
echo "Gocker 安裝完成！"