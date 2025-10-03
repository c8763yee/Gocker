// metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Counter: 已啟動容器總數
	ContainersStartedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gocker_containers_started_total",
		Help: "The total number of containers started since gocker was launched.",
	})

	// Gauge: 當前運行的容器數量
	ContainersRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "gocker_containers_running_current",
		Help: "The current number of containers running under gocker.",
	})

	// Histogram: 命令執行延遲 (單位：秒)
	CommandDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gocker_command_duration_seconds",
		Help:    "Duration of gocker commands.",
		Buckets: prometheus.LinearBuckets(0.1, 0.1, 10), // 10 個 bucket，從 0.1 秒開始，每個 bucket 寬 0.1 秒
	}, []string{"command"}) // 使用 "command" 標籤來區分是 run, stop 還是 rm

	// Counter with Labels: 錯誤總數
	ErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gocker_errors_total",
		Help: "Total number of errors encountered, categorized by command.",
	}, []string{"command"}) // 使用 "command" 標籤來區分錯誤來源
)
