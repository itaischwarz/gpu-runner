package controlplane

import (
	"context"
	"errors"
	"gpu-runner/internal/jobs"
	"testing"
)

type fakeProvider struct {
	slots []Slot
	err   error
}

func (f *fakeProvider) Resource() jobs.JobResource               { return jobs.ResourceGPU }
func (f *fakeProvider) Poll(ctx context.Context) ([]Slot, error) { return f.slots, f.err }
func (f *fakeProvider) Env(slotID string) []string               { return nil }

func discovered(t *testing.T) *SlotTable {
	t.Helper()
	table, err := DiscoverSlots(context.Background(), &fakeProvider{slots: []Slot{
		{ID: "GPU-a", Model: "A100", TotalMemoryMB: 81920, FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", Model: "RTX 4090", TotalMemoryMB: 24564, FreeMemoryMB: 24000, Healthy: true},
	}})
	if err != nil {
		t.Fatalf("DiscoverSlots failed: %v", err)
	}
	return table
}

func TestDiscoverSlotsFailsOnErrorOrNoSlots(t *testing.T) {
	if _, err := DiscoverSlots(context.Background(), &fakeProvider{err: errors.New("boom")}); err == nil {
		t.Error("expected an error when the first poll fails")
	}
	if _, err := DiscoverSlots(context.Background(), &fakeProvider{}); err == nil {
		t.Error("expected an error when no slots are reported")
	}
}

func TestUpdateOnlyChangesFreeMemoryAndHealth(t *testing.T) {
	table := discovered(t)

	table.Update([]Slot{
		{ID: "GPU-a", Model: "changed", TotalMemoryMB: 1, FreeMemoryMB: 1000, Healthy: false},
		{ID: "GPU-b", Model: "RTX 4090", TotalMemoryMB: 24564, FreeMemoryMB: 500, Healthy: true},
	}, nil)

	got := table.Snapshot()
	want := []Slot{
		{ID: "GPU-a", Model: "A100", TotalMemoryMB: 81920, FreeMemoryMB: 1000, Healthy: false},
		{ID: "GPU-b", Model: "RTX 4090", TotalMemoryMB: 24564, FreeMemoryMB: 500, Healthy: true},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d: expected %+v, got %+v", i, want[i], got[i])
		}
	}
}

func TestUpdateMarksMissingSlotUnhealthyAndIgnoresNewOnes(t *testing.T) {
	table := discovered(t)

	table.Update([]Slot{
		{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-new", FreeMemoryMB: 80000, Healthy: true},
	}, nil)

	got := table.Snapshot()
	if len(got) != 2 {
		t.Fatalf("expected the slot set to stay at 2, got %d", len(got))
	}
	if !got[0].Healthy {
		t.Error("expected GPU-a to stay healthy")
	}
	if got[1].Healthy {
		t.Error("expected GPU-b to be unhealthy after going missing")
	}
}

func TestUpdatePollErrorMarksAllUnhealthyThenRecovers(t *testing.T) {
	table := discovered(t)

	table.Update(nil, errors.New("nvidia-smi timed out"))
	for _, s := range table.Snapshot() {
		if s.Healthy {
			t.Errorf("expected %s to be unhealthy after a failed poll", s.ID)
		}
	}

	table.Update([]Slot{
		{ID: "GPU-a", FreeMemoryMB: 80000, Healthy: true},
		{ID: "GPU-b", FreeMemoryMB: 24000, Healthy: true},
	}, nil)
	for _, s := range table.Snapshot() {
		if !s.Healthy {
			t.Errorf("expected %s to recover on the next good poll", s.ID)
		}
	}
}
