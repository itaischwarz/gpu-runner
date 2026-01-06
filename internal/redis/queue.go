package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
)

const (
	JobQueueKey      = "gpu-runner:jobs:pending"
	JobProcessingKey = "gpu-runner:jobs:processing"
)

// JobMessage represents a job in the Redis queue

// Enqueue adds a job to the pending queue
func (c *Client) Enqueue(ctx context.Context, job jobs.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := c.rdb.LPush(ctx, JobQueueKey, data).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	return nil
}

// Dequeue blocks until a job is available, then returns it
// Uses BRPOPLPUSH for reliable queue processing (atomic move to processing list)
func (c *Client) Dequeue(ctx context.Context, timeout time.Duration) (*jobs.Job, error) {
	result, err := c.rdb.BRPopLPush(ctx, JobQueueKey, JobProcessingKey, timeout).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(result), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// Acknowledge removes a completed job from the processing list
func (c *Client) Acknowledge(ctx context.Context, job jobs.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := c.rdb.LRem(ctx, JobProcessingKey, 1, data).Err(); err != nil {
		return fmt.Errorf("failed to acknowledge job: %w", err)
	}

	return nil
}

// QueueLength returns the number of pending jobs
func (c *Client) QueueLength(ctx context.Context) (int64, error) {
	return c.rdb.LLen(ctx, JobQueueKey).Result()
}


// RequeueStaleJobs moves jobs from processing back to pending (for crash recovery)
func (c *Client) RequeueStaleJobs(ctx context.Context) (int64, error) {
	var count int64
	for {
		result, err := c.rdb.RPopLPush(ctx, JobProcessingKey, JobQueueKey).Result()
		if err != nil {
			break // No more items
		}
		if result == "" {
			break
		}
		count++
	}
	return count, nil
}

func (c *Client) StartRedisAdapter(ctx context.Context, jobQueue *jobs.JobQueue, sink *StreamSink) error {
	go func() {
		defer close(jobQueue.Queue)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				job, err := c.Dequeue(ctx, 5*time.Second)
				if err != nil {
					continue
				}
				// Recreate the logger after deserialization (Logger can't be serialized to JSON)
				job.Logger = logger.NewJobLogger(ctx, job.ID, sink)
				select {
				case <-ctx.Done():
					return
				case jobQueue.Queue <- job:
				}
				}
		}
	}()
	return nil
}

		
	

