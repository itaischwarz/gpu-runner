package logger

import (
    "log"
    "os"
    "io"
    "log/slog"
)

var Server *slog.Logger


func init() {
    serverPath, err := os.OpenFile("/temp/var/gpu-runner/log/server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

    if err != nil {
        log.Fatalf("failed to open server log: %v", err)
    }

    handler := slog.NewTextHandler(
        io.MultiWriter(os.Stdout, serverPath),
        &slog.HandlerOptions{Level: slog.LevelInfo},
    )

    Server = slog.New(handler)
		jobPath, err := os.OpenFile("/temp/var/gpu-runner/log/temp.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644))
		if err != nil {
        log.Fatalf("failed to open server log: %v", err)
    }
		handler := slog.NewJSONHandler(
			io.MultiWriter(os.Stdout, serverPath),
			&slog.HandlerOptions{Level: slog.LevelInfo},
		)


}