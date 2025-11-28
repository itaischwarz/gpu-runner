package api

import (
	"encoding/json"
	"fmt"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"gpu-runner/internal/store"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"log/slog"

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
        Status:    jobs.StatusPending,
        CreatedAt: time.Now(),
    }

    if err := h.JobStore.CreateJob(job); err != nil {
        ServerLogger.Error("Failed to create job", http.StatusInternalServerError)
    }
    
    h.Queue.Enqueue(job)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(job)
}

func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]
    job, err := h.JobStore.GetJob(id)
    if err != nil {
        ServerLogger.Error("Failed to Fetch job", http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(job)
}
