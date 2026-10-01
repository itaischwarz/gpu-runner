package gpu

import (
	"context"
	"gpu-runner/internal/controlplane"
	"gpu-runner/internal/jobs"
)

// Provider exposes the GPUs reported by nvidia-smi as control-plane slots,
// one per GPU, keyed by UUID. It keeps no state: each Poll is a fresh reading,
// and the control plane decides what to do with it.
type Provider struct {
	smiPath string
}

var _ controlplane.Provider = (*Provider)(nil)

func NewProvider(smiPath string) *Provider {
	return &Provider{smiPath: smiPath}
}

func (p *Provider) Resource() jobs.JobResource {
	return jobs.ResourceGPU
}

func (p *Provider) Poll(ctx context.Context) ([]controlplane.Slot, error) {
	devices, err := Query(ctx, p.smiPath)
	if err != nil {
		return nil, err
	}

	slots := make([]controlplane.Slot, len(devices))
	for i, d := range devices {
		slots[i] = controlplane.Slot{
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
func (p *Provider) Env(slotID string) []string {
	return []string{"CUDA_VISIBLE_DEVICES=" + slotID}
}
