package jobs

import "time"

type JobStatus string

const (
    StatusPending  JobStatus = "pending"
    StatusRunning  JobStatus = "running"
    StatusSuccess  JobStatus = "success"
    StatusFailed   JobStatus = "failed"
)

type Job struct {
    ID        string     `json:"id"`
    Command   string     `json:"command"`
    Status    JobStatus  `json:"status"`
    Log       string     `json:"log"`
    CreatedAt time.Time  `json:"created_at"`
}