package box

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"gpu-runner/internal/controlplane"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
)

type fakeGPUs struct {
	slots []controlplane.Slot
	err   error
}

func (f *fakeGPUs) Resource() jobs.JobResource { return jobs.ResourceGPU }
func (f *fakeGPUs) Poll(context.Context) ([]controlplane.Slot, error) {
	return f.slots, f.err
}
func (f *fakeGPUs) Env(string) []string { return nil }

// recordSink keeps a job's log lines so tests can check what reached them.
type recordSink struct {
	mu    sync.Mutex
	lines []string
}

func (s *recordSink) Append(_ context.Context, _, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, data)
	return nil
}

func (s *recordSink) contains(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// startBox runs a box server in-process and returns a client for it.
func startBox(t *testing.T, gpus *fakeGPUs, exec *executer.Executor) *Client {
	t.Helper()
	srv := httptest.NewServer(NewHandler(gpus, exec, t.TempDir()))
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func run(c *Client, ctx context.Context, sink *recordSink, command string, env []string, memoryMB int) (string, error) {
	jl := logger.NewJobLogger(ctx, "job-1", sink)
	return c.RunJobWithEnv(command, "job-1", "/ignored", ctx, *jl, env, memoryMB)
}

func TestPollReturnsTheBoxGPUs(t *testing.T) {
	gpus := &fakeGPUs{slots: []controlplane.Slot{
		{ID: "GPU-a", Model: "RTX 4090", TotalMemoryMB: 49140, FreeMemoryMB: 48000, Healthy: true},
	}}
	c := startBox(t, gpus, executer.NewExecutor(time.Minute))

	slots, err := c.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll failed: %v", err)
	}
	if len(slots) != 1 || slots[0] != gpus.slots[0] {
		t.Fatalf("expected %v, got %v", gpus.slots, slots)
	}

	gpus.err = errors.New("nvidia-smi failed")
	if _, err := c.Poll(context.Background()); err == nil {
		t.Error("expected an error when the box's nvidia-smi fails")
	}
}

func TestPollFailsWhenBoxIsDown(t *testing.T) {
	srv := httptest.NewServer(NewHandler(&fakeGPUs{}, executer.NewExecutor(time.Minute), t.TempDir()))
	c := New(srv.URL)
	srv.Close()

	if _, err := c.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected an unreachable error, got %v", err)
	}
}

func TestRunReturnsOutputAndAppliesEnv(t *testing.T) {
	c := startBox(t, &fakeGPUs{}, executer.NewExecutor(time.Minute))
	sink := &recordSink{}

	out, err := run(c, context.Background(), sink, `echo "pinned to $CUDA_VISIBLE_DEVICES"`, c.Env("GPU-a"), 0)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if strings.TrimSpace(out) != "pinned to GPU-a" {
		t.Errorf("expected the job to see its GPU, got %q", out)
	}
	if !sink.contains("Successfully executed command") {
		t.Error("expected the success to reach the job's log")
	}
}

func TestRunReportsCommandFailure(t *testing.T) {
	c := startBox(t, &fakeGPUs{}, executer.NewExecutor(time.Minute))
	sink := &recordSink{}

	_, err := run(c, context.Background(), sink, "echo oops >&2; exit 3", nil, 0)
	if err == nil || !strings.Contains(err.Error(), "exit 3") || !strings.Contains(err.Error(), "oops") {
		t.Fatalf("expected the exit code and stderr in the error, got %v", err)
	}
	var memErr *executer.MemoryExceededError
	if errors.As(err, &memErr) {
		t.Error("a plain failure must not look like a memory kill")
	}
	if !sink.contains("Command execution failed") {
		t.Error("expected the failure to reach the job's log")
	}
}

func TestRunMemoryKillComesBackAsMemoryExceededError(t *testing.T) {
	exec := executer.NewExecutor(time.Minute)
	exec.SetMemoryLimits(func(context.Context, int) (int, error) { return 5000, nil }, 20*time.Millisecond, 2)
	c := startBox(t, &fakeGPUs{}, exec)
	sink := &recordSink{}

	_, err := run(c, context.Background(), sink, "sleep 10", nil, 1000)
	var memErr *executer.MemoryExceededError
	if !errors.As(err, &memErr) {
		t.Fatalf("expected a MemoryExceededError so the job isn't retried, got %v", err)
	}
	if memErr.UsedMB != 5000 || memErr.LimitMB != 1000 {
		t.Errorf("expected used 5000 of 1000, got %+v", memErr)
	}
	if !sink.contains("GPU memory exceeded") {
		t.Error("expected the memory kill to reach the job's log")
	}
}

func TestCancelKillsTheJobOnTheBox(t *testing.T) {
	c := startBox(t, &fakeGPUs{}, executer.NewExecutor(time.Minute))
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := run(c, ctx, &recordSink{}, "echo $$ > "+pidFile+"; sleep 30", nil, 0)
		done <- err
	}()

	pid := waitForPID(t, pidFile)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected a cancelled error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not return after cancel")
	}

	// The box notices the request closed and kills the job.
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("job process %d still running after cancel", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunFailsWhenBoxIsDown(t *testing.T) {
	srv := httptest.NewServer(NewHandler(&fakeGPUs{}, executer.NewExecutor(time.Minute), t.TempDir()))
	c := New(srv.URL)
	srv.Close()

	_, err := run(c, context.Background(), &recordSink{}, "true", nil, 0)
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected an unreachable error, got %v", err)
	}
	var memErr *executer.MemoryExceededError
	if errors.As(err, &memErr) {
		t.Error("an unreachable box must stay retryable")
	}
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				return pid
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job never wrote its pid")
	return 0
}
