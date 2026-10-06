package queue

import (
	"context"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"sort"
	"time"
)

var queueLogger = logger.Server

// Offer is free capacity on a slot.
type Offer struct {
	SlotID       string
	FreeMemoryMB int
}

// Dispatcher is what the queue needs from the control plane's dispatcher. The
// queue calls it; the dispatcher never calls the queue.
type Dispatcher interface {
	// Available returns the slots that can take a job of memoryMB right now.
	Available(memoryMB int) []Offer
	// Place starts job on slotID. It returns an error if the slot can no
	// longer take the job, e.g. it went unhealthy since Available.
	Place(slotID string, job *jobs.Job) error
}

// Queue holds waiting jobs and decides which one runs next. It matches jobs to
// free slots whenever a job arrives, when Wake is called (e.g. a job just
// finished), and on a fallback timer that catches slots that recovered. A
// single goroutine owns pending, so it needs no lock.
type Queue struct {
	intake     chan *jobs.Job
	wake       chan struct{}
	dispatcher Dispatcher
	policy     Policy
	recheck    time.Duration

	pending []*jobs.Job // waiting jobs, in arrival order
}

// New creates a queue. recheck is how often it retries matching without being
// woken; 0 disables the timer.
func New(dispatcher Dispatcher, policy Policy, size int, recheck time.Duration) *Queue {
	return &Queue{
		intake:     make(chan *jobs.Job, size),
		wake:       make(chan struct{}, 1),
		dispatcher: dispatcher,
		policy:     policy,
		recheck:    recheck,
	}
}

// Intake is where incoming jobs are sent. The Redis adapter sends here and
// closes it on shutdown.
func (q *Queue) Intake() chan *jobs.Job {
	return q.intake
}

// Wake asks the queue to try matching again, e.g. because a slot was released.
// It never blocks, and several calls before the queue runs collapse into one.
func (q *Queue) Wake() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Start runs the queue until ctx is cancelled.
func (q *Queue) Start(ctx context.Context) {
	go func() {
		var tick <-chan time.Time
		if q.recheck > 0 {
			ticker := time.NewTicker(q.recheck)
			defer ticker.Stop()
			tick = ticker.C
		}

		intake := q.intake
		for {
			select {
			case <-ctx.Done():
				return
			case job, ok := <-intake:
				if !ok {
					// The adapter closed intake on shutdown. Stop selecting
					// on it so the loop doesn't spin on a closed channel.
					intake = nil
					continue
				}
				q.pending = append(q.pending, job)
				queueLogger.Info("Job queued", "job_id", job.ID, "memory_mb", job.MemoryMB, "pending", len(q.pending))
			case <-q.wake:
			case <-tick:
			}
			q.match()
		}
	}()
}

// match tries each pending job, in policy order, on the slots that fit it.
// Among those, the smallest slot is tried first, so big slots stay free for
// big jobs. A job that fits nowhere stays pending.
func (q *Queue) match() {
	if len(q.pending) == 0 {
		return
	}
	placed := make(map[*jobs.Job]bool)
	for _, job := range q.policy.Order(q.pending) {
		for _, offer := range bySize(q.dispatcher.Available(job.MemoryMB)) {
			if err := q.dispatcher.Place(offer.SlotID, job); err != nil {
				// The slot changed since Available; try the next one.
				queueLogger.Warn("Placement rejected", "slot_id", offer.SlotID, "job_id", job.ID, "error", err)
				continue
			}
			placed[job] = true
			queueLogger.Info("Job placed", "job_id", job.ID, "slot_id", offer.SlotID, "memory_mb", job.MemoryMB, "free_memory_mb", offer.FreeMemoryMB)
			break
		}
	}

	remaining := q.pending[:0]
	for _, job := range q.pending {
		if !placed[job] {
			remaining = append(remaining, job)
		}
	}
	q.pending = remaining
}

func bySize(offers []Offer) []Offer {
	sort.Slice(offers, func(i, j int) bool {
		if offers[i].FreeMemoryMB != offers[j].FreeMemoryMB {
			return offers[i].FreeMemoryMB < offers[j].FreeMemoryMB
		}
		return offers[i].SlotID < offers[j].SlotID
	})
	return offers
}
