package box

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"gpu-runner/internal/controlplane"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/logger"
)

var boxLogger = logger.Server

// NewHandler serves a box's GPUs and runs jobs on them. gpus reports the GPUs
// (controlplane.NewGPUProvider in production); exec runs the jobs, with
// memory enforcement if SetMemoryLimits was called on it; jobs run in
// workDir.
//
// The box keeps no state and does no scheduling: it runs whatever it is sent,
// as many at once as it is sent. The central dispatcher decides that.
func NewHandler(gpus controlplane.Provider, exec *executer.Executor, workDir string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /gpus", func(w http.ResponseWriter, r *http.Request) {
		slots, err := gpus.Poll(r.Context())
		if err != nil {
			boxLogger.Error("GPU poll failed", "error", err)
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, slots)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		slots, err := gpus.Poll(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		for _, s := range slots {
			if s.Healthy {
				writeJSON(w, map[string]string{"status": "ok"})
				return
			}
		}
		http.Error(w, "no healthy GPU", http.StatusServiceUnavailable)
	})

	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		var req RunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid run request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.JobID == "" || req.Command == "" {
			http.Error(w, "job_id and command are required", http.StatusBadRequest)
			return
		}

		boxLogger.Info("Running job", "job_id", req.JobID, "env", req.Env, "memory_mb", req.MemoryMB)
		// The job's log stream lives in the central server's Redis; the
		// central client logs the outcome there. The box only logs locally.
		jobLogger := logger.NewJobLogger(r.Context(), req.JobID, discardSink{})

		// r.Context() ends if the central server cancels or disconnects, and
		// the executor then kills the job's whole process group.
		output, err := exec.RunJobWithEnv(req.Command, req.JobID, workDir, r.Context(), *jobLogger, req.Env, req.MemoryMB)

		var resp RunResponse
		resp.Output = output
		var memErr *executer.MemoryExceededError
		switch {
		case errors.As(err, &memErr):
			resp.MemoryExceeded = &MemoryExceeded{UsedMB: memErr.UsedMB, LimitMB: memErr.LimitMB}
			resp.Error = memErr.Error()
		case err != nil:
			resp.Error = err.Error()
		}
		boxLogger.Info("Job finished", "job_id", req.JobID, "error", resp.Error)
		writeJSON(w, resp) // fails harmlessly if the central server already left
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		boxLogger.Warn("Failed to write response", "error", err)
	}
}

// discardSink drops job log lines on the box; see the /run handler.
type discardSink struct{}

func (discardSink) Append(context.Context, string, string) error { return nil }
