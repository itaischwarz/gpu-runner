package jobs

import "gpu-runner/internal/config"

const (
	Volume10MB JobStorage = 10 * 1024 * 1024
	Volume25MB JobStorage = 25 * 1024 * 1024
	Volume50MB JobStorage = 50 * 1024 * 1024
)

var VolumePaths = map[JobStorage]string{
	Volume10MB: "/var/lib/jobrunner/volumes/10mb",
	Volume25MB: "/var/lib/jobrunner/volumes/25mb",
	Volume50MB: "/var/lib/jobrunner/volumes/50mb",
}

// InitVolumePaths initializes volume paths from configuration
func InitVolumePaths(cfg *config.StorageConfig) {
	VolumePaths[Volume10MB] = cfg.Volume10MBPath
	VolumePaths[Volume25MB] = cfg.Volume25MBPath
	VolumePaths[Volume50MB] = cfg.Volume50MBPath
}

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusSuccess   JobStatus = "success"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

const (     
	ImmediatePriority JobPriority = "highest"
 	HighPriority JobPriority = "high"
	MediumPriority JobPriority = "medium"
	LowPriority JobPriority= "low"
)