// Package box is how the central server talks to a GPU box: a machine whose
// only job is to report its GPUs and run the jobs it is sent.
//
// The client (Client) lives on the central server. It is a
// controlplane.Provider, so the control plane polls a box like a local GPU,
// and it runs jobs with the same signature as the local executor, so a worker
// can use either. The server (NewHandler) runs on the box, behind cmd/worker.
//
// A box serves:
//
//	GET  /gpus    the box's GPUs, as []controlplane.Slot
//	POST /run     run one job; the request stays open until the job ends
//	GET  /health  200 if at least one GPU is healthy
package box

// RunRequest asks a box to run one job.
type RunRequest struct {
	JobID    string   `json:"job_id"`
	Command  string   `json:"command"`
	Env      []string `json:"env"`       // e.g. CUDA_VISIBLE_DEVICES=<uuid>, from the provider
	MemoryMB int      `json:"memory_mb"` // the job's request; the box kills it if exceeded
}

// RunResponse is how the job ended. Error is empty on success.
type RunResponse struct {
	Output         string          `json:"output"`
	Error          string          `json:"error,omitempty"`
	MemoryExceeded *MemoryExceeded `json:"memory_exceeded,omitempty"`
}

// MemoryExceeded means the box killed the job for using more GPU memory than
// it requested.
type MemoryExceeded struct {
	UsedMB  int `json:"used_mb"`
	LimitMB int `json:"limit_mb"`
}
