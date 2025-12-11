package jobs


const (
    Volume10MB JobStorage = 10 * 1024 * 1024
    Volume25MB JobStorage = 25 * 1024 * 1024
    Volume50MB JobStorage = 50 * 1024 * 1024
)

var VolumePaths = map[JobStorage] string{
	Volume10MB:"/var/lib/jobrunner/volumes/10mb",
	Volume25MB:"/var/lib/jobrunner/volumes/25mb",
	Volume50MB:"/var/lib/jobrunner/volumes/50mb",
}


const (
    StatusPending  JobStatus = "pending"
    StatusRunning  JobStatus = "running"
    StatusSuccess  JobStatus = "success"
    StatusFailed   JobStatus = "failed"
)
