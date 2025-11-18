package executer

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"time"
)


func RunCommand(command, jobID string) (string, error)  {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

  cmd := exec.CommandContext(ctx, "bash", "-c", command)
	logger1 := slog.New(slog.NewJSONHandler(
		os.Stdout,
		&slog.HandlerOptions{Level:slog.LevelDebug},
	))
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=/tmp/job-" + jobID,
		"USER=jobrunner",
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr


	if err := cmd.Run(); err != nil {
		output := stdout.String() + stderr.String()
		logger1.Error("Unable to run job", "err", err)
		return output, err
	}
	output := stdout.String() + stderr.String()
	logger1.Info("Succesfully Ran Job", "command", command)
	return output, nil

 

}


		
