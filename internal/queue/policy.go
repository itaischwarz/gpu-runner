package queue

import "gpu-runner/internal/jobs"

// Policy decides the order in which waiting jobs get a chance to run.
// Scheduling algorithms (FIFO, priority with aging, ...) are implementations
// of Policy; which GPUs a job fits on is the dispatcher's call, based on the
// job's MemoryMB.
type Policy interface {
	// Order returns pending jobs in the order they should be tried. It must
	// not modify pending.
	Order(pending []*jobs.Job) []*jobs.Job
}

// FIFO tries jobs oldest first. A job that fits nowhere is skipped, so newer
// jobs that fit can still run.
type FIFO struct{}

func (FIFO) Order(pending []*jobs.Job) []*jobs.Job {
	return append([]*jobs.Job(nil), pending...)
}
