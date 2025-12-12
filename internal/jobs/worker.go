package jobs

import (
	"context"
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
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            w.JobQueue.Executor.SetCancelFunc(job.ID, cancel)
            w.JobQueue.Executor.RunJob(job.Command, job.ID, volumePath, &ctx)
            // Simulate GPU job duration
            time.Sleep(1 * time.Second)

            job.Status = StatusSuccess
            job.Log += "Completed.\n"
            logger := job.Logger 
            logger.Info(job.Log)
        }
    }()
}