// cmd/pf_exporter/main.go
// go 1.22+
// 需要 root 或 CAP_BPF+CAP_PERFMON+CAP_SYS_ADMIN（最簡單 sudo 跑）

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
	cgroupRootDefault = "/sys/fs/cgroup"
	gockerPathDefault = "/sys/fs/cgroup/gocker"
	bpfObjDefault     = "bpf/pf.bpf.o" // 可用環境變數 BPF_OBJ 覆寫
)

type event struct {
	TsNS     uint64
	Type     uint32
	Pid      uint32
	Tgid     uint32
	CgroupID uint64
	Comm     [16]byte
}

var (
	pageFaults = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "page_faults_total",
			Help: "Aggregated page fault events from eBPF (sampled).",
		},
		[]string{"type", "cgroup_id"},
	)
)

func isUnifiedCgroupV2(root string) bool {
	var st syscall.Statfs_t
	if err := syscall.Statfs(root, &st); err != nil {
		return false
	}
	const CGROUP2_SUPER_MAGIC = 0x63677270
	return st.Type == CGROUP2_SUPER_MAGIC
}

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
	prometheus.MustRegister(pageFaults)

	cgroupRoot := getenv("CGROUP_ROOT", cgroupRootDefault)
	targetPath := getenv("PF_TARGET_CGROUP", gockerPathDefault)
	bpfObjPath := getenv("BPF_OBJ", bpfObjDefault)
	sampleRate := getenvUint("PF_SAMPLE_RATE", 1)

	if !isUnifiedCgroupV2(cgroupRoot) {
		log.Fatalf("require cgroup v2 unified mount at %s", cgroupRoot)
	}
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

	// 讀取 BPF ELF
	bpfBytes, err := os.ReadFile(bpfObjPath)
	if err != nil {
		log.Fatalf("read BPF obj: %v", err)
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(bpfBytes))
	if err != nil {
		log.Fatalf("load spec: %v", err)
	}

	// 覆寫 .rodata 常數
	if err := spec.RewriteConstants(map[string]interface{}{
		"SAMPLE_RATE":   uint32(sampleRate),
		"ENABLE_FILTER": uint32(1),
		"TARGET_LEVEL":  uint32(level),
		"TARGET_CGID":   uint64(targetID),
	}); err != nil {
		log.Fatalf("rewrite consts: %v", err)
	}

	// 載入並繫結 maps/programs
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

	// /metrics
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Printf("Prometheus metrics on :2112/metrics")
		log.Fatal(http.ListenAndServe(":2112", nil))
	}()

	log.Printf("Reading ring buffer…")
	log.Printf("Reading ring buffer…")
	for {
		rec, err := rb.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return
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

		pageFaults.WithLabelValues(labelType(e.Type), fmt.Sprintf("%d", e.CgroupID)).Inc()
	}
}

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
