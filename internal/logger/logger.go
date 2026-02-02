package logger

import (
	"gpu-runner/internal/config"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var Server *slog.Logger

func init() {
	// Initialize with default config for tests and early startup
	// Should be re-initialized with proper config via InitLogger
	Server = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// InitLogger initializes the server logger with the provided configuration
func InitLogger(cfg *config.LoggerConfig) error {
	_ = os.MkdirAll(cfg.LogDir, 0o755)

	logPath := filepath.Join(cfg.LogDir, cfg.LogFile)
	serverPath, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	// Parse log level
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
		log.Printf("Unknown log level '%s', defaulting to 'info'", cfg.Level)
	}

	handler := slog.NewTextHandler(
		io.MultiWriter(os.Stdout, serverPath),
		&slog.HandlerOptions{Level: level},
	)

	Server = slog.New(handler)
	return nil
}
