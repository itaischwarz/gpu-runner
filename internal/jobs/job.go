package jobs

import "time"

type JobStatus string




type JobStorage int



type Job struct {
    ID        string     `json:"id"`
    Command   string     `json:"command"`
    Status    JobStatus  `json:"status"`
    Log       string     `json:"log"`
    CreatedAt time.Time  `json:"created_at"`
    StorageBytes   JobStorage `json:"storage"`
    VolumePath string    `json:"volume_path"`
    StartedAt string     `json:"started_at"`
    FinishedAt string    `json:"finished_at"`
}