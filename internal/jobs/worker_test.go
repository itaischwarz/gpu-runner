package jobs

import (
	"context"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/logger"
	"strings"
	"testing"
	"time"
)

type noopSink struct{}

func (noopSink) Append(_ context.Context, _, _ string) error { return nil }

func newTestJob(id, command string) *Job {
	return &Job{ID: id, Command: command, Logger: logger.NewJobLogger(context.Background(), id, noopSink{})}
}

func TestGPUWorkerRunsInboxJobsWithEnvThenCallsOnIdle(t *testing.T) {
	jq := NewJobQueue(1)
	jq.Executor = executer.NewExecutor(10 * time.Second)
	results := make(chan *Job, 2)
	inbox := make(chan *Job, 1)
	idle := make(chan struct{}, 2)

	w := NewWorker(1, jq, results)
	w.Inbox = inbox
	w.Env = []string{"CUDA_VISIBLE_DEVICES=GPU-aaa"}
	w.OnIdle = func(*Job) { idle <- struct{}{} }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	// Succeeds only if the worker passed its Env to the job.
	inbox <- newTestJob("ok", `test "$CUDA_VISIBLE_DEVICES" = GPU-aaa`)
	if job := waitForResult(t, results); job.Status != StatusSuccess {
		t.Fatalf("expected success, got %s", job.Status)
	}
	waitForIdle(t, idle)

	// A failed job must release the slot too.
	inbox <- newTestJob("fail", "exit 1")
	if job := waitForResult(t, results); job.Status != StatusFailed {
		t.Fatalf("expected failure, got %s", job.Status)
	}
	waitForIdle(t, idle)

	// The shared CPU queue is not read by a worker with its own inbox.
	jq.Queue <- newTestJob("cpu", "true")
	select {
	case job := <-results:
		t.Fatalf("GPU worker took a job from the shared queue: %s", job.ID)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNewWorkerDefaultsToSharedQueue(t *testing.T) {
	jq := NewJobQueue(1)
	w := NewWorker(1, jq, make(chan *Job))
	if w.Inbox != (<-chan *Job)(jq.Queue) {
		t.Error("expected a new worker to read the shared JobQueue by default")
	}
}

func waitForResult(t *testing.T, results chan *Job) *Job {
	t.Helper()
	select {
	case job := <-results:
		return job
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a job result")
		return nil
	}
}

func waitForIdle(t *testing.T, idle chan struct{}) {
	t.Helper()
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for OnIdle")
	}
}

func TestWorkerMarksMemoryKillAsFailedWithoutRetry(t *testing.T) {
	jq := NewJobQueue(1)
	jq.Executor = executer.NewExecutor(10 * time.Second)
	overLimit := func(ctx context.Context, pgid int) (int, error) { return 12000, nil }
	jq.Executor.SetMemoryLimits(overLimit, 10*time.Millisecond, 2)

	results := make(chan *Job, 1)
	inbox := make(chan *Job, 1)
	w := NewWorker(1, jq, results)
	w.Inbox = inbox

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	job := newTestJob("hog", "sleep 30")
	job.MemoryMB = 8000
	inbox <- job

	got := waitForResult(t, results)
	if got.Status != StatusFailed || !got.NoRetry {
		t.Fatalf("expected failed and not retried, got status=%s noRetry=%v", got.Status, got.NoRetry)
	}
	if !strings.Contains(got.Error, "GPU memory exceeded: used 12000 MB of 8000 MB requested") {
		t.Errorf("expected the memory error on the job, got %q", got.Error)
	}
}

func TestWorkerKeepsUserCancelledJobCancelled(t *testing.T) {
	jq := NewJobQueue(1)
	jq.Executor = executer.NewExecutor(10 * time.Second)
	results := make(chan *Job, 1)
	inbox := make(chan *Job, 1)
	w := NewWorker(1, jq, results)
	w.Inbox = inbox

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	inbox <- newTestJob("c1", "sleep 30")

	// Cancel through the executor, the way the API's cancel endpoint does.
	deadline := time.Now().Add(2 * time.Second)
	for jq.Executor.CancelJob("c1") != nil {
		if time.Now().After(deadline) {
			t.Fatal("job never became cancellable")
		}
		time.Sleep(10 * time.Millisecond)
	}

	got := waitForResult(t, results)
	if got.Status != StatusCancelled || !got.NoRetry {
		t.Fatalf("expected cancelled and not retried, got status=%s noRetry=%v", got.Status, got.NoRetry)
	}
}
