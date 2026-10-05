package queue

import (
	"context"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"sort"
)

var queueLogger = logger.Server

// Offer is free capacity on a slot, reported by the dispatcher.
type Offer struct {
	SlotID       string
	FreeMemoryMB int
}

// Placer starts a chosen job on a slot. The control plane's dispatcher
// implements it; the queue never imports the control plane.
type Placer interface {
	// Place starts job on slotID. It returns an error if the slot can no
	// longer take the job, e.g. it went unhealthy after the offer was made.
	// Place is called from the queue's goroutine, so it must not call
	// Queue.Offer synchronously.
	Place(slotID string, job *jobs.Job) error
}

// Queue holds waiting jobs and decides which one runs when capacity frees up.
// Jobs arrive on intake (from the Redis adapter) and capacity arrives as
// offers (from the dispatcher). A single goroutine owns pending and open, so
// neither needs a lock.
type Queue struct {
	intake chan *jobs.Job
	offers chan Offer
	placer Placer
	policy Policy

	pending []*jobs.Job      // waiting jobs, in arrival order
	open    map[string]Offer // slot ID → latest unused offer
}

func New(placer Placer, policy Policy, size int) *Queue {
	return &Queue{
		intake: make(chan *jobs.Job, size),
		offers: make(chan Offer, size),
		placer: placer,
		policy: policy,
		open:   make(map[string]Offer),
	}
}

// Intake is where incoming jobs are sent. The Redis adapter sends here and
// closes it on shutdown.
func (q *Queue) Intake() chan *jobs.Job {
	return q.intake
}

// Offer reports free capacity on a slot. The dispatcher calls it at startup,
// when a job finishes, and when a poll shows a slot changed.
func (q *Queue) Offer(o Offer) {
	q.offers <- o
}

// Start runs the queue until ctx is cancelled. Every new job or offer
// triggers a matching pass.
func (q *Queue) Start(ctx context.Context) {
	go func() {
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
			case offer := <-q.offers:
				q.open[offer.SlotID] = offer
			}
			q.match()
		}
	}()
}

// match asks the policy to fill each open offer. Offers are considered
// smallest first, so small jobs land on small slots before the large slots
// that big jobs need are handed out.
func (q *Queue) match() {
	for _, offer := range q.openBySize() {
		if len(q.pending) == 0 {
			return
		}
		i := q.policy.Pick(q.pending, offer)
		if i < 0 {
			continue
		}
		job := q.pending[i]
		delete(q.open, offer.SlotID)

		if err := q.placer.Place(offer.SlotID, job); err != nil {
			// The offer is stale. Drop it and keep the job; the dispatcher
			// offers the slot again when it changes.
			queueLogger.Warn("Offer rejected, dropping it", "slot_id", offer.SlotID, "job_id", job.ID, "error", err)
			continue
		}
		q.pending = append(q.pending[:i], q.pending[i+1:]...)
		queueLogger.Info("Job placed", "job_id", job.ID, "slot_id", offer.SlotID, "memory_mb", job.MemoryMB, "free_memory_mb", offer.FreeMemoryMB)
	}
}

func (q *Queue) openBySize() []Offer {
	offers := make([]Offer, 0, len(q.open))
	for _, o := range q.open {
		offers = append(offers, o)
	}
	sort.Slice(offers, func(i, j int) bool {
		if offers[i].FreeMemoryMB != offers[j].FreeMemoryMB {
			return offers[i].FreeMemoryMB < offers[j].FreeMemoryMB
		}
		return offers[i].SlotID < offers[j].SlotID
	})
	return offers
}
