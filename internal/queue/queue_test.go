package queue

import (
	"context"
	"errors"
	"gpu-runner/internal/jobs"
	"testing"
	"time"
)

type placement struct {
	slotID string
	jobID  string
}

// fakePlacer records placements and rejects slots listed in reject.
type fakePlacer struct {
	placed []placement
	reject map[string]bool
	notify chan placement
}

func (f *fakePlacer) Place(slotID string, job *jobs.Job) error {
	if f.reject[slotID] {
		return errors.New("slot no longer available")
	}
	p := placement{slotID, job.ID}
	f.placed = append(f.placed, p)
	if f.notify != nil {
		f.notify <- p
	}
	return nil
}

func newTestQueue() (*Queue, *fakePlacer) {
	placer := &fakePlacer{reject: map[string]bool{}}
	return New(placer, FIFO{}, 10), placer
}

func TestMatchPlacesJobOnOpenOffer(t *testing.T) {
	q, placer := newTestQueue()
	q.open["GPU-a"] = Offer{SlotID: "GPU-a", FreeMemoryMB: 80000}
	q.pending = []*jobs.Job{{ID: "1"}}

	q.match()

	if len(placer.placed) != 1 || placer.placed[0] != (placement{"GPU-a", "1"}) {
		t.Fatalf("expected job 1 on GPU-a, got %+v", placer.placed)
	}
	if len(q.pending) != 0 || len(q.open) != 0 {
		t.Errorf("expected job and offer to be used, got pending=%d open=%d", len(q.pending), len(q.open))
	}
}

func TestMatchRespectsMemoryRequest(t *testing.T) {
	q, placer := newTestQueue()
	q.open["GPU-small"] = Offer{SlotID: "GPU-small", FreeMemoryMB: 24000}
	q.pending = []*jobs.Job{{ID: "big", MemoryMB: 30000}}

	q.match()

	if len(placer.placed) != 0 {
		t.Fatalf("expected no placement on a 24000 MB offer, got %+v", placer.placed)
	}

	q.open["GPU-large"] = Offer{SlotID: "GPU-large", FreeMemoryMB: 80000}
	q.match()

	if len(placer.placed) != 1 || placer.placed[0] != (placement{"GPU-large", "big"}) {
		t.Fatalf("expected big on GPU-large, got %+v", placer.placed)
	}
	if _, ok := q.open["GPU-small"]; !ok {
		t.Error("expected the unused GPU-small offer to stay open")
	}
}

func TestMatchFillsSmallSlotsFirst(t *testing.T) {
	q, placer := newTestQueue()
	q.open["GPU-large"] = Offer{SlotID: "GPU-large", FreeMemoryMB: 80000}
	q.open["GPU-small"] = Offer{SlotID: "GPU-small", FreeMemoryMB: 24000}
	q.pending = []*jobs.Job{{ID: "small", MemoryMB: 8000}, {ID: "big", MemoryMB: 60000}}

	q.match()

	want := []placement{{"GPU-small", "small"}, {"GPU-large", "big"}}
	if len(placer.placed) != 2 || placer.placed[0] != want[0] || placer.placed[1] != want[1] {
		t.Fatalf("expected %+v, got %+v", want, placer.placed)
	}
}

func TestMatchLetsSmallJobPassLargeWaitingJob(t *testing.T) {
	q, placer := newTestQueue()
	q.open["GPU-small"] = Offer{SlotID: "GPU-small", FreeMemoryMB: 24000}
	q.pending = []*jobs.Job{{ID: "big", MemoryMB: 60000}, {ID: "small", MemoryMB: 8000}}

	q.match()

	if len(placer.placed) != 1 || placer.placed[0] != (placement{"GPU-small", "small"}) {
		t.Fatalf("expected small on GPU-small, got %+v", placer.placed)
	}
	if len(q.pending) != 1 || q.pending[0].ID != "big" {
		t.Fatalf("expected big to keep waiting, got %+v", q.pending)
	}
}

func TestMatchDropsRejectedOfferAndKeepsJob(t *testing.T) {
	q, placer := newTestQueue()
	placer.reject["GPU-a"] = true
	q.open["GPU-a"] = Offer{SlotID: "GPU-a", FreeMemoryMB: 24000}
	q.open["GPU-b"] = Offer{SlotID: "GPU-b", FreeMemoryMB: 80000}
	q.pending = []*jobs.Job{{ID: "1"}}

	q.match()

	if len(placer.placed) != 1 || placer.placed[0] != (placement{"GPU-b", "1"}) {
		t.Fatalf("expected fallback to GPU-b, got %+v", placer.placed)
	}
	if len(q.open) != 0 {
		t.Errorf("expected the rejected GPU-a offer to be dropped, got %+v", q.open)
	}
}

func TestFIFOPicksOldestJobThatFits(t *testing.T) {
	pending := []*jobs.Job{{ID: "big", MemoryMB: 60000}, {ID: "a", MemoryMB: 8000}, {ID: "b", MemoryMB: 8000}}

	if i := (FIFO{}).Pick(pending, Offer{FreeMemoryMB: 24000}); i != 1 {
		t.Errorf("expected index 1 (oldest that fits), got %d", i)
	}
	if i := (FIFO{}).Pick(pending, Offer{FreeMemoryMB: 4000}); i != -1 {
		t.Errorf("expected -1 when nothing fits, got %d", i)
	}
}

func TestStartMatchesJobsAndOffersFromChannels(t *testing.T) {
	q, placer := newTestQueue()
	placer.notify = make(chan placement, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	// Job first, offer later: the job waits for capacity.
	q.Intake() <- &jobs.Job{ID: "1"}
	q.Offer(Offer{SlotID: "GPU-a", FreeMemoryMB: 80000})
	expectPlacement(t, placer.notify, placement{"GPU-a", "1"})

	// Offer first, job later: the job is placed as soon as it arrives.
	q.Offer(Offer{SlotID: "GPU-b", FreeMemoryMB: 80000})
	q.Intake() <- &jobs.Job{ID: "2"}
	expectPlacement(t, placer.notify, placement{"GPU-b", "2"})

	// A closed intake (adapter shutdown) must not stop offers from working.
	close(q.Intake())
	q.Offer(Offer{SlotID: "GPU-c", FreeMemoryMB: 80000})
	select {
	case p := <-placer.notify:
		t.Fatalf("unexpected placement with no pending jobs: %+v", p)
	case <-time.After(50 * time.Millisecond):
	}
}

func expectPlacement(t *testing.T, notify chan placement, want placement) {
	t.Helper()
	select {
	case got := <-notify:
		if got != want {
			t.Fatalf("expected %+v, got %+v", want, got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %+v", want)
	}
}
