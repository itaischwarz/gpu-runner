package queue

import (
	"context"
	"errors"
	"gpu-runner/internal/jobs"
	"sync"
	"testing"
	"time"
)

type placement struct {
	slotID string
	jobID  string
}

// fakeDispatcher behaves like the real one: Place takes a slot out of
// available, and Free puts it back. Slots in reject fail Place.
type fakeDispatcher struct {
	mu        sync.Mutex
	available map[string]int
	reject    map[string]bool
	placed    []placement
	notify    chan placement
}

func newFakeDispatcher(available map[string]int) *fakeDispatcher {
	return &fakeDispatcher{available: available, reject: map[string]bool{}}
}

func (f *fakeDispatcher) Available(memoryMB int) []Offer {
	f.mu.Lock()
	defer f.mu.Unlock()
	offers := make([]Offer, 0, len(f.available))
	for id, mem := range f.available {
		if mem >= memoryMB {
			offers = append(offers, Offer{SlotID: id, FreeMemoryMB: mem})
		}
	}
	return offers
}

func (f *fakeDispatcher) Place(slotID string, job *jobs.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.reject[slotID] {
		return errors.New("slot no longer available")
	}
	delete(f.available, slotID)
	p := placement{slotID, job.ID}
	f.placed = append(f.placed, p)
	if f.notify != nil {
		f.notify <- p
	}
	return nil
}

func (f *fakeDispatcher) Free(slotID string, mem int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.available[slotID] = mem
}

func TestMatchPlacesJobOnFreeSlot(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-a": 80000})
	q := New(d, FIFO{}, 10, 0)
	q.pending = []*jobs.Job{{ID: "1", MemoryMB: 8000}}

	q.match()

	if len(d.placed) != 1 || d.placed[0] != (placement{"GPU-a", "1"}) {
		t.Fatalf("expected job 1 on GPU-a, got %+v", d.placed)
	}
	if len(q.pending) != 0 {
		t.Errorf("expected no pending jobs, got %d", len(q.pending))
	}
}

func TestMatchRespectsMemoryRequest(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-small": 24000})
	q := New(d, FIFO{}, 10, 0)
	q.pending = []*jobs.Job{{ID: "big", MemoryMB: 30000}}

	q.match()
	if len(d.placed) != 0 {
		t.Fatalf("expected no placement on a 24000 MB slot, got %+v", d.placed)
	}

	d.Free("GPU-large", 80000)
	q.match()
	if len(d.placed) != 1 || d.placed[0] != (placement{"GPU-large", "big"}) {
		t.Fatalf("expected big on GPU-large, got %+v", d.placed)
	}
}

func TestMatchFillsSmallSlotsFirst(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-large": 80000, "GPU-small": 24000})
	q := New(d, FIFO{}, 10, 0)
	q.pending = []*jobs.Job{{ID: "small", MemoryMB: 8000}, {ID: "big", MemoryMB: 60000}}

	q.match()

	want := []placement{{"GPU-small", "small"}, {"GPU-large", "big"}}
	if len(d.placed) != 2 || d.placed[0] != want[0] || d.placed[1] != want[1] {
		t.Fatalf("expected %+v, got %+v", want, d.placed)
	}
}

func TestMatchLetsSmallJobPassLargeWaitingJob(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-small": 24000})
	q := New(d, FIFO{}, 10, 0)
	q.pending = []*jobs.Job{{ID: "big", MemoryMB: 60000}, {ID: "small", MemoryMB: 8000}}

	q.match()

	if len(d.placed) != 1 || d.placed[0] != (placement{"GPU-small", "small"}) {
		t.Fatalf("expected small on GPU-small, got %+v", d.placed)
	}
	if len(q.pending) != 1 || q.pending[0].ID != "big" {
		t.Fatalf("expected big to keep waiting, got %+v", q.pending)
	}
}

func TestMatchKeepsJobWhenPlaceRejected(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-a": 24000, "GPU-b": 80000})
	d.reject["GPU-a"] = true
	q := New(d, FIFO{}, 10, 0)
	q.pending = []*jobs.Job{{ID: "1", MemoryMB: 8000}}

	q.match()

	if len(d.placed) != 1 || d.placed[0] != (placement{"GPU-b", "1"}) {
		t.Fatalf("expected fallback to GPU-b, got %+v", d.placed)
	}
}

func TestFIFOOrdersOldestFirstWithoutModifyingPending(t *testing.T) {
	pending := []*jobs.Job{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	ordered := (FIFO{}).Order(pending)
	if len(ordered) != 3 || ordered[0].ID != "a" || ordered[1].ID != "b" || ordered[2].ID != "c" {
		t.Fatalf("expected a, b, c, got %+v", ordered)
	}
	ordered[0] = nil
	if pending[0] == nil {
		t.Error("Order must return a copy, not pending itself")
	}
}

func TestMatchTriesJobsInArrivalOrder(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-a": 80000})
	q := New(d, FIFO{}, 10, 0)
	q.pending = []*jobs.Job{{ID: "first", MemoryMB: 8000}, {ID: "second", MemoryMB: 8000}}

	q.match()

	if len(d.placed) != 1 || d.placed[0] != (placement{"GPU-a", "first"}) {
		t.Fatalf("expected the oldest job on the only GPU, got %+v", d.placed)
	}
	if len(q.pending) != 1 || q.pending[0].ID != "second" {
		t.Fatalf("expected second to keep waiting, got %+v", q.pending)
	}
}

func TestStartPlacesOnArrivalAndOnWake(t *testing.T) {
	d := newFakeDispatcher(map[string]int{"GPU-a": 80000})
	d.notify = make(chan placement, 2)
	q := New(d, FIFO{}, 10, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	// A free slot is used as soon as a job arrives.
	q.Intake() <- &jobs.Job{ID: "1", MemoryMB: 8000}
	expectPlacement(t, d.notify, placement{"GPU-a", "1"})

	// No slot free: the job waits until a release wakes the queue.
	q.Intake() <- &jobs.Job{ID: "2", MemoryMB: 8000}
	expectNoPlacement(t, d.notify)
	d.Free("GPU-a", 80000)
	q.Wake()
	expectPlacement(t, d.notify, placement{"GPU-a", "2"})
}

func TestStartRechecksOnTimerWithoutWake(t *testing.T) {
	d := newFakeDispatcher(map[string]int{})
	d.notify = make(chan placement, 1)
	q := New(d, FIFO{}, 10, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	q.Intake() <- &jobs.Job{ID: "1", MemoryMB: 8000}
	d.Free("GPU-a", 80000) // e.g. a GPU recovered; nobody calls Wake
	expectPlacement(t, d.notify, placement{"GPU-a", "1"})
}

func TestStartKeepsWorkingAfterIntakeCloses(t *testing.T) {
	d := newFakeDispatcher(map[string]int{})
	d.notify = make(chan placement, 1)
	q := New(d, FIFO{}, 10, 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	q.Intake() <- &jobs.Job{ID: "1", MemoryMB: 8000}
	close(q.Intake())
	d.Free("GPU-a", 80000)
	q.Wake()
	expectPlacement(t, d.notify, placement{"GPU-a", "1"})
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

func expectNoPlacement(t *testing.T, notify chan placement) {
	t.Helper()
	select {
	case p := <-notify:
		t.Fatalf("unexpected placement: %+v", p)
	case <-time.After(50 * time.Millisecond):
	}
}
