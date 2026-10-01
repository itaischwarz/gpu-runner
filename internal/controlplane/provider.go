package controlplane

import (
	"context"
	"gpu-runner/internal/jobs"
)

// Slot is one GPU the control plane can place a job on.
//
// ID, Model and TotalMemoryMB identify the GPU and are fixed at discovery.
// FreeMemoryMB and Healthy are refreshed on every poll.
type Slot struct {
	ID            string `json:"id"`
	Model         string `json:"model"`
	TotalMemoryMB int    `json:"total_memory_mb"`
	FreeMemoryMB  int    `json:"free_memory_mb"`
	Healthy       bool   `json:"healthy"`
}

// Provider reports the GPUs a pool can schedule onto. It is the only
// infrastructure-specific part of a pool: the scheduler, workers and handlers
// are the same whichever provider is plugged in.
//
// The production provider is gpu.Provider (nvidia-smi). A CPU provider that
// fakes slots exists only for local load testing on machines without GPUs.
type Provider interface {
	// Resource is the job resource this provider's pool serves.
	Resource() jobs.JobResource

	// Poll returns a fresh reading of every slot currently visible. It holds
	// no state between calls. An error means no reading can be trusted.
	Poll(ctx context.Context) ([]Slot, error)

	// Env returns extra environment variables that restrict a job to slotID.
	Env(slotID string) []string
}
