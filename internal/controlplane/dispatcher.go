package controlplane

import (
	"fmt"
	"sync"
	"context"

	"gpu-runner/internal/jobs"
	"gpu-runner/internal/queue"
)


type Offerer interface {
	Offer(queue.Offer)
}


type Dispatcher struct {
	available map[string]int // idle slot ID → free memory (MB); health is checked on read
	released  chan finished    // slot IDs whose job just finished
	polled    chan struct{}   // signal that a poll just completed
	inboxes   map[string] chan *jobs.Job
	mu        sync.Mutex
	controlplane ControlPlane
}

type finished struct {
	slotID string
	job    *jobs.Job
}




func (d *Dispatcher) Release(slotID string, job *jobs.Job) {
    d.released <- finished{slotID, job}
}


// Polled tells the dispatcher a GPU poll just finished. It never blocks, and
// several calls before the dispatcher runs collapse into one.
func (d *Dispatcher) Polled() {
	select {
	case d.polled <- struct{}{}:
	default:
	}
}


func (d *Dispatcher) Start(ctx context.Context) error {
	if err := d.controlplane.Health(ctx); err != nil {
		return fmt.Errorf("dispatcher: control plane not ready: %w", err)
	}
	for _, s := range d.controlplane.slotTable.slots {
		if _, ok := d.inboxes[s.ID]; !ok {
			return fmt.Errorf("dispatcher: slot %s has no worker inbox", s.ID)
		}
	}
	return nil
}





func NewDispatcher(ctx context.Context, cp ControlPlane) *Dispatcher {
	free := cp.PollFreeMemory(ctx)
	release := make(chan finished)
	polled := make(chan struct{}, 1)
	// Every slot starts idle, including ones that are unhealthy right now, so
	// a GPU that recovers is offered again.
	available := make(map[string]int)
	inboxes := make(map[string] chan *jobs.Job)
	for _, s := range cp.slotTable.Snapshot() {
		available[s.ID] = free[s.ID]
		inboxes[s.ID] = make(chan *jobs.Job, 1)
	}
	return &Dispatcher{available: available, released: release, polled: polled, inboxes: inboxes, controlplane: cp}
}


func(d *Dispatcher) Dispatch(ctx context.Context) {

	go func(){
		for{
			select{
			case jobFinished := <- d.released:
				// The slot is idle again: re-read its free memory. If it went
				// unhealthy, it stays idle but isn't offered until it recovers.
				free := d.controlplane.PollFreeMemory(ctx)
				d.mu.Lock()
				d.available[jobFinished.slotID] = free[jobFinished.slotID]
				d.mu.Unlock()
			case <- d.polled:
				// A poll just ran: refresh free memory for idle slots from it.
				free := d.controlplane.FreeMemory()
				d.mu.Lock()
				for slotID := range d.available {
					d.available[slotID] = free[slotID]
				}
				d.mu.Unlock()
			case <- ctx.Done(): return
			}
		}
	}()
}


func (d *Dispatcher) Inbox() map[string] chan *jobs.Job {
	return d.inboxes
}


// Available returns the idle, healthy slots with at least memoryMB free, for
// the queue to place a job of that size.
func (d *Dispatcher) Available(memoryMB int) []queue.Offer {
	healthy := d.controlplane.FreeMemory()
	d.mu.Lock()
	defer d.mu.Unlock()
	offers := make([]queue.Offer, 0, len(d.available))
	for id, mem := range d.available {
		if _, ok := healthy[id]; ok && mem >= memoryMB {
			offers = append(offers, queue.Offer{SlotID: id, FreeMemoryMB: mem})
		}
	}
	return offers
}



func (d *Dispatcher) Place(slotID string, job *jobs.Job) error {
	healthy := d.controlplane.FreeMemory()
	d.mu.Lock()
	defer d.mu.Unlock()

	free, idle := d.available[slotID]
	if !idle {
		return fmt.Errorf("slot %s is not available", slotID) // the offer went stale
	}
	if _, ok := healthy[slotID]; !ok {
		return fmt.Errorf("slot %s is unhealthy", slotID)
	}
	if job.MemoryMB > free {
		return fmt.Errorf("slot %s has %d MB free, job needs %d", slotID, free, job.MemoryMB)
	}
	delete(d.available, slotID) // busy until Release
	d.inboxes[slotID] <- job    // that slot's worker picks it up
	jobsPlaced.WithLabelValues(slotID).Inc()
	return nil
}



//In v1,if a Job is queued the dispatcher will just try to fit a
// job in any available slot. In v2, this will be extended to not include GPUs
// reserved for larger jobs
func(d *Dispatcher) SearchGPUs()(string, int){
	var bestKey string
	var bestVal int
	for k, v := range d.available{
		if v > bestVal{
			bestKey, bestVal = k, v
		}
	}
	return bestKey, bestVal
}