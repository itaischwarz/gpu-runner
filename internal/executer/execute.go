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

	if err := cmd.Run(); err != nil {
		output := stdout.String() + stderr.String()
		exitCode := "unknown"
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = fmt.Sprintf("%d", ee.ExitCode())
		}

		jobLogger.Error("Unable to run job",
			logger.Item("err", err),
			logger.Item("exitCode", exitCode),
			logger.Item("command", command,),
			logger.Item("dir", volumePath),
			logger.Item("stdout", stdout.String()),
			logger.Item("stderr", stderr.String()),
		)

		return output, fmt.Errorf("command failed (exit %s): %s\nstderr:\n%s", exitCode, command, stderr.String())
	}
	output := stdout.String() + stderr.String()
	jobLogger.Info("Succesfully Ran Job",  logger.Item("command", command))
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
			return fmt.Errorf("job %s cannot be cancelled because it does not exist", jobID)
	 	}

		cancel()
		return nil


}
