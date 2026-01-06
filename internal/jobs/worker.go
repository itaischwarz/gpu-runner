package jobs

import (
	"context"
	"gpu-runner/internal/logger"
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

func (w *Worker) Start(ctx context.Context) {
    go func() {
        for {
            select {
            case <- ctx.Done():
                return
            case job := <-w.JobQueue.Queue:
                job.Status = StatusRunning
                job.Logger.Info("Job Running", logger.Item("Job Status", job.Status) , logger.Item("worker", w.ID),  logger.Item("command", job.Command))
                volumePath := VolumePaths[job.StorageBytes]
                jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
                w.JobQueue.Executor.SetCancelFunc(job.ID, cancel)
                w.JobQueue.Executor.RunJob(job.Command, job.ID, volumePath, jobCtx, *job.Logger)
                time.Sleep(1 * time.Second)

                job.Status = StatusSuccess
                job.Logger.Info("Completed Job", logger.Item("Job Status", job.Status), logger.Item("worker", w.ID),  logger.Item("command", job.Command))
            }
        }
    }()
}