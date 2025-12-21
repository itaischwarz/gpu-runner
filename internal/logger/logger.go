package logger

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
)

var Server *slog.Logger

func init() {
	baseDir := filepath.Join("~", "log", "gpu-runner")
	_ = os.MkdirAll(baseDir, 0o755)

	serverPath, err := os.OpenFile(filepath.Join(baseDir, "server.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("failed to open server log: %v", err)
	}

	handler := slog.NewTextHandler(
		io.MultiWriter(os.Stdout, serverPath),
		&slog.HandlerOptions{Level: slog.LevelInfo},
	)

	Server = slog.New(handler)
}

func CreateJobLogger(jobID string) *slog.Logger {
    home, err := os.UserHomeDir()
    if err != nil{
        Server.Error("Unable to open home directory ")
    }
	baseDir := filepath.Join(home, "log", "gpu-runner")
	_ = os.MkdirAll(baseDir, 0o755)
	jobPath, err := os.OpenFile(filepath.Join(baseDir, fmt.Sprintf("job%s.log", jobID)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("Unable to open job log: job %s", jobID)
	}

	handler := slog.NewJSONHandler(
		io.MultiWriter(os.Stdout, jobPath),
		&slog.HandlerOptions{Level: slog.LevelInfo},
	)

	return slog.New(handler)
}
