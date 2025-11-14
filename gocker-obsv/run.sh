#!/usr/bin/env bash
set -euo pipefail

# === 先決條件檢查 ===
if ! mount | grep -q "on /sys/fs/cgroup type cgroup2"; then
  echo "[ERR] cgroup v2 未掛載在 /sys/fs/cgroup"; exit 1
fi
test -e /sys/kernel/btf/vmlinux || { echo "[ERR] 缺少 BTF：/sys/kernel/btf/vmlinux"; exit 1; }
command -v bpftool >/dev/null || { echo "[ERR] 需要 bpftool"; exit 1; }
command -v clang >/dev/null || { echo "[ERR] 需要 clang"; exit 1; }

# === 編譯 eBPF ===
echo "[*] Build eBPF objects..."
make -C bpf clean && make -C bpf

# === 編譯 exporter / ctl ===
echo "[*] Build exporter..."
pushd cmd/collector >/dev/null
go mod tidy
go build -o ../../collector .
popd >/dev/null

echo "[*] Build ctl (optional)..."
pushd cmd/ctl >/dev/null
go mod tidy || true
go build -o ../../ctl || true
popd >/dev/null

# === 啟動 exporter ===
echo "[*] Start exporter (sudo)..."
sudo PF_TARGET_CGROUP=/sys/fs/cgroup/gocker PF_SAMPLE_RATE=1 ./collector &
sleep 1
curl -s http://127.0.0.1:2112/metrics | head || true
echo "[OK] /metrics ready at :2112"

# === 啟動 Prometheus（你需要自備 Prometheus binary 並放到 PATH 或同目錄）===
if ! command -v prometheus >/dev/null; then
  echo "[WARN] 找不到 prometheus，可到 https://prometheus.io 下載；"
  echo "      也可自行手啟：prometheus --config.file=prometheus.yml --web.enable-lifecycle"
else
  echo "[*] Start Prometheus on :9090 ..."
  prometheus --config.file=prometheus.yml --web.enable-lifecycle >/tmp/prometheus.log 2>&1 &
  sleep 2
  echo "[OK] Prometheus http://localhost:9090"
fi
