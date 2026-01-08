package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
)

var redisLogger = logger.Server

const (
	JobQueueKey      = "gpu-runner:jobs:pending"
	JobProcessingKey = "gpu-runner:jobs:processing"
)

// JobMessage represents a job in the Redis queue

// Enqueue adds a job to the pending queue
func (c *Client) Enqueue(ctx context.Context, job jobs.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		if job.Logger != nil {
			job.Logger.Error("Failed to marshal job for Redis queue", logger.Item("error", err))
		}
		redisLogger.Error("Failed to marshal job", "error", err, "job_id", job.ID)
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := c.rdb.LPush(ctx, JobQueueKey, data).Err(); err != nil {
		if job.Logger != nil {
			job.Logger.Error("Failed to enqueue job to Redis", logger.Item("error", err))
		}
		redisLogger.Error("Redis LPush failed", "error", err, "job_id", job.ID, "queue", JobQueueKey)
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	if job.Logger != nil {
		job.Logger.Info("Job enqueued to Redis queue")
	}
	redisLogger.Info("Job enqueued successfully", "job_id", job.ID, "queue", JobQueueKey)
	return nil
}

// Dequeue blocks until a job is available, then returns it
// Uses BRPOPLPUSH for reliable queue processing (atomic move to processing list)
func (c *Client) Dequeue(ctx context.Context, timeout time.Duration) (*jobs.Job, error) {
	result, err := c.rdb.BRPopLPush(ctx, JobQueueKey, JobProcessingKey, timeout).Result()
	if err != nil {
		// Don't log timeout errors as they're expected during normal operation
		if err.Error() != "redis: nil" {
			redisLogger.Error("Redis dequeue failed", "error", err, "queue", JobQueueKey)
		}
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	var job jobs.Job
	if err := json.Unmarshal([]byte(result), &job); err != nil {
		redisLogger.Error("Failed to unmarshal dequeued job", "error", err)
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	redisLogger.Info("Job dequeued from Redis", "job_id", job.ID, "status", job.Status)
	return &job, nil
}

// Acknowledge removes a completed job from the processing list
func (c *Client) Acknowledge(ctx context.Context, job jobs.Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		if job.Logger != nil {
			job.Logger.Error("Failed to marshal job for acknowledgment", logger.Item("error", err))
		}
		redisLogger.Error("Failed to marshal job for acknowledgment", "error", err, "job_id", job.ID)
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := c.rdb.LRem(ctx, JobProcessingKey, 1, data).Err(); err != nil {
		if job.Logger != nil {
			job.Logger.Error("Failed to acknowledge job in Redis", logger.Item("error", err))
		}
		redisLogger.Error("Redis LRem failed during acknowledgment", "error", err, "job_id", job.ID, "queue", JobProcessingKey)
		return fmt.Errorf("failed to acknowledge job: %w", err)
	}

	if job.Logger != nil {
		job.Logger.Info("Job acknowledged in Redis")
	}
	redisLogger.Info("Job acknowledged and removed from processing queue", "job_id", job.ID, "status", job.Status)
	return nil
}

// QueueLength returns the number of pending jobs
func (c *Client) QueueLength(ctx context.Context) (int64, error) {
	length, err := c.rdb.LLen(ctx, JobQueueKey).Result()
	if err != nil {
		redisLogger.Error("Failed to get queue length", "error", err, "queue", JobQueueKey)
		return 0, err
	}
	return length, nil
}


// RequeueStaleJobs moves jobs from processing back to pending (for crash recovery)
func (c *Client) RequeueStaleJobs(ctx context.Context) (int64, error) {
	redisLogger.Info("Starting stale job requeue process", "processing_queue", JobProcessingKey)
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
		redisLogger.Info("Requeued stale job", "count", count)
	}
	if count > 0 {
		redisLogger.Info("Stale job requeue completed", "total_requeued", count)
	} else {
		redisLogger.Info("No stale jobs found to requeue")
	}
	return count, nil
}

func (c *Client) StartRedisAdapter(ctx context.Context, jobQueue *jobs.JobQueue, sink *StreamSink) error {
	redisLogger.Info("Starting Redis adapter", "queue", JobQueueKey)
	go func() {
		defer func() {
			redisLogger.Info("Redis adapter shutting down, closing job queue")
			close(jobQueue.Queue)
		}()
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
				redisLogger.Info("Passing job to worker queue", "job_id", job.ID)

				select {
				case <-ctx.Done():
					redisLogger.Warn("Context cancelled while sending job to queue", "job_id", job.ID)
					return
				case jobQueue.Queue <- job:
					redisLogger.Info("Job sent to worker queue successfully", "job_id", job.ID)
				}
				}
		}
	}()
	return nil
}



		
	

