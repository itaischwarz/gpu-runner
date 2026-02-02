package executer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gpu-runner/internal/logger"
)

// noopSink implements logger.StreamSink and discards all logs.
type noopSink struct{}

func (n *noopSink) Append(_ context.Context, _, _ string) error { return nil }

func newTestLogger(jobID string) logger.JobLogger {
	return *logger.NewJobLogger(context.Background(), jobID, &noopSink{})
}

func TestRunJobSuccess(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	ctx := context.Background()
	jl := newTestLogger("test-1")

	output, err := e.RunJob("echo hello world", "test-1", tmpDir, ctx, jl)
	if err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", output)
	}
}

func TestRunJobFailure(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	ctx := context.Background()
	jl := newTestLogger("test-2")

	_, err := e.RunJob("exit 1", "test-2", tmpDir, ctx, jl)
	if err == nil {
		t.Fatal("expected error for failing command")
	}
	if !strings.Contains(err.Error(), "exit") {
		t.Errorf("expected exit error, got: %v", err)
	}
}

func TestRunJobCapturesStderr(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	ctx := context.Background()
	jl := newTestLogger("test-3")

	output, err := e.RunJob("echo error-output >&2 && exit 1", "test-3", tmpDir, ctx, jl)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(output, "error-output") {
		t.Errorf("expected stderr in output, got %q", output)
	}
}

func TestRunJobContextCancellation(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	jl := newTestLogger("test-4")
	e.SetCancelFunc("test-4", cancel)

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err := e.RunJob("sleep 30", "test-4", tmpDir, ctx, jl)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRunJobTimeout(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	jl := newTestLogger("test-5")

	_, err := e.RunJob("sleep 30", "test-5", tmpDir, ctx, jl)
	if err == nil {
		t.Fatal("expected error from timeout")
	}
}

func TestRunJobWorkingDirectory(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	// Create a file in tmpDir to verify working directory
	if err := os.WriteFile(tmpDir+"/marker.txt", []byte("found"), 0644); err != nil {
		t.Fatalf("failed to create marker file: %v", err)
	}

	ctx := context.Background()
	jl := newTestLogger("test-6")

	output, err := e.RunJob("cat marker.txt", "test-6", tmpDir, ctx, jl)
	if err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}
	if !strings.Contains(output, "found") {
		t.Errorf("expected output 'found', got %q", output)
	}
}

func TestCancelFuncLifecycle(t *testing.T) {
	e := NewExecutor(30 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e.SetCancelFunc("job-1", cancel)

	// Should be able to cancel
	if err := e.CancelJob("job-1"); err != nil {
		t.Fatalf("CancelJob failed: %v", err)
	}

	// Verify context was cancelled
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

func TestCancelNonExistentJob(t *testing.T) {
	e := NewExecutor(30 * time.Second)

	err := e.CancelJob("does-not-exist")
	if err == nil {
		t.Fatal("expected error for non-existent job")
	}
}

func TestRemoveCancelFunc(t *testing.T) {
	e := NewExecutor(30 * time.Second)

	_, cancel := context.WithCancel(context.Background())
	e.SetCancelFunc("job-2", cancel)

	e.RemoveCancelFunc("job-2")

	err := e.CancelJob("job-2")
	if err == nil {
		t.Fatal("expected error after removing cancel func")
	}
}

func TestRunJobRemovesCancelFunc(t *testing.T) {
	e := NewExecutor(30 * time.Second)
	tmpDir := t.TempDir()

	ctx := context.Background()
	_, cancel := context.WithCancel(ctx)
	e.SetCancelFunc("test-cleanup", cancel)

	jl := newTestLogger("test-cleanup")
	_, err := e.RunJob("echo done", "test-cleanup", tmpDir, ctx, jl)
	if err != nil {
		t.Fatalf("RunJob failed: %v", err)
	}

	// After RunJob, the cancel func should be removed via defer
	if err := e.CancelJob("test-cleanup"); err == nil {
		t.Error("expected cancel func to be removed after RunJob completes")
	}
}
