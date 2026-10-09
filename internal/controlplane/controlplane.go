package controlplane

import (
	"context"
	"fmt"
	"time"
)

type ControlPlane struct {
	provider  Provider
	slotTable *SlotTable
	interval  time.Duration
	onPoll    func() // called after each background poll; see OnPoll
}

// NewControlPlane builds the GPU provider and runs discovery, so a
// ControlPlane always has its slots.
func NewControlPlane(ctx context.Context, smiPath string, interval time.Duration) (*ControlPlane, error) {
	provider := NewGPUProvider(smiPath)
	slotTable, err := DiscoverSlots(ctx, provider)
	if err != nil {
		return nil, err
	}
	return &ControlPlane{
		provider:  provider,
		slotTable: slotTable,
		interval:  interval,
	}, nil
}

// UpdateGPUHealth polls the provider every interval until ctx is cancelled.
// SlotTable.Update applies each reading to every slot: healthy slots only get
// fresh free memory, and health changes are logged.
func (cp *ControlPlane) UpdateGPUHealth(ctx context.Context) {
	go func() {
		for {
			cp.PollFreeMemory(ctx)
			if cp.onPoll != nil {
				cp.onPoll()
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(cp.interval):
			}
		}
	}()
}

// PollFreeMemory polls every GPU once, updates the slot table, and returns
// slot ID → free memory (MB) for the slots that are healthy. Unhealthy slots
// are left out, so callers never offer a GPU that can't take a job.
func (cp *ControlPlane) PollFreeMemory(ctx context.Context) map[string]int {
	_ = cp.poll(ctx) // a failed poll marks every slot unhealthy, so none are returned
	return cp.FreeMemory()
}

// FreeMemory returns slot ID → free memory (MB) for the healthy slots, as of
// the last poll. It doesn't run nvidia-smi.
func (cp *ControlPlane) FreeMemory() map[string]int {
	free := make(map[string]int)
	for _, s := range cp.slotTable.Snapshot() {
		if s.Healthy {
			free[s.ID] = s.FreeMemoryMB
		}
	}
	return free
}

// OnPoll registers fn to run after every background poll, e.g. to tell the
// dispatcher GPU state changed. Call it before UpdateGPUHealth.
func (cp *ControlPlane) OnPoll(fn func()) {
	cp.onPoll = fn
}

// MaxMemoryMB returns the total memory of the largest GPU found at discovery:
// the biggest job this machine could ever run.
func (cp *ControlPlane) MaxMemoryMB() int {
	largest := 0
	for _, s := range cp.slotTable.Snapshot() {
		if s.TotalMemoryMB > largest {
			largest = s.TotalMemoryMB
		}
	}
	return largest
}

// Health runs nvidia-smi now and returns an error unless it can reach at
// least one healthy GPU.
func (cp *ControlPlane) Health(ctx context.Context) error {
	if err := cp.poll(ctx); err != nil {
		return fmt.Errorf("nvidia-smi unavailable: %w", err)
	}
	for _, s := range cp.slotTable.Snapshot() {
		if s.Healthy {
			return nil
		}
	}
	return fmt.Errorf("nvidia-smi reports no healthy GPU")
}

// poll runs one provider poll and applies it to the slot table.
func (cp *ControlPlane) poll(ctx context.Context) error {
	start := time.Now()
	slots, err := cp.provider.Poll(ctx)
	pollSeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		pollFailures.Inc()
	}
	cp.slotTable.Update(slots, err)
	return err
}
