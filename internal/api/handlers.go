package api

import (
	"context"
	"encoding/json"
	"fmt"
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
    var body struct {
        Command string          `json:"command"`
        Storage jobs.JobStorage `json:"storage"` 
        MaxRetries int          `json:"max_retries"`  
    }
    fmt.Println("HERE")

    json.NewDecoder(r.Body).Decode(&body)


    limits := [3]jobs.JobStorage{jobs.Volume10MB, jobs.Volume25MB,  jobs.Volume50MB}



    for i, v := range(limits) {
        if body.Storage < v {
            body.Storage = v
            break
        }
        if i == len(limits)-1 {
            ServerLogger.Error("This job exceeds our storage capacity", "command", body.Command)
        }
    }
    
    if body.MaxRetries == 0{
        body.MaxRetries = 3
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

    fmt.Printf("%+v\n", job)

    if err := h.JobStore.CreateJob(job); err != nil {
        ServerLogger.Error("Failed to create job", "error", err)
        return
    }
    job.Logger =  logger.NewJobLogger(h.ctx, job.ID, h.StreamSink)
    job.Logger.Info("Succesfully Created Job!")
    h.Client.Enqueue(h.ctx, *job)

    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(job)
}

func (h *Handlers) CancelJob(w http.ResponseWriter, r *http.Request){
    id := mux.Vars(r)["id"]
    var body struct{
        Reason string `json:"reason"`
    }
    _ = json.NewDecoder(r.Body).Decode(&body)
    reason := body.Reason

    if err := h.Queue.Executor.CancelJob(id); err != nil {
        ServerLogger.Error("Unable to cancel running job", "error", err, "job", id)
    }

    job, err := h.JobStore.CancelJob(id)
    if err != nil {
        ServerLogger.Error("Unable to cancel job in store", "error", err, "job", id)
        http.Error(w, "job not cancellable", http.StatusBadRequest)
        return
    }
    if job.Logger == nil {
        job.Logger =  logger.NewJobLogger(h.ctx, job.ID, h.StreamSink)
    }
    job.Logger.Info("Succesfully Cancelled Job!", 
            logger.Item("reason", reason))

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(job)
}

func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]

    fmt.Printf("Type: %T\n", id)

    // fmt.Println("id"  id)
    // fmt.Println("id:", mux.Vars(r)["id"])

    job, err := h.JobStore.GetJob(id)
    if err != nil {
        ServerLogger.Error("Failed to Fetch job",  "error" , err)
        return
    }
    

    json.NewEncoder(w).Encode(job)
}
