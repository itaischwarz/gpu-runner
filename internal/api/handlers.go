package api

import (
    "encoding/json"
    "github.com/gorilla/mux"
    "gpu-runner/internal/jobs"
    "net/http"
    "time"
    "github.com/google/uuid"
)

type Handlers struct {
    Queue     *jobs.JobQueue
    JobStore  map[string]*jobs.Job
}

func NewHandlers(queue *jobs.JobQueue) *Handlers {
    return &Handlers{
        Queue:    queue,
        JobStore: make(map[string]*jobs.Job),
    }
}

func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
    var body struct {
        Command string `json:"command"`
    }

    json.NewDecoder(r.Body).Decode(&body)

    id := uuid.New().String()
    job := &jobs.Job{
        ID:        id,
        Command:   body.Command,
        Status:    jobs.StatusPending,
        CreatedAt: time.Now(),
    }

    h.JobStore[id] = job
    h.Queue.Enqueue(job)

    json.NewEncoder(w).Encode(job)
}

func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]

    job, ok := h.JobStore[id]
    if !ok {
        http.NotFound(w, r)
        return
    }

    json.NewEncoder(w).Encode(job)
}
