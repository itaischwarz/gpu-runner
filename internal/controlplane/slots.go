package controlplane

import (
	"context"
	"fmt"
	"gpu-runner/internal/logger"
	"sync"
)

var controlPlaneLogger = logger.Server

// SlotTable holds the slots found at discovery. The set of slots never changes
// after that, because a worker is bound to each one at startup. Each poll only
// updates the fields that can change: FreeMemoryMB and Healthy.
type SlotTable struct {
	mu    sync.RWMutex
	order []string         // slot IDs in discovery order
	slots map[string]*Slot // keyed by slot ID
}

// DiscoverSlots runs the first poll and fixes the set of slots.
func DiscoverSlots(ctx context.Context, p Provider) (*SlotTable, error) {
	polled, err := p.Poll(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover %s slots: %w", p.Resource(), err)
	}
	if len(polled) == 0 {
		return nil, fmt.Errorf("discover %s slots: provider reported none", p.Resource())
	}

	t := &SlotTable{slots: make(map[string]*Slot, len(polled))}
	for _, s := range polled {
		t.order = append(t.order, s.ID)
		t.slots[s.ID] = &s
		controlPlaneLogger.Info("Discovered slot", "resource", p.Resource(), "id", s.ID, "model", s.Model, "total_memory_mb", s.TotalMemoryMB, "healthy", s.Healthy)
	}
	return t, nil
}

// Update applies one poll result. If the poll failed, every slot is marked
// unhealthy. A slot missing from the poll is marked unhealthy, since
// nvidia-smi omits a GPU that has fallen off the bus rather than reporting it
// as failed. Slots that were not present at discovery are ignored, since no
// worker is bound to them.
func (t *SlotTable) Update(polled []Slot, pollErr error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if pollErr != nil {
		controlPlaneLogger.Error("Slot poll failed", "error", pollErr)
		for _, id := range t.order {
			t.setHealth(t.slots[id], false, "poll failed")
		}
		return
	}

	seen := make(map[string]bool, len(polled))
	for _, p := range polled {
		s, ok := t.slots[p.ID]
		if !ok {
			controlPlaneLogger.Warn("Ignoring slot not present at discovery", "id", p.ID)
			continue
		}
		seen[p.ID] = true
		s.FreeMemoryMB = p.FreeMemoryMB
		t.setHealth(s, p.Healthy, "reported unhealthy")
	}
	for _, id := range t.order {
		if !seen[id] {
			t.setHealth(t.slots[id], false, "missing from poll")
		}
	}
}

// Snapshot returns a copy of every slot in discovery order.
func (t *SlotTable) Snapshot() []Slot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	out := make([]Slot, len(t.order))
	for i, id := range t.order {
		out[i] = *t.slots[id]
	}
	return out
}

// setHealth updates a slot's health and logs only when it changes.
func (t *SlotTable) setHealth(s *Slot, healthy bool, reason string) {
	if s.Healthy == healthy {
		return
	}
	s.Healthy = healthy
	if healthy {
		controlPlaneLogger.Info("Slot recovered", "id", s.ID)
	} else {
		controlPlaneLogger.Warn("Slot became unhealthy", "id", s.ID, "reason", reason)
	}
}
