package executer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"gpu-runner/internal/logger"
)

var executorLogger = logger.Server

type Executor struct {
	cancels map[string]context.CancelFunc
	mu      sync.RWMutex
}

func NewExecutor() *Executor {
	return &Executor{
		cancels: make(map[string]context.CancelFunc),
	}
}

func (e *Executor) RunJob(command, jobID, volumePath string, ctx context.Context, jobLogger logger.JobLogger) (string, error) {
	defer e.RemoveCancelFunc(jobID)

	jobLogger.Info("Setting up command execution environment", logger.Item("volume_path", volumePath))

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = volumePath
	cmd.Env = append(
		os.Environ(),
		"USER=jobrunner",
		fmt.Sprintf("PATH=%s:%s", volumePath, os.Getenv("PATH")),
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	jobLogger.Info("Executing command", logger.Item("command", command))

	if err := cmd.Run(); err != nil {
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

	if cancel == nil{
		executorLogger.Warn("Attempted to cancel non-existent or already completed job", "job_id", jobID)
		return fmt.Errorf("job %s cannot be cancelled because it does not exist", jobID)
	}

	executorLogger.Info("Cancelling job execution", "job_id", jobID)
	cancel()
	return nil
}
