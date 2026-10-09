package controlplane

import (
	"strings"
	"testing"

	"gpu-runner/internal/jobs"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestGPUCollectorReportsMemoryHealthAndBusy(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", Model: "A100", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", Model: "RTX 4090", TotalMemoryMB: 24564, FreeMemoryMB: 24000, Healthy: true},
	}}
	cp, d := newTestDispatcher(t, provider)

	placedBefore := testutil.ToFloat64(jobsPlaced.WithLabelValues("GPU-a"))
	if err := d.Place("GPU-a", &jobs.Job{ID: "1", MemoryMB: 8000}); err != nil {
		t.Fatalf("Place failed: %v", err)
	}
	if got := testutil.ToFloat64(jobsPlaced.WithLabelValues("GPU-a")) - placedBefore; got != 1 {
		t.Errorf("expected 1 placement counted on GPU-a, got %v", got)
	}

	// GPU-b fails; a failed GPU's memory reading is unreadable, so it reports 0.
	provider.slots = []Slot{{ID: "GPU-a", FreeMemoryMB: 70000, Healthy: true}, {ID: "GPU-b", Healthy: false}}
	cp.PollFreeMemory(t.Context())

	expected := `
# HELP gpu_runner_gpu_busy 1 while the GPU is running a job, else 0.
# TYPE gpu_runner_gpu_busy gauge
gpu_runner_gpu_busy{gpu="GPU-a"} 1
gpu_runner_gpu_busy{gpu="GPU-b"} 0
# HELP gpu_runner_gpu_healthy 1 if the GPU was healthy at the last poll, else 0.
# TYPE gpu_runner_gpu_healthy gauge
gpu_runner_gpu_healthy{gpu="GPU-a"} 1
gpu_runner_gpu_healthy{gpu="GPU-b"} 0
# HELP gpu_runner_gpu_memory_free_mb Free GPU memory as of the last poll, in MB.
# TYPE gpu_runner_gpu_memory_free_mb gauge
gpu_runner_gpu_memory_free_mb{gpu="GPU-a",model="A100"} 70000
gpu_runner_gpu_memory_free_mb{gpu="GPU-b",model="RTX 4090"} 0
# HELP gpu_runner_gpu_memory_total_mb Total GPU memory, in MB.
# TYPE gpu_runner_gpu_memory_total_mb gauge
gpu_runner_gpu_memory_total_mb{gpu="GPU-a",model="A100"} 81920
gpu_runner_gpu_memory_total_mb{gpu="GPU-b",model="RTX 4090"} 24564
`
	if err := testutil.CollectAndCompare(NewGPUCollector(cp, d), strings.NewReader(expected)); err != nil {
		t.Error(err)
	}
}
