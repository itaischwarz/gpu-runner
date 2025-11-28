package jobs

import (
	"fmt"
	"gpu-runner/internal/executer"
	"time"
)

type Worker struct {
    ID       int
    JobQueue *JobQueue
}

func NewWorker(id int, jq *JobQueue) *Worker {
    return &Worker{
        ID:       id,
        JobQueue: jq,
    }
}

func (w *Worker) Start() {
    go func() {
        for job := range w.JobQueue.Queue {
            job.Status = StatusRunning
            job.Log = fmt.Sprintf("Worker %d: Executing %s\n", w.ID, job.Command)
            volumePath := VolumePaths[job.StorageBytes]
            executer.RunCommand(job.Command, job.ID, volumePath)
            // Simulate GPU job duration
            time.Sleep(1 * time.Second)

            job.Status = StatusSuccess
            job.Log += "Completed.\n"
        }
    }()
}
