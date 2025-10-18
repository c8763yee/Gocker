// Go 1.22+ ; cilium/ebpf v0.15+
// 需 root 或 CAP_BPF+CAP_PERFMON+CAP_SYS_ADMIN
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	cgroupRootDefault = "/sys/fs/cgroup"
	gockerPathDefault = "/sys/fs/cgroup/gocker"
)

type cgKey struct {
	Cgid uint64
	Type uint32
	Pad  uint32
}

type cfg struct {
	SampleRate   uint32 `json:"sample_rate"`
	EnableFilter uint32 `json:"enable_filter"`
	TargetLevel  uint32 `json:"target_level"`
	TargetCgid   uint64 `json:"target_cgid"`
}

// Prom metrics
var (
	pageFaults = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "page_faults_total", Help: "Per-cgroup page faults."},
		[]string{"type", "cgroup_id"},
	)
	schedEvents = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "sched_events_total", Help: "Per-cgroup scheduler events."},
		[]string{"type", "cgroup_id"},
	)
	sysCalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "syscall_calls_total", Help: "Per-cgroup per-syscall calls."},
		[]string{"syscall", "cgroup_id"},
	)
	sysLatency = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "syscall_latency_nanoseconds_total", Help: "Per-cgroup per-syscall total latency (ns)."},
		[]string{"syscall", "cgroup_id"},
	)
	cpuSched = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "cpu_sched_ns_total", Help: "Per-cgroup scheduler time buckets (ns)."},
		[]string{"type", "cgroup_id"},
	)
)

func labelPF(t uint32) string {
	if t == 1 {
		return "user"
	}
	if t == 2 {
		return "kernel"
	}
	return "unknown"
}
func labelSC(t uint32) string {
	if t == 1 {
		return "switch"
	}
	if t == 2 {
		return "wakeup"
	}
	return "unknown"
}
func labelSys(sysnr uint32) string { return fmt.Sprintf("%d", sysnr) }
func labelCPU(t uint32) string {
	switch t {
	case 1:
		return "runtime"
	case 2:
		return "wait"
	case 3:
		return "iowait"
	default:
		return "unknown"
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
		if _, err := fmt.Sscanf(v, "%d", &x); err == nil {
			return x
		}
	}
	return def
}

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
	st := fi.Sys().(*syscall.Stat_t)
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

func collectAllowed(root string) (map[uint64]struct{}, error) {
	allowed := make(map[uint64]struct{})
	add := func(path string) error {
		ino, err := cgroupInode(path)
		if err != nil {
			return err
		}
		allowed[ino] = struct{}{}
		return nil
	}
	if err := add(root); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return allowed, err
	}
	var agg error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := add(filepath.Join(root, entry.Name())); err != nil && !os.IsNotExist(err) {
			agg = errors.Join(agg, err)
		}
	}
	return allowed, agg
}

// --- modules typed holders (含 config map) ---
type pfModule struct {
	ProgU *ebpf.Program `ebpf:"tp_page_fault_user"`
	ProgK *ebpf.Program `ebpf:"tp_page_fault_kernel"`
	MapPF *ebpf.Map     `ebpf:"cg_pf_cnt"`
	Cfg   *ebpf.Map     `ebpf:"cfg_map"` // ← 原本是 "config"
}
type schedModule struct {
	Sw   *ebpf.Program `ebpf:"tp_sched_switch"`
	Wkp  *ebpf.Program `ebpf:"tp_sched_wakeup"`
	Exit *ebpf.Program `ebpf:"tp_sched_exit"`
	MapS *ebpf.Map     `ebpf:"cg_sched_cnt"`
	Cfg  *ebpf.Map     `ebpf:"cfg_map"` // ←
}
type sysModule struct {
	En   *ebpf.Program `ebpf:"tp_sys_enter"`
	Ex   *ebpf.Program `ebpf:"tp_sys_exit"`
	MapC *ebpf.Map     `ebpf:"cg_sys_cnt"`
	MapL *ebpf.Map     `ebpf:"cg_sys_lat_ns"`
	Cfg  *ebpf.Map     `ebpf:"cfg_map"` // ←
}
type cpuModule struct {
	Run    *ebpf.Program `ebpf:"tp_sched_stat_runtime"`
	Wait   *ebpf.Program `ebpf:"tp_sched_stat_wait"`
	IO     *ebpf.Program `ebpf:"tp_sched_stat_iowait"`
	Switch *ebpf.Program `ebpf:"tp_sched_switch_cpu"`
	Exit   *ebpf.Program `ebpf:"tp_sched_exit_cpu"`
	MapNs  *ebpf.Map     `ebpf:"cg_cpu_ns"`
	Cfg    *ebpf.Map     `ebpf:"cfg_map"`
}

func loadModule(path string, rew map[string]interface{}, out interface{}) (func(), error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	spec, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("load spec %s: %w", path, err)
	}
	// 保留 RewriteConstants 作為 fallback；主要控制用 config map 熱更新
	if len(rew) > 0 {
		if err := spec.RewriteConstants(rew); err != nil {
			return nil, fmt.Errorf("rewrite consts %s: %w", path, err)
		}
	}
	if err := spec.LoadAndAssign(out, nil); err != nil {
		return nil, fmt.Errorf("load&assign %s: %w", path, err)
	}
	return func() {
		switch m := out.(type) {
		case *pfModule:
			if m.ProgU != nil {
				m.ProgU.Close()
			}
			if m.ProgK != nil {
				m.ProgK.Close()
			}
			if m.MapPF != nil {
				m.MapPF.Close()
			}
			if m.Cfg != nil {
				m.Cfg.Close()
			}
		case *schedModule:
			if m.Sw != nil {
				m.Sw.Close()
			}
			if m.Wkp != nil {
				m.Wkp.Close()
			}
			if m.Exit != nil {
				m.Exit.Close()
			}
			if m.MapS != nil {
				m.MapS.Close()
			}
			if m.Cfg != nil {
				m.Cfg.Close()
			}
		case *sysModule:
			if m.En != nil {
				m.En.Close()
			}
			if m.Ex != nil {
				m.Ex.Close()
			}
			if m.MapC != nil {
				m.MapC.Close()
			}
			if m.MapL != nil {
				m.MapL.Close()
			}
			if m.Cfg != nil {
				m.Cfg.Close()
			}
		case *cpuModule:
			if m.Run != nil {
				m.Run.Close()
			}
			if m.Wait != nil {
				m.Wait.Close()
			}
			if m.IO != nil {
				m.IO.Close()
			}
			if m.Switch != nil {
				m.Switch.Close()
			}
			if m.Exit != nil {
				m.Exit.Close()
			}
			if m.MapNs != nil {
				m.MapNs.Close()
			}
			if m.Cfg != nil {
				m.Cfg.Close()
			}
		}
	}, nil
}

func attachTracepoint(cat, name string, prog *ebpf.Program) link.Link {
	if prog == nil {
		return nil
	}
	lnk, err := link.Tracepoint(cat, name, prog, nil)
	if err != nil {
		log.Printf("attach %s/%s failed: %v", cat, name, err)
		return nil
	}
	return lnk
}

func writeCfgAll(c cfg, mods ...interface{}) {
	key := uint32(0)
	for _, m := range mods {
		switch mo := m.(type) {
		case *pfModule:
			if mo != nil && mo.Cfg != nil {
				_ = mo.Cfg.Update(&key, &c, ebpf.UpdateAny)
			}
		case *schedModule:
			if mo != nil && mo.Cfg != nil {
				_ = mo.Cfg.Update(&key, &c, ebpf.UpdateAny)
			}
		case *sysModule:
			if mo != nil && mo.Cfg != nil {
				_ = mo.Cfg.Update(&key, &c, ebpf.UpdateAny)
			}
		case *cpuModule:
			if mo != nil && mo.Cfg != nil {
				_ = mo.Cfg.Update(&key, &c, ebpf.UpdateAny)
			}
		}
	}
}

func main() {
	prometheus.MustRegister(pageFaults, schedEvents, sysCalls, sysLatency, cpuSched)

	cgroupRoot := getenv("CGROUP_ROOT", cgroupRootDefault)
	targetPath := getenv("PF_TARGET_CGROUP", gockerPathDefault)
	sampleRate := getenvUint("PF_SAMPLE_RATE", 1)

	if !isUnifiedCgroupV2(cgroupRoot) {
		log.Fatalf("require cgroup v2 at %s", cgroupRoot)
	}
	tgid, err := cgroupInode(targetPath)
	if err != nil {
		log.Fatalf("stat %s: %v", targetPath, err)
	}
	level, err := cgroupLevel(cgroupRoot, targetPath)
	if err != nil {
		log.Fatalf("compute level: %v", err)
	}

	cfg0 := cfg{SampleRate: uint32(sampleRate), EnableFilter: 1, TargetLevel: uint32(level), TargetCgid: uint64(tgid)}
	log.Printf("Target subtree %s (inode=%d, level=%d), sample_rate=%d", targetPath, tgid, level, sampleRate)

	var (
		allowedMu    sync.RWMutex
		allowed      map[uint64]struct{}
		allowedCount int
	)
	refreshAllowed := func() {
		set, err := collectAllowed(targetPath)
		if err != nil {
			log.Printf("collect allowed from %s: %v", targetPath, err)
		}
		if set == nil {
			set = make(map[uint64]struct{})
		}
		if len(set) == 0 {
			set[cfg0.TargetCgid] = struct{}{}
		}
		allowedMu.Lock()
		allowed = set
		allowedMu.Unlock()
		if len(set) != allowedCount {
			allowedCount = len(set)
			log.Printf("allowed cgroup whitelist size=%d", allowedCount)
		}
	}
	refreshAllowed()

	// 載入各模組（無 pin，程序退出 maps 就銷毀）
	var pf pfModule
	var sched schedModule
	var sys sysModule
	var cpu cpuModule

	rew := map[string]interface{}{"SAMPLE_RATE": uint32(sampleRate), "ENABLE_FILTER": uint32(1), "TARGET_LEVEL": uint32(level), "TARGET_CGID": uint64(tgid)}
	if closer, err := loadModule("bpf/pf.bpf.o", rew, &pf); err != nil {
		log.Printf("pf module skipped: %v", err)
	} else {
		defer closer()
	}
	if closer, err := loadModule("bpf/sched.bpf.o", rew, &sched); err != nil {
		log.Printf("sched module skipped: %v", err)
	} else {
		defer closer()
	}
	if closer, err := loadModule("bpf/sys.bpf.o", rew, &sys); err != nil {
		log.Printf("sys module skipped: %v", err)
	} else {
		defer closer()
	}
	if closer, err := loadModule("bpf/cpu.bpf.o", rew, &cpu); err != nil {
		log.Printf("cpu module skipped: %v", err)
	} else {
		defer closer()
	}

	writeCfgAll(cfg0, &pf, &sched, &sys, &cpu)

	links := []link.Link{
		attachTracepoint("exceptions", "page_fault_user", pf.ProgU),
		attachTracepoint("exceptions", "page_fault_kernel", pf.ProgK),
		attachTracepoint("sched", "sched_switch", sched.Sw),
		attachTracepoint("sched", "sched_wakeup", sched.Wkp),
		attachTracepoint("sched", "sched_process_exit", sched.Exit),
		attachTracepoint("raw_syscalls", "sys_enter", sys.En),
		attachTracepoint("raw_syscalls", "sys_exit", sys.Ex),
		attachTracepoint("sched", "sched_stat_runtime", cpu.Run),
		attachTracepoint("sched", "sched_stat_wait", cpu.Wait),
		attachTracepoint("sched", "sched_stat_iowait", cpu.IO),
		attachTracepoint("sched", "sched_switch", cpu.Switch),
		attachTracepoint("sched", "sched_process_exit", cpu.Exit),
	}
	defer func() {
		for _, l := range links {
			if l != nil {
				l.Close()
			}
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/admin/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var in cfg
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if in.SampleRate == 0 {
			in.SampleRate = cfg0.SampleRate
		}
		if in.TargetLevel == 0 {
			in.TargetLevel = cfg0.TargetLevel
		}
		if in.TargetCgid == 0 {
			in.TargetCgid = cfg0.TargetCgid
		}
		if in.EnableFilter != 0 && in.EnableFilter != 1 {
			in.EnableFilter = 1
		}
		cfg0 = in
		writeCfgAll(cfg0, &pf, &sched, &sys, &cpu)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
		log.Printf("config updated: %+v", cfg0)
	})
	go func() {
		log.Printf("HTTP: metrics on :2112/metrics ; admin on :2112/admin/config (POST JSON)")
		log.Fatal(http.ListenAndServe(":2112", nil))
	}()

	ncpu := runtime.NumCPU()
	val := make([]uint64, ncpu)
	type lastMap = map[cgKey]uint64
	lastPF := make(lastMap, 4096)
	lastSC := make(lastMap, 4096)
	lastSysCnt := make(lastMap, 16384)
	lastSysSum := make(lastMap, 16384)
	lastCPU := make(lastMap, 4096)

	allowedHas := func(cgid uint64) bool {
		allowedMu.RLock()
		defer allowedMu.RUnlock()
		if allowed == nil {
			return false
		}
		_, ok := allowed[cgid]
		return ok
	}

	scan := func(m *ebpf.Map, last lastMap, apply func(k cgKey, delta uint64)) {
		if m == nil {
			return
		}
		var k cgKey
		it := m.Iterate()
		for it.Next(&k, &val) {
			var total uint64
			for _, v := range val {
				total += v
			}
			if !allowedHas(k.Cgid) {
				delete(last, k)
				continue
			}
			prev := last[k]
			if total >= prev {
				d := total - prev
				if d > 0 {
					apply(k, d)
				}
			}
			last[k] = total
		}
		if err := it.Err(); err != nil {
			log.Printf("iterate map: %v", err)
		}
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	log.Printf("Scanning maps each 1s…")
	for range ticker.C {
		refreshAllowed()
		scan(pf.MapPF, lastPF, func(k cgKey, d uint64) {
			pageFaults.WithLabelValues(labelPF(k.Type), fmt.Sprintf("%d", k.Cgid)).Add(float64(d))
		})
		scan(sched.MapS, lastSC, func(k cgKey, d uint64) {
			schedEvents.WithLabelValues(labelSC(k.Type), fmt.Sprintf("%d", k.Cgid)).Add(float64(d))
		})
		scan(sys.MapC, lastSysCnt, func(k cgKey, d uint64) {
			sysCalls.WithLabelValues(labelSys(k.Type), fmt.Sprintf("%d", k.Cgid)).Add(float64(d))
		})
		scan(sys.MapL, lastSysSum, func(k cgKey, d uint64) {
			sysLatency.WithLabelValues(labelSys(k.Type), fmt.Sprintf("%d", k.Cgid)).Add(float64(d))
		})
		scan(cpu.MapNs, lastCPU, func(k cgKey, d uint64) {
			cpuSched.WithLabelValues(labelCPU(k.Type), fmt.Sprintf("%d", k.Cgid)).Add(float64(d))
		})
	}
}
