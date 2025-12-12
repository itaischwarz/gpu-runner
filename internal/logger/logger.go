package logger

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
)

var Server *slog.Logger



func init() {
    serverPath, err := os.OpenFile("/Users/itaischwarz/log/gpu-runner/server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

    if err != nil {
        log.Fatalf("failed to open server log: %v", err)
    }

    handler := slog.NewTextHandler(
        io.MultiWriter(os.Stdout, serverPath),
        &slog.HandlerOptions{Level: slog.LevelInfo},
    )



    Server = slog.New(handler)

}
func CreateJobLogger(job_id string) *slog.Logger {
    jobPath, err := os.OpenFile(fmt.Sprintf("/Users/itaischwarz/log/gpu-runner/job%s.log", job_id), os.O_APPEND | os.O_CREATE | os.O_WRONLY, 0644)

    if err != nil {
        log.Fatalf("Unable to open job log: job %s", job_id)
    }

    handler := slog.NewJSONHandler(
        io.MultiWriter(os.Stdout, jobPath),
        &slog.HandlerOptions{Level: slog.LevelInfo},
    )

    return slog.New(handler)

}
