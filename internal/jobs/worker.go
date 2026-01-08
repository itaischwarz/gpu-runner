package jobs

import (
	"context"
	"gpu-runner/internal/logger"
	"time"
)

var workerLogger = logger.Server

type Worker struct {
    ID       int
    JobQueue *JobQueue
    Results chan *Job
}

func NewWorker(id int, jq *JobQueue, results chan *Job) *Worker {
    workerLogger.Info("Creating new worker", "worker_id", id)
    return &Worker{
        ID:       id,
        JobQueue: jq,
        Results: results,
    }
}

func (w *Worker) Start(ctx context.Context) {
    workerLogger.Info("Starting worker", "worker_id", w.ID)
    go func() {
        for {
            select {
            case <- ctx.Done():
                workerLogger.Info("Worker shutting down", "worker_id", w.ID)
                return
            case job := <-w.JobQueue.Queue:
                job.Status = StatusRunning
                workerLogger.Info("Worker received job from queue", "worker_id", w.ID, "job_id", job.ID, "status", job.Status)
                job.Logger.Info("Job Running", logger.Item("Job Status", job.Status) , logger.Item("worker", w.ID),  logger.Item("command", job.Command))
                volumePath := VolumePaths[job.StorageBytes]
                jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
                workerLogger.Info("Setting up job execution context", "worker_id", w.ID, "job_id", job.ID, "volume_path", volumePath)

                w.JobQueue.Executor.SetCancelFunc(job.ID, cancel)

                workerLogger.Info("Executing job command", "worker_id", w.ID, "job_id", job.ID)
                output, err := w.JobQueue.Executor.RunJob(job.Command, job.ID, volumePath, jobCtx, *job.Logger)

                if err != nil{
                    job.Status = StatusFailed
                    workerLogger.Error("Job execution failed", "worker_id", w.ID, "job_id", job.ID, "error", err)
                    job.Logger.Error("Job did not complete successfully",
                        logger.Item("error", err),
                        logger.Item("volume_path", volumePath),
                        logger.Item("job_id", job.ID),
                    )
                    w.Results <- job
                    continue
                }

                workerLogger.Info("Job execution completed successfully", "worker_id", w.ID, "job_id", job.ID, "output_length", len(output))
                time.Sleep(1 * time.Second)
                job.Status = StatusSuccess
                job.Logger.Info("Completed job", logger.Item("status", job.Status), logger.Item("worker_id", w.ID), logger.Item("command", job.Command))
                w.Results <- job 
            }
        }
    }()
}