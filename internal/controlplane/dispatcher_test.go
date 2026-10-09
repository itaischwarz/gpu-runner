package controlplane

import (
	"context"
	"gpu-runner/internal/jobs"
	"sort"
	"testing"
	"time"
)

// newTestDispatcher discovers slots from provider and starts the dispatcher's
// loop. Changing provider.slots and calling cp.PollFreeMemory simulates a poll.
func newTestDispatcher(t *testing.T, provider *fakeProvider) (*ControlPlane, *Dispatcher) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	table, err := DiscoverSlots(ctx, provider)
	if err != nil {
		t.Fatalf("DiscoverSlots failed: %v", err)
	}
	cp := &ControlPlane{provider: provider, slotTable: table, interval: time.Second}
	d := NewDispatcher(ctx, *cp)
	d.Dispatch(ctx)
	return cp, d
}

func offeredIDs(d *Dispatcher, memoryMB int) []string {
	var ids []string
	for _, o := range d.Available(memoryMB) {
		ids = append(ids, o.SlotID)
	}
	sort.Strings(ids)
	return ids
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestDispatcherHidesUnhealthyIdleGPUAndOffersItAfterRecovery(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", TotalMemoryMB: 24564, FreeMemoryMB: 24000, Healthy: true},
	}}
	cp, d := newTestDispatcher(t, provider)

	provider.slots = []Slot{{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true}, {ID: "GPU-b", Healthy: false}}
	cp.PollFreeMemory(context.Background())
	if got := offeredIDs(d, 1000); len(got) != 1 || got[0] != "GPU-a" {
		t.Fatalf("expected only GPU-a while GPU-b is down, got %v", got)
	}
	if err := d.Place("GPU-b", &jobs.Job{ID: "1", MemoryMB: 1000}); err == nil {
		t.Error("expected Place to reject an unhealthy GPU")
	}

	provider.slots = []Slot{{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true}, {ID: "GPU-b", FreeMemoryMB: 24000, Healthy: true}}
	cp.PollFreeMemory(context.Background())
	d.Polled()
	waitFor(t, "GPU-b to be offered after recovery", func() bool { return len(offeredIDs(d, 1000)) == 2 })
}

func TestDispatcherOffersGPUThatWasDownAtStartupOnceItRecovers(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", TotalMemoryMB: 24564, Healthy: false},
	}}
	cp, d := newTestDispatcher(t, provider)

	if got := offeredIDs(d, 1000); len(got) != 1 || got[0] != "GPU-a" {
		t.Fatalf("expected only GPU-a at startup, got %v", got)
	}

	provider.slots = []Slot{{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true}, {ID: "GPU-b", FreeMemoryMB: 24000, Healthy: true}}
	cp.PollFreeMemory(context.Background())
	d.Polled()
	waitFor(t, "GPU-b to be offered with its free memory", func() bool { return len(offeredIDs(d, 20000)) == 2 })
}

func TestDispatcherPolledRefreshesFreeMemory(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{{ID: "GPU-a", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true}}}
	cp, d := newTestDispatcher(t, provider)

	// Another process takes most of the GPU's memory.
	provider.slots = []Slot{{ID: "GPU-a", FreeMemoryMB: 4000, Healthy: true}}
	cp.PollFreeMemory(context.Background())
	d.Polled()
	waitFor(t, "an 8000 MB job to stop fitting", func() bool { return len(offeredIDs(d, 8000)) == 0 })
	if got := offeredIDs(d, 2000); len(got) != 1 {
		t.Errorf("expected GPU-a to still fit a 2000 MB job, got %v", got)
	}
}

func TestDispatcherPlaceThenReleaseReturnsGPU(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{{ID: "GPU-a", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true}}}
	_, d := newTestDispatcher(t, provider)

	job := &jobs.Job{ID: "1", MemoryMB: 8000}
	if err := d.Place("GPU-a", job); err != nil {
		t.Fatalf("Place failed: %v", err)
	}
	if got := <-d.Inbox()["GPU-a"]; got != job {
		t.Fatalf("expected the job in GPU-a's inbox, got %+v", got)
	}
	if got := offeredIDs(d, 1000); len(got) != 0 {
		t.Fatalf("expected busy GPU-a not to be offered, got %v", got)
	}

	// A poll while the job runs must not make the busy GPU available.
	d.Polled()
	time.Sleep(20 * time.Millisecond)
	if got := offeredIDs(d, 1000); len(got) != 0 {
		t.Fatalf("busy GPU-a was offered after a poll: %v", got)
	}

	d.Release("GPU-a", job)
	waitFor(t, "GPU-a to be offered after Release", func() bool { return len(offeredIDs(d, 1000)) == 1 })
}
