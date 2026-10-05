package queue

import "gpu-runner/internal/jobs"

// Policy decides which waiting job runs when a slot is offered. Scheduling
// algorithms (FIFO, priority with aging, ...) are implementations of Policy;
// the Queue's mechanics stay the same whichever one is plugged in.
type Policy interface {
	// Pick returns the index in pending of the job to place on offer, or -1
	// to leave the offer unused. pending is in arrival order.
	Pick(pending []*jobs.Job, offer Offer) int
}

// FIFO picks the oldest job that fits the offer. A job that doesn't fit is
// skipped, so smaller jobs behind it can still run on smaller slots.
type FIFO struct{}

func (FIFO) Pick(pending []*jobs.Job, offer Offer) int {
	for i, job := range pending {
		if Fits(job, offer) {
			return i
		}
	}
	return -1
}

// Fits reports whether job can run on offer. A job with MemoryMB 0 is
// exclusive and fits any offer, since in v1 every offer is a whole idle GPU.
func Fits(job *jobs.Job, offer Offer) bool {
	return job.MemoryMB <= offer.FreeMemoryMB
}
