#!/bin/bash
set -e

SERVICE_FILE="gocker-daemon.service"
INSTALL_PATH="/usr/local/bin"
BINS=("gocker" "gocker-daemon" "sched_monitor")
STORAGE_PATH="/var/lib/gocker"

if [ "$(id -u)" -ne 0 ]; then
    echo "錯誤：此腳本需要 root 權限來執行。請使用 'sudo'。"
    exit 1
fi

echo "--- Gocker 卸載程序 ---"

echo "--> 正在停止並禁用 gocker-daemon 服務..."
systemctl stop $SERVICE_FILE || true
systemctl disable $SERVICE_FILE || true

echo "--> 正在移除 systemd 服務檔案..."
rm -f "/etc/systemd/system/$SERVICE_FILE"
systemctl daemon-reload

echo "--> 正在移除執行檔..."
for bin in "${BINS[@]}"; do
    rm -f "$INSTALL_PATH/$bin"
done

echo "--> 正在移除儲存目錄 $STORAGE_PATH..."
# read -p "確定要刪除所有 Gocker 容器數據嗎？[y/N] " -n 1 -r
# echo
# if [[ $REPLY =~ ^[Yy]$ ]]; then
#     rm -rf "$STORAGE_PATH"
#     echo "儲存目錄已移除。"
# else
#     echo "保留儲存目錄。"
# fi
echo "保留儲存目錄 $STORAGE_PATH。如果需要，請手動刪除。"


echo "✅ Gocker 卸載完成。"