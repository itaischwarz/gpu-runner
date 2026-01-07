package jobs

import (
	"context"
	"gpu-runner/internal/logger"
	"time"
    "fmt"

)

type Worker struct {
    ID       int
    JobQueue *JobQueue
    Results chan *Job

}



func NewWorker(id int, jq *JobQueue, results chan *Job) *Worker {
    return &Worker{
        ID:       id,
        JobQueue: jq,
        Results: results,
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
                fmt.Println(job.Status)
                job.Logger.Info("Job Running", logger.Item("Job Status", job.Status) , logger.Item("worker", w.ID),  logger.Item("command", job.Command))
                fmt.Println("logger is real")
                volumePath := VolumePaths[job.StorageBytes]
                jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
                fmt.Println("jobCtx is real")

                w.JobQueue.Executor.SetCancelFunc(job.ID, cancel)
                 fmt.Println("set cancelfunc is real")
                _, err := w.JobQueue.Executor.RunJob(job.Command, job.ID, volumePath, jobCtx, *job.Logger)
                 fmt.Println("job ran")
                if err != nil{
                    job.Logger.Error("Job did not complete succesfully",
                        logger.Item("err", err),
                        logger.Item("dir", volumePath),
                        logger.Item("jobID", job.ID),
                    )
                    w.Results <- job 
                }
                time.Sleep(1 * time.Second)
                job.Status = StatusSuccess
                job.Logger.Info("Completed Job", logger.Item("Job Status", job.Status), logger.Item("worker", w.ID),  logger.Item("command", job.Command))
                w.Results <- job 
            }
        }
    }()
}