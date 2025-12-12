package executer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
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

func (e *Executor) RunJob(command, jobID, volumePath string, ctx context.Context) (string, error) {
	defer e.RemoveCancelFunc(jobID)
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	logger1 := slog.New(slog.NewJSONHandler(
		os.Stdout,
	&slog.HandlerOptions{Level: slog.LevelDebug},
	))

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

		logger1.Error("Unable to run job",
			"err", err,
			"exitCode", exitCode,
			"command", command,
			"dir", volumePath,
			"stdout", stdout.String(),
			"stderr", stderr.String(),
		)

		return output, fmt.Errorf("command failed (exit %s): %s\nstderr:\n%s", exitCode, command, stderr.String())
	}
	output := stdout.String() + stderr.String()
	logger1.Info("Succesfully Ran Job", "command", command)
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
