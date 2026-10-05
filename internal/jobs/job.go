package jobs

import (
	"gpu-runner/internal/logger"
	"time"
)

type JobStatus string

type JobStorage int

type JobPriority string

// JobResource selects which pool (infrastructure) a job runs on.
type JobResource string

type Job struct {
	ID           string            `json:"id"`
	Command      string            `json:"command"`
	Status       JobStatus         `json:"status"`
	Logger       *logger.JobLogger `json:"logger"`
	Log 		      string 					 `json:"log"`
	CreatedAt    time.Time         `json:"created_at"`
	StorageBytes JobStorage        `json:"storage"`
	VolumePath   string            `json:"volume_path"`
	StartedAt    string            `json:"started_at"`
	FinishedAt   string            `json:"finished_at"`
	MaxRetries   int               `json:"max_retries"`
	JobTrial     int               `json:"job_trial"`
	// MemoryMB is the GPU memory the job asks for; the dispatcher only places
	// it on a GPU with at least this much free. 0 means no requirement.
	MemoryMB int `json:"memory_mb"`
}
