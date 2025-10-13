package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sys/unix"
)

const (
	bpfProgramPath = "./bpf/latency.o" //你的 eBPF ELF 物件路徑（會用 cilium/ebpf 載入）
	memLockLimit   = 100 * 1024 * 1024 // 100MB(設定 RLIMIT_MEMLOCK，用來避免建立 map/program 時因可鎖定記憶體額度太小而失敗)
)

type LatencyT struct {
	TimestampIn  uint64
	TimestampOut uint64
	Delta        uint64
	Layer3       L3
}

type L3 struct {
	SrcIP  uint32
	DstIP  uint32
	HProto uint8
	// add padding to match the size of the struct in the BPF program
	_ [3]byte
}

// 定義與註冊 Prometheus metircs, 讓 eBPF 程式在 userspace 透過 Prometheus SDK 暴露資料給 /metrics。
var (
	// 宣告兩個 Prometheus metrics：PacketsCount 與 LatencyHistogram
	/*
		Gauge 是一種「可增可減」的即時數值（如目前的 packet 數、CPU 使用率）。
		Vec 表示這個 gauge 有多組 label，例如不同 IP 對應不同值。
	*/
	PacketsCount = prometheus.NewGaugeVec( // 建立一個 GaugeVec：可標上多個 label 的 gauge 型指標
		prometheus.GaugeOpts{ // Gauge 的基本設定（metadata）
			Name: "packets_count",              // 指標名稱
			Help: "Number of packets received", // 說明文字
		},
		[]string{"src_ip", "dst_ip"}, // 定義這個指標的 labels（標籤維度）
	)
	LatencyHistogram = prometheus.NewHistogramVec( // 建立一個 HistogramVec：有 label 的 直方圖型 (Histogram) 指標
		prometheus.HistogramOpts{ // 設定 histogram 的基本屬性
			Name:    "latency_histogram",   // /metrics 中的指標名稱
			Help:    "Latency histogram",   // 說明這個指標的用途
			Buckets: prometheus.DefBuckets, // 定義 bucket 邊界（每個延遲範圍）
		},
		[]string{"src_ip", "dst_ip"},
	)
)

/*
將你建立的 metrics 註冊到 Prometheus 的全域 registry。
只有註冊過的指標才會出現在 /metrics。
MustRegister() 會在註冊失敗時直接 panic（方便除錯）。
*/
func init() {
	prometheus.MustRegister(PacketsCount)
	prometheus.MustRegister(LatencyHistogram)
}

func main() {
	// Set the RLIMIT_MEMLOCK resource limit
	//當前 process 的 memlock 上限被提升為 100MB。
	// eBPF 在建立 map / prog 時不會再因資源限制報錯。
	var rLimit unix.Rlimit
	rLimit.Cur = memLockLimit
	rLimit.Max = memLockLimit
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &rLimit); err != nil {
		log.Fatalf("Failed to set RLIMIT_MEMLOCK: %v", err)
	}

	// Parse the ELF file containing the BPF program
	// spec *ebpf.CollectionSpec：描述整個 BPF collection 的結構（但此時還沒載入到 kernel）。
	// err error：若檔案不存在、格式錯誤或不是合法的 eBPF ELF 檔，就回傳錯誤。
	spec, err := ebpf.LoadCollectionSpec(bpfProgramPath) // 讀取並解析指定路徑的 ELF 檔，把其中所有的 eBPF map、program、license 等 metadata 讀進記憶體。
	if err != nil {
		log.Fatalf("Failed to load BPF program: %v", err)
	}

	// Load the BPF program into the kernel
	// coll 是一個 *ebpf.Collection 結構，代表目前 kernel 中那份 BPF 物件
	coll, err := ebpf.NewCollection(spec) // 這行把剛剛解析的 spec（ELF 描述資訊）實際載入到 kernel 中
	if err != nil {
		log.Fatalf("Failed to create BPF collection: %v", err)
	}
	defer coll.Close() //結束程式時自動清理資源（unpin、close fd）

	// Attach BPF programs to kprobe receive events
	tp_rcv, err := link.Kprobe("ip_rcv", coll.Programs["ip_rcv"], &link.KprobeOptions{})
	if err != nil {
		log.Fatalf("Failed to attach trace_ip: %v", err)
	}
	defer tp_rcv.Close()

	// Attach BPF programs to kprobe return events
	tp_ret, err := link.Kprobe("ip_rcv_finish", coll.Programs["ip_rcv_finish"], &link.KprobeOptions{})
	if err != nil {
		log.Fatalf("Failed to attach trace_ip_output: %v", err)
	}

	// Set up ring buffer to read data from BPF program
	reader, err := ringbuf.NewReader(coll.Maps["events"])
	if err != nil {
		log.Fatalf("Failed to get ring: %v", err)
	}

	// Handle signals for graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// Goroutine to handle graceful shutdown on receiving a signal
	go func() {
		<-sig
		tp_rcv.Close()
		tp_ret.Close()
		coll.Close()
		os.Exit(0)
	}()

	go func() {
		// Read and print the output from the eBPF program
		var event LatencyT

		for {

			// Read data from the ring buffer
			data, err := reader.Read()
			if err != nil {
				log.Fatalf("Failed to read from ring buffer: %v", err)
			}

			if err := binary.Read(bytes.NewReader(data.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("Failed to parse ring event: %v", err)
				continue
			}

			// Convert IP addresses to string format
			srcIP := toIpV4(event.Layer3.SrcIP)
			dstIP := toIpV4(event.Layer3.DstIP)

			// Increment Prometheus metric
			PacketsCount.WithLabelValues(srcIP, dstIP).Inc()
			LatencyHistogram.WithLabelValues(srcIP, dstIP).Observe(float64(event.Delta))

			// Print the output
			fmt.Printf("TimestampIn: %s, TimestampOut: %s, Delta: %d, SrcIP: %s, DstIP: %s, HProto: %s\n", timestampToString(event.TimestampIn), timestampToString(event.TimestampOut), event.Delta, srcIP, dstIP, protoToString(event.Layer3.HProto))
		}
	}()

	// Start Prometheus HTTP server
	http.Handle("/metrics", promhttp.Handler()) // 註冊一個 HTTP handler，當有用戶（或 Prometheus server）連到 /metrics 時，由 promhttp.Handler() 自動輸出目前所有已註冊的指標（metrics）
	log.Fatal(http.ListenAndServe(":2112", nil))
}

func toIpV4(ip uint32) string {
	ipOut := make(net.IP, 4)                 // Create a 4-byte IP address
	binary.LittleEndian.PutUint32(ipOut, ip) // Convert uint32 to byte slice in little-endian order
	return ipOut.String()                    // Convert IP address to string format
}

func protoToString(protocol uint8) string {
	switch protocol {
	case 1:
		return "ICMP"
	case 2:
		return "IGMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 89:
		return "OSPF"
	default:
		return "Unknown"
	}
}

func timestampToString(timestamp uint64) string {
	// Convert the timestamp to a time.Time object
	t := time.Unix(0, int64(timestamp))
	// Format the time.Time object to a human-readable string
	return t.Format(time.RFC3339)
}
