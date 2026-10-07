package executer

import (
	"bytes"
	"context"
	"fmt"
	"gpu-runner/internal/logger"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var executorLogger = logger.Server

type Executor struct {
	cancels    map[string]context.CancelFunc
	mu         sync.RWMutex
	jobTimeout time.Duration

	// GPU memory enforcement; off until SetMemoryLimits is called.
	memProbe       MemoryProbe
	memInterval    time.Duration
	memGraceChecks int
}

func NewExecutor(jobTimeout time.Duration) *Executor {
	return &Executor{
		cancels:    make(map[string]context.CancelFunc),
		jobTimeout: jobTimeout,
	}
}

// GetJobTimeout returns the configured job timeout
func (e *Executor) GetJobTimeout() time.Duration {
	return e.jobTimeout
}

func (e *Executor) RunJob(command, jobID, volumePath string, ctx context.Context, jobLogger logger.JobLogger) (string, error) {
	return e.RunJobWithEnv(command, jobID, volumePath, ctx, jobLogger, nil, 0)
}

// RunJobWithEnv runs a job with extraEnv added to its environment, e.g.
// CUDA_VISIBLE_DEVICES to pin it to one GPU. If memory limits are on and
// memoryLimitMB > 0, the job is killed when it uses more GPU memory than that.
func (e *Executor) RunJobWithEnv(command, jobID, volumePath string, ctx context.Context, jobLogger logger.JobLogger, extraEnv []string, memoryLimitMB int) (string, error) {
	defer e.RemoveCancelFunc(jobID)

	executorLogger.Info("Setting up command execution environment", "volume_path", volumePath)

	// runCtx ends the job on timeout or user cancel (via ctx) or when the
	// memory watcher kills it.
	runCtx, stopRun := context.WithCancel(ctx)
	defer stopRun()

	cmd := exec.CommandContext(runCtx, "bash", "-c", command)
	// Run the job in its own process group, and kill the whole group when it
	// ends early, so children of `bash -c` (where GPU work usually runs) die
	// too and release their GPU memory.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Dir = volumePath
	cmd.Env = append(
		os.Environ(),
		"USER=jobrunner",
		fmt.Sprintf("PATH=%s:%s", volumePath, os.Getenv("PATH")),
	)
	cmd.Env = append(cmd.Env, extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	executorLogger.Info("Executing command", "command", command)

	if err := cmd.Start(); err != nil {
		jobLogger.Error("Command failed to start", logger.Item("error", err))
		return "", fmt.Errorf("command failed to start: %w", err)
	}

	memDone := make(chan *MemoryExceededError, 1)
	if e.memProbe != nil && memoryLimitMB > 0 {
		go e.watchMemory(runCtx, cmd.Process.Pid, memoryLimitMB, stopRun, memDone, jobID)
	} else {
		memDone <- nil
	}

	err := cmd.Wait()
	stopRun() // stop the memory watcher
	if exceeded := <-memDone; exceeded != nil {
		executorLogger.Warn("Job killed for exceeding its GPU memory request", "job_id", jobID, "used_mb", exceeded.UsedMB, "limit_mb", exceeded.LimitMB)
		logMemoryKill(jobLogger, exceeded)
		return stdout.String() + stderr.String(), exceeded
	}

	if err != nil {
		output := stdout.String() + stderr.String()
		exitCode := "unknown"
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = fmt.Sprintf("%d", ee.ExitCode())
		}

		// Check if it was a context cancellation
		if ctx.Err() == context.Canceled {
			jobLogger.Info("Command execution cancelled", logger.Item("exit_code", exitCode))
			executorLogger.Info("Job cancelled by context", "job_id", jobID)
		} else if ctx.Err() == context.DeadlineExceeded {
			jobLogger.Error("Command execution timed out", logger.Item("exit_code", exitCode))
			executorLogger.Warn("Job timed out", "job_id", jobID)
		} else {
			jobLogger.Error("Command execution failed",
				logger.Item("exit_code", exitCode),
				logger.Item("error", err),
				logger.Item("stderr", stderr.String()))
		}

		return output, fmt.Errorf("command failed (exit %s): %s\nstderr:\n%s", exitCode, command, stderr.String())
	}

	output := stdout.String() + stderr.String()
	jobLogger.Info("Successfully executed command", logger.Item("output_length", len(output)))
	return output, nil
}

func (e *Executor) SetCancelFunc(jobID string, cancel context.CancelFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cancels[jobID] = cancel
}

func (e *Executor) RemoveCancelFunc(jobID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.cancels, jobID)
}

func (e *Executor) CancelJob(jobID string) error {
	e.mu.RLock()
	cancel := e.cancels[jobID]
	e.mu.RUnlock()

	if cancel == nil {
		executorLogger.Warn("Attempted to cancel non-existent or already completed job", "job_id", jobID)
		return fmt.Errorf("job %s cannot be cancelled because it does not exist", jobID)
	}

	executorLogger.Info("Cancelling job execution", "job_id", jobID)
	cancel()
	return nil
}
