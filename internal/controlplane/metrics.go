package controlplane

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	pollSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "gpu_runner_nvidia_smi_poll_seconds",
		Help:    "Time taken by each nvidia-smi health poll.",
		Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
	pollFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gpu_runner_nvidia_smi_poll_failures_total",
		Help: "nvidia-smi health polls that failed or timed out.",
	})
	jobsPlaced = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gpu_runner_gpu_jobs_placed_total",
		Help: "Jobs placed on each GPU.",
	}, []string{"gpu"})
)

var (
	memFreeDesc = prometheus.NewDesc("gpu_runner_gpu_memory_free_mb",
		"Free GPU memory as of the last poll, in MB.", []string{"gpu", "model"}, nil)
	memTotalDesc = prometheus.NewDesc("gpu_runner_gpu_memory_total_mb",
		"Total GPU memory, in MB.", []string{"gpu", "model"}, nil)
	healthyDesc = prometheus.NewDesc("gpu_runner_gpu_healthy",
		"1 if the GPU was healthy at the last poll, else 0.", []string{"gpu"}, nil)
	busyDesc = prometheus.NewDesc("gpu_runner_gpu_busy",
		"1 while the GPU is running a job, else 0.", []string{"gpu"}, nil)
)

// GPUCollector reports each GPU's state when Prometheus scrapes, reading the
// slot table and the dispatcher directly so the values are never stale.
type GPUCollector struct {
	cp         *ControlPlane
	dispatcher *Dispatcher
}

func NewGPUCollector(cp *ControlPlane, d *Dispatcher) *GPUCollector {
	return &GPUCollector{cp: cp, dispatcher: d}
}

func (c *GPUCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- memFreeDesc
	ch <- memTotalDesc
	ch <- healthyDesc
	ch <- busyDesc
}

func (c *GPUCollector) Collect(ch chan<- prometheus.Metric) {
	// A GPU is busy when it isn't in the dispatcher's idle set.
	c.dispatcher.mu.Lock()
	idle := make(map[string]bool, len(c.dispatcher.available))
	for id := range c.dispatcher.available {
		idle[id] = true
	}
	c.dispatcher.mu.Unlock()

	for _, s := range c.cp.slotTable.Snapshot() {
		ch <- prometheus.MustNewConstMetric(memFreeDesc, prometheus.GaugeValue, float64(s.FreeMemoryMB), s.ID, s.Model)
		ch <- prometheus.MustNewConstMetric(memTotalDesc, prometheus.GaugeValue, float64(s.TotalMemoryMB), s.ID, s.Model)
		ch <- prometheus.MustNewConstMetric(healthyDesc, prometheus.GaugeValue, boolToFloat(s.Healthy), s.ID)
		ch <- prometheus.MustNewConstMetric(busyDesc, prometheus.GaugeValue, boolToFloat(!idle[s.ID]), s.ID)
	}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
