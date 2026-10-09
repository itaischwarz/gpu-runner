package jobs

import (
	"context"
	"errors"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/logger"
	"time"
)

var workerLogger = logger.Server

type Worker struct {
	ID       int
	JobQueue *JobQueue // still provides the Executor
	Results  chan *Job
	Inbox    <-chan *Job // where jobs arrive; defaults to JobQueue.Queue
	Env      []string    // extra env for every job, e.g. CUDA_VISIBLE_DEVICES=<uuid>
	OnIdle   func(*Job)  // called after each job with that job, e.g. to release the GPU slot
}

// NewWorker creates a worker that reads the shared JobQueue. GPU workers
// replace Inbox, Env and OnIdle before Start.
func NewWorker(id int, jq *JobQueue, results chan *Job) *Worker {
	workerLogger.Info("Creating new worker", "worker_id", id)
	return &Worker{
		ID:       id,
		JobQueue: jq,
		Results:  results,
		Inbox:    jq.Queue,
	}
}

// finish reports the job's result, then tells whoever owns this worker's
// slot that it is free again.
func (w *Worker) finish(job *Job) {
	w.Results <- job
	if w.OnIdle != nil {
		w.OnIdle(job)
	}
}

func (w *Worker) Start(ctx context.Context) {
	workerLogger.Info("Starting worker", "worker_id", w.ID)
	go func() {
		for {
			select {
			case <-ctx.Done():
				workerLogger.Info("Worker shutting down", "worker_id", w.ID)
				return
			case job, ok := <-w.Inbox:
				if !ok {
					// The inbox was closed on shutdown; a nil job would panic below.
					workerLogger.Info("Worker inbox closed, shutting down", "worker_id", w.ID)
					return
				}
				job.Status = StatusRunning
				workerLogger.Info("Worker received job from queue", "worker_id", w.ID, "job_id", job.ID, "status", job.Status)
				job.Logger.Info("Job Running", logger.Item("Job Status", job.Status), logger.Item("worker", w.ID), logger.Item("command", job.Command))
				volumePath := VolumePaths[job.StorageBytes]
				jobTimeout := w.JobQueue.Executor.GetJobTimeout()
				jobCtx, cancel := context.WithTimeout(ctx, jobTimeout)
				workerLogger.Info("Setting up job execution context", "worker_id", w.ID, "job_id", job.ID, "volume_path", volumePath, "timeout", jobTimeout)

				w.JobQueue.Executor.SetCancelFunc(job.ID, cancel)

				workerLogger.Info("Executing job command", "worker_id", w.ID, "job_id", job.ID)
				output, err := w.JobQueue.Executor.RunJobWithEnv(job.Command, job.ID, volumePath, jobCtx, *job.Logger, w.Env, job.MemoryMB)
				// Read before cancel(): afterwards jobCtx always reports Canceled.
				userCancelled := errors.Is(jobCtx.Err(), context.Canceled)
				cancel()

				if err != nil {
					job.Status = StatusFailed
					job.Error = err.Error()
					var memErr *executer.MemoryExceededError
					if errors.As(err, &memErr) {
						job.NoRetry = true
					}
					if userCancelled {
						// Cancelled by the user: keep it cancelled, don't retry.
						job.Status = StatusCancelled
						job.NoRetry = true
					}
					workerLogger.Error("Job execution failed", "worker_id", w.ID, "job_id", job.ID, "error", err)
					job.Logger.Error("Job did not complete successfully",
						logger.Item("error", err.Error()),
						logger.Item("volume_path", volumePath),
						logger.Item("job_id", job.ID),
					)
					w.finish(job)
					continue
				}

				workerLogger.Info("Job execution completed successfully", "worker_id", w.ID, "job_id", job.ID, "output_length", len(output))
				time.Sleep(1 * time.Second)
				job.Status = StatusSuccess
				job.Error = ""
				job.Logger.Info("Completed job", logger.Item("status", job.Status), logger.Item("worker_id", w.ID), logger.Item("command", job.Command))
				w.finish(job)
			}
		}
	}()
}
