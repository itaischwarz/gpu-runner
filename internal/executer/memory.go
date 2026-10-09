package executer

import (
	"context"
	"fmt"
	"gpu-runner/internal/logger"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var memoryKills = promauto.NewCounter(prometheus.CounterOpts{
	Name: "gpu_runner_gpu_memory_kills_total",
	Help: "Jobs killed for using more GPU memory than they requested.",
})

// MemoryProbe returns how much GPU memory, in MB, the processes in process
// group pgid are using right now.
type MemoryProbe func(ctx context.Context, pgid int) (int, error)

// MemoryExceededError means a job used more GPU memory than it requested and
// was killed. Retrying with the same request would fail the same way.
type MemoryExceededError struct {
	UsedMB  int
	LimitMB int
}

func (e *MemoryExceededError) Error() string {
	return fmt.Sprintf("GPU memory exceeded: used %d MB of %d MB requested; resubmit with --memory of at least %d",
		e.UsedMB, e.LimitMB, e.UsedMB)
}

// SetMemoryLimits turns on GPU memory enforcement. Every interval, a running
// job with a memory limit is measured with probe; after graceChecks checks in
// a row above its limit, the job's whole process group is killed. Call it once
// at startup, before any job runs.
func (e *Executor) SetMemoryLimits(probe MemoryProbe, interval time.Duration, graceChecks int) {
	e.memProbe = probe
	e.memInterval = interval
	e.memGraceChecks = graceChecks
}

// watchMemory enforces limitMB on process group pgid until ctx is done. If
// the job stays over its limit, it calls kill and reports the breach on done;
// otherwise it reports nil. It always sends exactly once on done.
func (e *Executor) watchMemory(ctx context.Context, pgid, limitMB int, kill func(), done chan<- *MemoryExceededError, jobID string) {
	ticker := time.NewTicker(e.memInterval)
	defer ticker.Stop()

	strikes := 0
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-ticker.C:
			used, err := e.memProbe(ctx, pgid)
			if err != nil {
				// Never kill a job on a reading we couldn't take.
				executorLogger.Warn("GPU memory check failed", "job_id", jobID, "error", err)
				continue
			}
			if used <= limitMB {
				strikes = 0
				continue
			}
			strikes++
			executorLogger.Warn("Job over GPU memory request", "job_id", jobID, "used_mb", used, "limit_mb", limitMB, "strike", strikes)
			if strikes >= e.memGraceChecks {
				memoryKills.Inc()
				kill()
				done <- &MemoryExceededError{UsedMB: used, LimitMB: limitMB}
				return
			}
		}
	}
}

// NvidiaSMIMemoryProbe measures a process group's GPU memory with
// nvidia-smi's per-process report. A job's GPU work usually runs in a child of
// `bash -c`, so processes are matched by process group, not PID.
func NvidiaSMIMemoryProbe(smiPath string) MemoryProbe {
	return func(ctx context.Context, pgid int) (int, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, smiPath,
			"--query-compute-apps=pid,used_memory",
			"--format=csv,noheader,nounits",
		).Output()
		if err != nil {
			return 0, fmt.Errorf("nvidia-smi compute-apps query failed: %w", err)
		}

		total := 0
		for pid, mb := range parseComputeApps(string(out)) {
			if group, err := syscall.Getpgid(pid); err == nil && group == pgid {
				total += mb
			}
		}
		return total, nil
	}
}

// parseComputeApps reads "pid, used_memory" lines into PID → MB. Lines it
// can't read (e.g. "[N/A]" memory) are skipped.
func parseComputeApps(out string) map[int]int {
	usage := make(map[int]int)
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, ",")
		if len(parts) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		mb, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil {
			continue
		}
		usage[pid] += mb
	}
	return usage
}

// logMemoryKill records the kill in the job's log stream, so anyone following
// the job's logs sees why it stopped.
func logMemoryKill(jobLogger logger.JobLogger, err *MemoryExceededError) {
	jobLogger.Error(err.Error(),
		logger.Item("used_mb", err.UsedMB),
		logger.Item("requested_mb", err.LimitMB),
	)
}
