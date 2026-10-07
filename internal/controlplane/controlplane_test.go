package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollFreeMemoryReturnsHealthySlotsOnly(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", TotalMemoryMB: 24564, FreeMemoryMB: 24000, Healthy: true},
	}}
	ctx := context.Background()
	table, err := DiscoverSlots(ctx, provider)
	if err != nil {
		t.Fatalf("DiscoverSlots failed: %v", err)
	}
	cp := &ControlPlane{provider: provider, slotTable: table, interval: time.Second}

	// GPU-a's free memory dropped; GPU-b failed.
	provider.slots = []Slot{
		{ID: "GPU-a", FreeMemoryMB: 50000, Healthy: true},
		{ID: "GPU-b", FreeMemoryMB: 24000, Healthy: false},
	}
	free := cp.PollFreeMemory(ctx)

	if len(free) != 1 || free["GPU-a"] != 50000 {
		t.Fatalf("expected only GPU-a with 50000 MB free, got %v", free)
	}
	if got := table.Snapshot()[0].FreeMemoryMB; got != 50000 {
		t.Errorf("expected the slot table to be updated to 50000, got %d", got)
	}
}

func TestMaxMemoryMBIsLargestGPUTotal(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", TotalMemoryMB: 24564, FreeMemoryMB: 1000, Healthy: true},
		{ID: "GPU-b", TotalMemoryMB: 81920, FreeMemoryMB: 1000, Healthy: false},
	}}
	table, err := DiscoverSlots(context.Background(), provider)
	if err != nil {
		t.Fatalf("DiscoverSlots failed: %v", err)
	}
	cp := &ControlPlane{provider: provider, slotTable: table, interval: time.Second}

	// Total memory, not free memory, and regardless of health right now.
	if got := cp.MaxMemoryMB(); got != 81920 {
		t.Errorf("expected 81920, got %d", got)
	}
}

func TestHealth(t *testing.T) {
	ctx := context.Background()
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", FreeMemoryMB: 24000, Healthy: true},
	}}
	table, err := DiscoverSlots(ctx, provider)
	if err != nil {
		t.Fatalf("DiscoverSlots failed: %v", err)
	}
	cp := &ControlPlane{provider: provider, slotTable: table, interval: time.Second}

	if err := cp.Health(ctx); err != nil {
		t.Errorf("expected healthy with two good GPUs, got %v", err)
	}

	// One GPU down is still healthy.
	provider.slots = []Slot{{ID: "GPU-a", Healthy: true}, {ID: "GPU-b", Healthy: false}}
	if err := cp.Health(ctx); err != nil {
		t.Errorf("expected healthy with one good GPU, got %v", err)
	}

	// Every GPU down is not.
	provider.slots = []Slot{{ID: "GPU-a", Healthy: false}, {ID: "GPU-b", Healthy: false}}
	if err := cp.Health(ctx); err == nil {
		t.Error("expected an error when no GPU is healthy")
	}

	// nvidia-smi itself failing is not.
	provider.err = errors.New("nvidia-smi timed out")
	if err := cp.Health(ctx); err == nil {
		t.Error("expected an error when nvidia-smi fails")
	}
}

func TestPollFreeMemoryReturnsNothingWhenPollFails(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true}}}
	ctx := context.Background()
	table, err := DiscoverSlots(ctx, provider)
	if err != nil {
		t.Fatalf("DiscoverSlots failed: %v", err)
	}
	cp := &ControlPlane{provider: provider, slotTable: table, interval: time.Second}

	provider.err = errors.New("nvidia-smi timed out")
	if free := cp.PollFreeMemory(ctx); len(free) != 0 {
		t.Fatalf("expected no healthy slots after a failed poll, got %v", free)
	}
}
