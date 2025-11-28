package executer

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

func RunCommand(command, jobID, volumePath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

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
