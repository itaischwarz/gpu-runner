package logger

import (
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

