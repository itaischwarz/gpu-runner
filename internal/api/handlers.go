package api

import (
	"encoding/json"
	"fmt"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"gpu-runner/internal/store"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

var ServerLogger = logger.Server

type Handlers struct {
    Queue     *jobs.JobQueue
    JobStore  *store.JobStore
}

func NewHandlers(queue *jobs.JobQueue, store *store.JobStore ) *Handlers {
    return &Handlers{
        Queue:    queue,
        JobStore:  store,
    }
}



func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
    var body struct {
        Command string          `json:"command"`
        Storage jobs.JobStorage `json:"storage"`   
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
    
    job := &jobs.Job{
        Command:   body.Command,
        StorageBytes:   body.Storage,
        VolumePath: jobs.VolumePaths[body.Storage],
        Status:    jobs.StatusPending,
        CreatedAt: time.Now(),
    }

    fmt.Printf("%+v\n", job)

    if err := h.JobStore.CreateJob(job); err != nil {
        ServerLogger.Error("Failed to create job", "error", err)
        return
    }
    job.Logger =  logger.CreateJobLogger(job.ID)

    job.Logger.Info("Succesfully Created Job!")

    
    h.Queue.Enqueue(job)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(job)
}

func (h *Handlers) CancelJob(w http.ResponseWriter, r *http.Request){
    id := mux.Vars(r)["id"]
    var body struct{
        reason string
    }
    json.NewDecoder(r.Body).Decode(&body)
    reason := body.reason
    job, err := h.JobStore.CancelJob(id)
    if err != nil {
        job.Logger.Error("Unable to Cancel Job", "error", err)
    }
    job.Logger.Info("Succesfully Cancelled Job!", "reason", reason)

    h.Queue.Enqueue(job)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(job)
}

func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    job, err := h.JobStore.GetJob(id)
    if err != nil {
        ServerLogger.Error("Failed to Fetch job",  "error" , err)
        return
    }
    

    json.NewEncoder(w).Encode(job)
}

