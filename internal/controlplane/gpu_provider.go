package controlplane

import (
	"context"
	"gpu-runner/internal/jobs"
)

// GPUProvider exposes the GPUs reported by nvidia-smi as slots, one per GPU,
// keyed by UUID. It keeps no state: each Poll is a fresh reading, and the
// control plane decides what to do with it.
type GPUProvider struct {
	smiPath string
}

var _ Provider = (*GPUProvider)(nil)

func NewGPUProvider(smiPath string) *GPUProvider {
	return &GPUProvider{smiPath: smiPath}
}

func (p *GPUProvider) Resource() jobs.JobResource {
	return jobs.ResourceGPU
}

func (p *GPUProvider) Poll(ctx context.Context) ([]Slot, error) {
	devices, err := QueryGPUs(ctx, p.smiPath)
	if err != nil {
		return nil, err
	}

	slots := make([]Slot, len(devices))
	for i, d := range devices {
		slots[i] = Slot{
			ID:            d.UUID,
			Model:         d.Model,
			TotalMemoryMB: d.TotalMemoryMB,
			FreeMemoryMB:  d.FreeMemoryMB,
			Healthy:       d.Healthy,
		}
	}
	return slots, nil
}

// Env pins a job to one GPU. The UUID is used rather than the index because
// indices can be reordered between driver reloads.
func (p *GPUProvider) Env(slotID string) []string {
	return []string{"CUDA_VISIBLE_DEVICES=" + slotID}
}
