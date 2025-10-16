# 操作步驟（Host-only）
A. 安裝先決套件（以 Debian/Ubuntu 為例）
sudo apt-get install -y clang llvm bpftool curl

B. 準備並啟動
bash run.sh

C. 驗證 Exporter
curl -s localhost:2112/metrics | egrep 'page_faults_total|sched_events_total|syscall_.*_total' | head


D. 啟動 Grafana 並匯入 Dashboard
grafana-server web
# 瀏覽 http://localhost:3000（預設帳密 admin/admin）
# → “+” → Import → 選 grafana-dashboard.json → 選取「Prometheus」資料源

F. 熱更新參數（可選）
# 把 sample_rate 改 10（降低事件量）
curl -XPOST -H 'Content-Type: application/json' \
  -d '{"sample_rate":10}' http://127.0.0.1:2112/admin/config
# 或用 CLI
./ctl --sample 10