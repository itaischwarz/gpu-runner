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
	available map[string]int // slot ID → free memory (MB)
	released  chan finished    // slot IDs whose job just finished
	polled    chan *jobs.Job  // signal that a poll just completed
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
	available := cp.PollFreeMemory(ctx)
	release := make(chan finished)
	polled := make(chan *jobs.Job)
	inboxes := make(map[string] chan *jobs.Job)
	return &Dispatcher{available: available, released: release, polled: polled, inboxes: inboxes, controlplane: cp}
}


func(d *Dispatcher) Dispatch(ctx context.Context) {

	go func(){
		for{
			select{
			case jobFinished := <- d.released:
				// TODO: doesn't compile as written: queue.Offer is a type, not a
				// function, and a job has no FreeMemoryMB.
				// queue.Offer(jobFinished.slot_id, jobFinished.job.FreeMemoryMB)
				_ = jobFinished
			case size := <- d.polled:
				// TODO: doesn't compile as written: size is a *jobs.Job, and
				// queue.Place and memory don't exist.
				// bestGPU, bestMemory := d.SearchGPUs()
				// if size > bestMemory{
				// 	queue.Place(bestGPU, memory)
				// }
				_ = size
			case <- ctx.Done(): return
			}
		}
	}()
}


func (d *Dispatcher) Inbox() map[string] chan *jobs.Job {
	return d.inboxes
}



func (d *Dispatcher) Place(slotID string, job *jobs.Job) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	free, idle := d.available[slotID]
	if !idle {
		return fmt.Errorf("slot %s is not available", slotID) // the offer went stale
	}
	if job.MemoryMB > free {
		return fmt.Errorf("slot %s has %d MB free, job needs %d", slotID, free, job.MemoryMB)
	}
	delete(d.available, slotID) // busy until Release
	d.inboxes[slotID] <- job    // that slot's worker picks it up
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