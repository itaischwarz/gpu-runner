package api

import (
	"context"
	"encoding/json"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"gpu-runner/internal/store"
	"net/http"
	"time"

	"gpu-runner/internal/redis"

	"github.com/gorilla/mux"
)

var ServerLogger = logger.Server

type Handlers struct {
    Queue     *jobs.JobQueue
    JobStore  *store.JobStore
    ctx       context.Context
    StreamSink    *redis.StreamSink 
    Client        *redis.Client
}

func NewHandlers(queue *jobs.JobQueue, store *store.JobStore, context context.Context, streamSink *redis.StreamSink, client *redis.Client) *Handlers {
    return &Handlers{
        Queue:    queue,
        JobStore:  store,
        ctx: context,
        StreamSink: streamSink,
        Client: client,
    }
}



func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
    ServerLogger.Info("Received create job request", "remote_addr", r.RemoteAddr)

    var body struct {
        Command string          `json:"command"`
        Storage jobs.JobStorage `json:"storage"`
        MaxRetries int          `json:"max_retries"`
    }

    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        ServerLogger.Error("Failed to decode request body", "error", err, "remote_addr", r.RemoteAddr)
        http.Error(w, "invalid request body", http.StatusBadRequest)
        return
    }

    ServerLogger.Info("Parsed job request", "command", body.Command, "storage", body.Storage, "max_retries", body.MaxRetries)


    limits := [3]jobs.JobStorage{jobs.Volume10MB, jobs.Volume25MB,  jobs.Volume50MB}



    for i, v := range(limits) {
        if body.Storage < v {
            ServerLogger.Info("Adjusted storage size", "requested", body.Storage, "adjusted", v)
            body.Storage = v
            break
        }
        if i == len(limits)-1 {
            ServerLogger.Error("Job exceeds storage capacity", "command", body.Command, "requested_storage", body.Storage)
            http.Error(w, "storage requirement exceeds maximum capacity", http.StatusBadRequest)
            return
        }
    }

    if body.MaxRetries == 0{
        body.MaxRetries = 3
        ServerLogger.Info("Using default max_retries", "max_retries", body.MaxRetries)
    }

    job := &jobs.Job{
        Command:   body.Command,
        StorageBytes:   body.Storage,
        VolumePath: jobs.VolumePaths[body.Storage],
        Status:    jobs.StatusPending,
        CreatedAt: time.Now(),
        MaxRetries: body.MaxRetries,
        JobTrial: 1,
    }

    ServerLogger.Info("Creating job in database", "command", job.Command, "storage", job.StorageBytes, "volume_path", job.VolumePath)

    if err := h.JobStore.CreateJob(job); err != nil {
        ServerLogger.Error("Failed to create job in database", "error", err, "command", job.Command)
        http.Error(w, "failed to create job", http.StatusInternalServerError)
        return
    }

    ServerLogger.Info("Job created in database", "job_id", job.ID, "command", job.Command)

    job.Logger =  logger.NewJobLogger(h.ctx, job.ID, h.StreamSink)
    job.Logger.Info("Successfully created job!")

    if err := h.Client.Enqueue(h.ctx, *job); err != nil {
        ServerLogger.Error("Failed to enqueue job to Redis", "error", err, "job_id", job.ID)
        http.Error(w, "failed to enqueue job", http.StatusInternalServerError)
        return
    }

    ServerLogger.Info("Job enqueued successfully", "job_id", job.ID)

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(job); err != nil {
        ServerLogger.Error("Failed to encode response", "error", err, "job_id", job.ID)
    }
}

func (h *Handlers) CancelJob(w http.ResponseWriter, r *http.Request){
    id := mux.Vars(r)["id"]
    ServerLogger.Info("Received cancel job request", "job_id", id, "remote_addr", r.RemoteAddr)

    var body struct{
        Reason string `json:"reason"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        ServerLogger.Warn("Failed to decode cancel request body, proceeding without reason", "error", err, "job_id", id)
    }
    reason := body.Reason

    ServerLogger.Info("Attempting to cancel running job", "job_id", id)
    if err := h.Queue.Executor.CancelJob(id); err != nil {
        ServerLogger.Warn("Job not currently running or already completed", "error", err, "job_id", id)
    } else {
        ServerLogger.Info("Successfully cancelled running job execution", "job_id", id)
    }

    job, err := h.JobStore.CancelJob(id)
    if err != nil {
        ServerLogger.Error("Unable to cancel job in store", "error", err, "job_id", id)
        http.Error(w, "job not cancellable", http.StatusBadRequest)
        return
    }

    ServerLogger.Info("Job cancelled in database", "job_id", id, "reason", reason)

    if job.Logger == nil {
        job.Logger =  logger.NewJobLogger(h.ctx, job.ID, h.StreamSink)
    }
    job.Logger.Info("Successfully cancelled job!",
            logger.Item("reason", reason))

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(job); err != nil {
        ServerLogger.Error("Failed to encode cancel response", "error", err, "job_id", job.ID)
    }
}

func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    ServerLogger.Info("Received get job request", "job_id", id, "remote_addr", r.RemoteAddr)

    job, err := h.JobStore.GetJob(id)
    if err != nil {
        ServerLogger.Error("Failed to fetch job from database", "error", err, "job_id", id)
        http.Error(w, "job not found", http.StatusNotFound)
        return
    }

    ServerLogger.Info("Successfully fetched job", "job_id", id, "status", job.Status)

    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(job); err != nil {
        ServerLogger.Error("Failed to encode job response", "error", err, "job_id", job.ID)
    }
}


func (h *Handlers) StartRedisAcknowledger(ctx context.Context, results chan *jobs.Job)  {
	ServerLogger.Info("Starting Redis acknowledger goroutine")
	go func(){
		for {
			select {
			case <-ctx.Done():
				ServerLogger.Info("Redis acknowledger shutting down")
				return
			case res := <- results:
				ServerLogger.Info("Processing job result", "job_id", res.ID, "status", res.Status, "trial", res.JobTrial)
				switch res.Status{
				case jobs.StatusSuccess:
					ServerLogger.Info("Acknowledging successful job", "job_id", res.ID)
					if err := h.Client.Acknowledge(ctx, *res); err != nil {
						ServerLogger.Error("Failed to acknowledge job", "error", err, "job_id", res.ID)
					}
				case jobs.StatusFailed:
					if res.JobTrial >= res.MaxRetries{
						ServerLogger.Warn("Job exhausted all retries", "job_id", res.ID, "trials", res.JobTrial, "max_retries", res.MaxRetries)
						if err := h.JobStore.UpdateJob(res); err != nil {
							ServerLogger.Error("Failed to update failed job", "error", err, "job_id", res.ID)
						}
						continue
					}
					res.JobTrial++
					ServerLogger.Info("Retrying failed job", "job_id", res.ID, "trial", res.JobTrial, "max_retries", res.MaxRetries)
					if err := h.Client.Enqueue(ctx, *res); err != nil {
						ServerLogger.Error("Failed to re-enqueue job", "error", err, "job_id", res.ID)
					}
                default:
					ServerLogger.Info("Updating job with status", "job_id", res.ID, "status", res.Status)
                    if err := h.JobStore.UpdateJob(res); err != nil {
						ServerLogger.Error("Failed to update job", "error", err, "job_id", res.ID)
					}
			}
		}
	}
	}()
}