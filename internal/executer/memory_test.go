package executer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fixedProbe reports the given usages in order, then repeats the last one.
func fixedProbe(usages ...int) MemoryProbe {
	var calls atomic.Int32
	return func(ctx context.Context, pgid int) (int, error) {
		i := int(calls.Add(1)) - 1
		if i >= len(usages) {
			i = len(usages) - 1
		}
		return usages[i], nil
	}
}

func newLimitedExecutor(probe MemoryProbe) *Executor {
	e := NewExecutor(30 * time.Second)
	e.SetMemoryLimits(probe, 10*time.Millisecond, 2)
	return e
}

func TestJobOverMemoryLimitIsKilledWithChildren(t *testing.T) {
	e := newLimitedExecutor(fixedProbe(12000))
	dir := t.TempDir()
	childPID := filepath.Join(dir, "child.pid")

	// The GPU work usually runs in a child of bash; it must die too.
	cmd := fmt.Sprintf("sleep 30 & echo $! > %s; wait", childPID)
	start := time.Now()
	_, err := e.RunJobWithEnv(cmd, "mem-kill", dir, context.Background(), newTestLogger("mem-kill"), nil, 8000)

	var memErr *MemoryExceededError
	if !errors.As(err, &memErr) {
		t.Fatalf("expected MemoryExceededError, got %v", err)
	}
	if memErr.UsedMB != 12000 || memErr.LimitMB != 8000 {
		t.Errorf("expected used 12000 / limit 8000, got %d / %d", memErr.UsedMB, memErr.LimitMB)
	}
	if !strings.Contains(err.Error(), "resubmit with --memory of at least 12000") {
		t.Errorf("expected advice in the error, got %q", err.Error())
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("job took %s to be killed", time.Since(start))
	}

	data, readErr := os.ReadFile(childPID)
	if readErr != nil {
		t.Fatalf("child pid not written: %v", readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Error("child process survived the kill")
	}
}

func TestJobUnderMemoryLimitCompletes(t *testing.T) {
	e := newLimitedExecutor(fixedProbe(4000))
	out, err := e.RunJobWithEnv("sleep 0.1; echo done", "mem-ok", t.TempDir(), context.Background(), newTestLogger("mem-ok"), nil, 8000)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("expected output 'done', got %q", out)
	}
}

func TestSingleMemorySpikeDoesNotKill(t *testing.T) {
	// One reading over the limit, then back under: below the 2-check grace.
	e := newLimitedExecutor(fixedProbe(12000, 4000))
	if _, err := e.RunJobWithEnv("sleep 0.2", "mem-spike", t.TempDir(), context.Background(), newTestLogger("mem-spike"), nil, 8000); err != nil {
		t.Fatalf("expected success after a single spike, got %v", err)
	}
}

func TestFailedMemoryCheckNeverKills(t *testing.T) {
	failing := func(ctx context.Context, pgid int) (int, error) { return 0, errors.New("nvidia-smi timed out") }
	e := newLimitedExecutor(failing)
	if _, err := e.RunJobWithEnv("sleep 0.2", "mem-unknown", t.TempDir(), context.Background(), newTestLogger("mem-unknown"), nil, 8000); err != nil {
		t.Fatalf("expected success when memory can't be read, got %v", err)
	}
}

func TestNoLimitMeansNoEnforcement(t *testing.T) {
	e := newLimitedExecutor(fixedProbe(999999))
	if _, err := e.RunJobWithEnv("sleep 0.1", "mem-none", t.TempDir(), context.Background(), newTestLogger("mem-none"), nil, 0); err != nil {
		t.Fatalf("expected success with no memory limit, got %v", err)
	}
}

func TestParseComputeApps(t *testing.T) {
	got := parseComputeApps("1234, 5000\n1234, 1000\n5678, [N/A]\n\nNo running processes found\n")
	if len(got) != 1 || got[1234] != 6000 {
		t.Fatalf("expected {1234: 6000}, got %v", got)
	}
}

func TestNvidiaSMIMemoryProbeMatchesByProcessGroup(t *testing.T) {
	// A process in its own group stands in for a job's GPU process.
	job := exec.Command("sleep", "5")
	job.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := job.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = job.Process.Kill(); _ = job.Wait() })
	pid := job.Process.Pid

	// Fake nvidia-smi reports our process plus one from another group (ours).
	smi := filepath.Join(t.TempDir(), "nvidia-smi")
	script := fmt.Sprintf("#!/bin/sh\necho \"%d, 7000\"\necho \"%d, 3000\"\n", pid, os.Getpid())
	if err := os.WriteFile(smi, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake nvidia-smi: %v", err)
	}

	used, err := NvidiaSMIMemoryProbe(smi)(context.Background(), pid)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if used != 7000 {
		t.Errorf("expected only the job's group (7000 MB), got %d", used)
	}
}
