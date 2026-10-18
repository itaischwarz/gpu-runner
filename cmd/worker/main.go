// Command worker runs on each GPU box. It reports the box's GPUs and runs the
// jobs the central server sends it, enforcing each job's GPU memory request.
// It has no queue, Redis, or scheduling: the central server does all of that.
//
// It listens on 127.0.0.1 by default because it runs any command it is sent:
// reach it from the central server through an SSH tunnel, e.g.
//
//	ssh -N -L 19000:127.0.0.1:19000 root@<box>
//
// then set GPU_WORKERS=http://127.0.0.1:19000 on the central server.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gpu-runner/internal/box"
	"gpu-runner/internal/controlplane"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/logger"
)

func main() {
	addr := flag.String("addr", envOr("WORKER_ADDRESS", "127.0.0.1:19000"), "address to listen on (WORKER_ADDRESS)")
	smiPath := flag.String("nvidia-smi", envOr("NVIDIA_SMI_PATH", "nvidia-smi"), "path to nvidia-smi (NVIDIA_SMI_PATH)")
	workDir := flag.String("work-dir", envOr("WORKER_WORK_DIR", filepath.Join(os.TempDir(), "gpu-runner-worker")), "directory jobs run in (WORKER_WORK_DIR)")
	memInterval := flag.Duration("memory-check-interval", envDuration("GPU_MEMORY_CHECK_INTERVAL", 2*time.Second), "how often job GPU memory is checked (GPU_MEMORY_CHECK_INTERVAL)")
	memGrace := flag.Int("memory-grace-checks", envInt("GPU_MEMORY_GRACE_CHECKS", 2), "checks in a row over the request before a job is killed (GPU_MEMORY_GRACE_CHECKS)")
	flag.Parse()

	l := logger.Server
	if err := os.MkdirAll(*workDir, 0o755); err != nil {
		log.Fatalf("create work dir %s: %v", *workDir, err)
	}

	gpus := controlplane.NewGPUProvider(*smiPath)
	slots, err := gpus.Poll(context.Background())
	if err != nil {
		log.Fatalf("nvidia-smi not usable at %q: %v", *smiPath, err)
	}
	for _, s := range slots {
		l.Info("Found GPU", "id", s.ID, "model", s.Model, "total_memory_mb", s.TotalMemoryMB, "healthy", s.Healthy)
	}

	// No job timeout here: the central server owns timeouts and cancels the
	// request, which kills the job.
	exec := executer.NewExecutor(0)
	exec.SetMemoryLimits(executer.NvidiaSMIMemoryProbe(*smiPath), *memInterval, *memGrace)

	// Every request's context derives from jobsCtx, so cancelling it on
	// shutdown kills the box's running jobs (their whole process groups)
	// instead of leaving them orphaned on the GPU. Each job then reports a
	// failure, and the central server retries it on another GPU.
	jobsCtx, killJobs := context.WithCancel(context.Background())
	defer killJobs()
	server := &http.Server{
		Addr:              *addr,
		Handler:           box.NewHandler(gpus, exec, *workDir),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return jobsCtx },
	}
	go func() {
		l.Info("GPU worker listening", "address", *addr, "gpus", len(slots), "work_dir", *workDir)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("worker server failed: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	<-ctx.Done()
	l.Info("GPU worker shutting down, stopping running jobs")
	killJobs()
	// Give the killed jobs' handlers a moment to tell the central server.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return d
	}
	return def
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return def
}
