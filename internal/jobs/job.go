package jobs

import (
	"gpu-runner/internal/logger"
	"time"
)

type JobStatus string

type JobStorage int

type Job struct {
	ID           string            `json:"id"`
	Command      string            `json:"command"`
	Status       JobStatus         `json:"status"`
	Logger       *logger.JobLogger `json:"logger"`
	CreatedAt    time.Time         `json:"created_at"`
	StorageBytes JobStorage        `json:"storage"`
	VolumePath   string            `json:"volume_path"`
	StartedAt    string            `json:"started_at"`
	FinishedAt   string            `json:"finished_at"`
	MaxRetries   int               `json:"max_retries"`
	JobTrial     int               `json:"job_trial"`
}
