// cmd/pf_exporter/main.go
// go 1.22+
// 需要 root 或 CAP_BPF+CAP_PERFMON+CAP_SYS_ADMIN（最簡單 sudo 跑）
// 功能：
// 1️⃣ 載入 pf.bpf.o (CO-RE eBPF 程式)
// 2️⃣ 將 page_fault_user/kernel tracepoint 事件送進 ringbuf
// 3️⃣ 讀取 ringbuf，轉成 Prometheus counter 暴露於 :2112/metrics
// 4️⃣ 只監控 cgroup /sys/fs/cgroup/gocker 子樹內的事件

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	cgroupRootDefault = "/sys/fs/cgroup"        // cgroup v2 的 root mount point
	gockerPathDefault = "/sys/fs/cgroup/gocker" // 你自定義的 gocker cgroup 根
	bpfObjDefault     = "bpf/pf.bpf.o"          // eBPF ELF 檔位置，可由環境變數 BPF_OBJ 覆寫
)

type event struct {
	TsNS     uint64   // 時間戳 (ns)
	Type     uint32   // 事件類型 (1=user, 2=kernel)
	Pid      uint32   // pid
	Tgid     uint32   // tgid
	CgroupID uint64   // 所屬 cgroup id
	Comm     [16]byte // 程式名稱(comm)
}

// Prometheus Counter
var (
	pageFaults = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "page_faults_total",
			Help: "Aggregated page fault events from eBPF (sampled).",
		},
		[]string{"type", "cgroup_id"},
	)
)

// 檢查是否為 cgroup v2
func isUnifiedCgroupV2(root string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return false
	}
	const CGROUP2_SUPER_MAGIC = 0x63677270
	return st.Type == CGROUP2_SUPER_MAGIC
}

// 取得指定 cgroup 目錄的 inode（作為 cgroup_id）
func cgroupInode(path string) (uint64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat conversion failed")
	}
	return uint64(st.Ino), nil
}

// 算出目標 cgroup 距離 root 的層級深度（給 bpf_get_current_ancestor_cgroup_id 使用）
func cgroupLevel(root, path string) (uint32, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0, err
	}
	if rel == "." {
		return 0, nil
	}
	segs := strings.Split(rel, string(filepath.Separator))
	n := 0
	for _, s := range segs {
		if s != "" {
			n++
		}
	}
	return uint32(n), nil
}

// 將事件類型轉為字串 label
func labelType(t uint32) string {
	switch t {
	case 1:
		return "user"
	case 2:
		return "kernel"
	default:
		return "unknown"
	}
}

func main() {
	// --- 初始化 Prometheus counter ---
	prometheus.MustRegister(pageFaults)

	// --- 讀取環境變數（若未指定就使用預設值） ---
	cgroupRoot := getenv("CGROUP_ROOT", cgroupRootDefault)
	targetPath := getenv("PF_TARGET_CGROUP", gockerPathDefault)
	bpfObjPath := getenv("BPF_OBJ", bpfObjDefault)
	sampleRate := getenvUint("PF_SAMPLE_RATE", 1)

	// --- 檢查 cgroup v2 環境 ---
	if !isUnifiedCgroupV2(cgroupRoot) {
		log.Fatalf("require cgroup v2 unified mount at %s", cgroupRoot)
	}

	// --- 取得 gocker 的 inode (cgroup_id) + 層級 ---
	targetID, err := cgroupInode(targetPath)
	if err != nil {
		log.Fatalf("stat %s: %v", targetPath, err)
	}
	level, err := cgroupLevel(cgroupRoot, targetPath)
	if err != nil {
		log.Fatalf("compute level for %s: %v", targetPath, err)
	}
	log.Printf("Filter subtree: %s (inode=%d, level=%d), sample_rate=%d",
		targetPath, targetID, level, sampleRate)

	// --- 載入 eBPF ELF 物件檔 ---
	bpfBytes, err := os.ReadFile(bpfObjPath)
	if err != nil {
		log.Fatalf("read BPF obj: %v", err)
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(bpfBytes))
	if err != nil {
		log.Fatalf("load spec: %v", err)
	}

	// --- 透過 RewriteConstants 設定 eBPF 全域常數 ---
	if err := spec.RewriteConstants(map[string]interface{}{
		"SAMPLE_RATE":   uint32(sampleRate),
		"ENABLE_FILTER": uint32(1),
		"TARGET_LEVEL":  uint32(level),
		"TARGET_CGID":   uint64(targetID),
	}); err != nil {
		log.Fatalf("rewrite consts: %v", err)
	}

	// --- 載入 eBPF programs/maps 到 kernel ---
	var objs struct {
		Programs struct {
			TpPageFaultUser   *ebpf.Program `ebpf:"tp_page_fault_user"`
			TpPageFaultKernel *ebpf.Program `ebpf:"tp_page_fault_kernel"`
		}
		Maps struct {
			Events    *ebpf.Map `ebpf:"events"`
			PerCpuCnt *ebpf.Map `ebpf:"per_cpu_cnt"`
		}
	}
	if err := spec.LoadAndAssign(&objs, nil); err != nil {
		log.Fatalf("load&assign: %v", err)
	}
	defer objs.Programs.TpPageFaultUser.Close()
	defer objs.Programs.TpPageFaultKernel.Close()
	defer objs.Maps.Events.Close()

	// Attach tracepoints
	lnk1, err := link.Tracepoint("exceptions", "page_fault_user", objs.Programs.TpPageFaultUser, nil)
	if err != nil {
		log.Fatalf("attach user: %v", err)
	}
	defer lnk1.Close()
	lnk2, err := link.Tracepoint("exceptions", "page_fault_kernel", objs.Programs.TpPageFaultKernel, nil)
	if err != nil {
		log.Fatalf("attach kernel: %v", err)
	}
	defer lnk2.Close()

	// Ring buffer reader
	rb, err := ringbuf.NewReader(objs.Maps.Events)
	if err != nil {
		log.Fatalf("ringbuf: %v", err)
	}
	defer rb.Close()

	// --- 啟動 Prometheus /metrics HTTP server ---
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("Prometheus metrics on :2112/metrics")
		log.Fatal(http.ListenAndServe(":2112", nil))
	}()

	// --- 讀取 ringbuf 事件 ---
	log.Printf("Reading ring buffer…")
	for {
		rec, err := rb.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return //結束
			}
			// 例如 EAGAIN/EINTR 等，略過重試
			continue
		}

		var e event
		// 確保長度足夠再解碼（避免邊界錯誤）
		if len(rec.RawSample) < binary.Size(e) {
			continue
		}
		if err := binary.Read(bytes.NewReader(rec.RawSample), binary.LittleEndian, &e); err != nil {
			continue
		}

		// 將 eBPF event 轉成 Prometheus counter
		pageFaults.WithLabelValues(labelType(e.Type), fmt.Sprintf("%d", e.CgroupID)).Inc()
	}
}

// --- 環境變數讀取輔助 ---
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
func getenvUint(key string, def uint) uint {
	if v := os.Getenv(key); v != "" {
		var x uint
		_, err := fmt.Sscanf(v, "%d", &x)
		if err == nil {
			return x
		}
	}
	return def
}
