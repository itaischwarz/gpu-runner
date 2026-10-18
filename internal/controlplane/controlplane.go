package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type ControlPlane struct {
	pools    []*pool // one per provider, e.g. one per GPU box
	interval time.Duration
	onPoll   func() // called after each background poll; see OnPoll
}

// pool is one provider and the slots it reported at discovery. A failed poll
// marks only this pool's slots unhealthy, so one unreachable box doesn't take
// the others down with it.
type pool struct {
	provider  Provider
	slotTable *SlotTable
}

// NewControlPlane runs discovery on every provider, so a ControlPlane always
// has its slots. A provider that can't be reached at startup is skipped and
// logged; it is an error only if none can be. Slot IDs are GPU UUIDs, which
// are unique across machines, so the pools' slots never clash.
func NewControlPlane(ctx context.Context, interval time.Duration, providers ...Provider) (*ControlPlane, error) {
	cp := &ControlPlane{interval: interval}
	seen := make(map[string]bool)
	var errs []error
	for _, p := range providers {
		table, err := DiscoverSlots(ctx, p)
		if err != nil {
			controlPlaneLogger.Error("Skipping provider that failed discovery", "error", err)
			errs = append(errs, err)
			continue
		}
		for _, s := range table.Snapshot() {
			if seen[s.ID] {
				return nil, fmt.Errorf("slot %s reported by two providers", s.ID)
			}
			seen[s.ID] = true
		}
		cp.pools = append(cp.pools, &pool{provider: p, slotTable: table})
	}
	if len(cp.pools) == 0 {
		return nil, fmt.Errorf("no provider could be discovered: %w", errors.Join(errs...))
	}
	return cp, nil
}

// UpdateGPUHealth polls the providers every interval until ctx is cancelled.
// SlotTable.Update applies each reading to its pool's slots: healthy slots
// only get fresh free memory, and health changes are logged.
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

// PollFreeMemory polls every GPU once, updates the slot tables, and returns
// slot ID → free memory (MB) for the slots that are healthy. Unhealthy slots
// are left out, so callers never offer a GPU that can't take a job.
func (cp *ControlPlane) PollFreeMemory(ctx context.Context) map[string]int {
	_ = cp.poll(ctx) // a failed poll marks that pool's slots unhealthy, so they aren't returned
	return cp.FreeMemory()
}

// FreeMemory returns slot ID → free memory (MB) for the healthy slots, as of
// the last poll. It doesn't poll.
func (cp *ControlPlane) FreeMemory() map[string]int {
	free := make(map[string]int)
	for _, s := range cp.Snapshot() {
		if s.Healthy {
			free[s.ID] = s.FreeMemoryMB
		}
	}
	return free
}

// Snapshot returns a copy of every slot, pool by pool, in discovery order.
func (cp *ControlPlane) Snapshot() []Slot {
	var out []Slot
	for _, p := range cp.pools {
		out = append(out, p.slotTable.Snapshot()...)
	}
	return out
}

// ProviderFor returns the provider that owns slotID, or nil if no pool has
// it. cmd/server uses it to decide where a slot's jobs run.
func (cp *ControlPlane) ProviderFor(slotID string) Provider {
	for _, p := range cp.pools {
		for _, s := range p.slotTable.Snapshot() {
			if s.ID == slotID {
				return p.provider
			}
		}
	}
	return nil
}

// OnPoll registers fn to run after every background poll, e.g. to tell the
// dispatcher GPU state changed. Call it before UpdateGPUHealth.
func (cp *ControlPlane) OnPoll(fn func()) {
	cp.onPoll = fn
}

// MaxMemoryMB returns the total memory of the largest GPU found at discovery:
// the biggest job any machine could ever run.
func (cp *ControlPlane) MaxMemoryMB() int {
	largest := 0
	for _, s := range cp.Snapshot() {
		if s.TotalMemoryMB > largest {
			largest = s.TotalMemoryMB
		}
	}
	return largest
}

// Health polls now and returns an error unless at least one GPU, on any
// machine, is healthy.
func (cp *ControlPlane) Health(ctx context.Context) error {
	err := cp.poll(ctx)
	if len(cp.FreeMemory()) > 0 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("no healthy GPU: %w", err)
	}
	return fmt.Errorf("no healthy GPU")
}

// poll polls every pool at once, so one slow or unreachable box doesn't delay
// the readings from the others, and applies each result to its own pool. It
// returns the pools' errors joined.
func (cp *ControlPlane) poll(ctx context.Context) error {
	errs := make([]error, len(cp.pools))
	var wg sync.WaitGroup
	for i, p := range cp.pools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			slots, err := p.provider.Poll(ctx)
			pollSeconds.Observe(time.Since(start).Seconds())
			if err != nil {
				pollFailures.Inc()
			}
			p.slotTable.Update(slots, err)
			errs[i] = err
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}
