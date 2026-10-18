package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newCP runs discovery on providers. Changing a provider's slots or err and
// then polling simulates that machine changing.
func newCP(t *testing.T, providers ...Provider) *ControlPlane {
	t.Helper()
	cp, err := NewControlPlane(context.Background(), time.Second, providers...)
	if err != nil {
		t.Fatalf("NewControlPlane failed: %v", err)
	}
	return cp
}

func TestOneUnreachableProviderOnlyTakesDownItsOwnSlots(t *testing.T) {
	boxA := &fakeProvider{slots: []Slot{{ID: "GPU-a", TotalMemoryMB: 49140, FreeMemoryMB: 48000, Healthy: true}}}
	boxB := &fakeProvider{slots: []Slot{{ID: "GPU-b", TotalMemoryMB: 32607, FreeMemoryMB: 32000, Healthy: true}}}
	cp := newCP(t, boxA, boxB)
	ctx := context.Background()

	if free := cp.PollFreeMemory(ctx); len(free) != 2 {
		t.Fatalf("expected both GPUs healthy, got %v", free)
	}

	boxA.err = errors.New("connection refused")
	free := cp.PollFreeMemory(ctx)
	if len(free) != 1 || free["GPU-b"] != 32000 {
		t.Fatalf("expected only GPU-b healthy while box A is down, got %v", free)
	}
	if err := cp.Health(ctx); err != nil {
		t.Errorf("expected healthy while one box is up, got %v", err)
	}
	if got := cp.ProviderFor("GPU-a"); got != boxA {
		t.Errorf("expected GPU-a to belong to box A, got %v", got)
	}

	boxA.err = nil
	if free := cp.PollFreeMemory(ctx); len(free) != 2 {
		t.Fatalf("expected box A's GPU back after it recovers, got %v", free)
	}
}

func TestNewControlPlaneSkipsProvidersDownAtStartup(t *testing.T) {
	up := &fakeProvider{slots: []Slot{{ID: "GPU-a", Healthy: true}}}
	down := &fakeProvider{err: errors.New("connection refused")}

	cp := newCP(t, down, up)
	if got := len(cp.Snapshot()); got != 1 {
		t.Errorf("expected only the reachable provider's slot, got %d slots", got)
	}

	if _, err := NewControlPlane(context.Background(), time.Second, down); err == nil {
		t.Error("expected an error when no provider can be discovered")
	}
}

func TestNewControlPlaneRejectsDuplicateSlotIDs(t *testing.T) {
	a := &fakeProvider{slots: []Slot{{ID: "GPU-a", Healthy: true}}}
	b := &fakeProvider{slots: []Slot{{ID: "GPU-a", Healthy: true}}}
	if _, err := NewControlPlane(context.Background(), time.Second, a, b); err == nil {
		t.Error("expected an error when two providers report the same GPU")
	}
}

func TestPollFreeMemoryReturnsHealthySlotsOnly(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", TotalMemoryMB: 24564, FreeMemoryMB: 24000, Healthy: true},
	}}
	ctx := context.Background()
	cp := newCP(t, provider)

	// GPU-a's free memory dropped; GPU-b failed.
	provider.slots = []Slot{
		{ID: "GPU-a", FreeMemoryMB: 50000, Healthy: true},
		{ID: "GPU-b", FreeMemoryMB: 24000, Healthy: false},
	}
	free := cp.PollFreeMemory(ctx)

	if len(free) != 1 || free["GPU-a"] != 50000 {
		t.Fatalf("expected only GPU-a with 50000 MB free, got %v", free)
	}
	if got := cp.Snapshot()[0].FreeMemoryMB; got != 50000 {
		t.Errorf("expected the slot table to be updated to 50000, got %d", got)
	}
}

func TestMaxMemoryMBIsLargestGPUTotal(t *testing.T) {
	provider := &fakeProvider{slots: []Slot{
		{ID: "GPU-a", TotalMemoryMB: 24564, FreeMemoryMB: 1000, Healthy: true},
		{ID: "GPU-b", TotalMemoryMB: 81920, FreeMemoryMB: 1000, Healthy: false},
	}}
	cp := newCP(t, provider)

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
	cp := newCP(t, provider)

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
	cp := newCP(t, provider)

	provider.err = errors.New("nvidia-smi timed out")
	if free := cp.PollFreeMemory(ctx); len(free) != 0 {
		t.Fatalf("expected no healthy slots after a failed poll, got %v", free)
	}
}
