package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	LogStreamPrefix = "gpu-runner:logs:"
	ConsumerGroup   = "log-consumers"
	StreamMaxLen    = 10000 // Max entries per job log stream
)

// StreamSink implements the LogSink interface using Redis Streams
type StreamSink struct {
	client *Client
}

// NewStreamSink creates a new Redis Streams-based log sink
func NewStreamSink(client *Client) *StreamSink {
	return &StreamSink{client: client}
}

// streamKey returns the Redis key for a job's log stream
func streamKey(jobID string) string {
	return LogStreamPrefix + jobID
}

// Append adds a log message to the job's stream
func (s *StreamSink) Append(ctx context.Context, jobID string, message string) error {
	key := streamKey(jobID)

	_, err := s.client.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		MaxLen: StreamMaxLen,
		Approx: true,
		Values: map[string]any{
			"message":   message,
			"timestamp": time.Now().UnixMilli(),
		},
	}).Result()

	if err != nil {
		return fmt.Errorf("failed to append log: %w", err)
	}

	return nil
}

// Stream returns a channel that receives log messages in real-time
func (s *StreamSink) Stream(ctx context.Context, jobID string, from string) (<-chan string, error) {
	key := streamKey(jobID)
	ch := make(chan string, 100)

	if from == "" {
		from = "0" // Start from beginning
	}

	go func() {
		defer close(ch)
		lastID := from

		for {
			select {
			case <-ctx.Done():
				return
			default:
				streams, err := s.client.rdb.XRead(ctx, &redis.XReadArgs{
					Streams: []string{key, lastID},
					Count:   100,
					Block:   time.Second * 2,
				}).Result()

				if err != nil {
					if err == redis.Nil || ctx.Err() != nil {
						continue
					}
					return
				}

				for _, stream := range streams {
					for _, msg := range stream.Messages {
						lastID = msg.ID
						if message, ok := msg.Values["message"].(string); ok {
							select {
							case ch <- message:
							case <-ctx.Done():
								return
							}
						}
					}
				}
			}
		}
	}()

	return ch, nil
}

// GetLogs retrieves all logs for a job (non-streaming)
func (s *StreamSink) GetLogs(ctx context.Context, jobID string, start, end string) ([]string, error) {
	key := streamKey(jobID)

	if start == "" {
		start = "-"
	}
	if end == "" {
		end = "+"
	}

	messages, err := s.client.rdb.XRange(ctx, key, start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	logs := make([]string, 0, len(messages))
	for _, msg := range messages {
		if message, ok := msg.Values["message"].(string); ok {
			logs = append(logs, message)
		}
	}

	return logs, nil
}

// DeleteLogs removes all logs for a job
func (s *StreamSink) DeleteLogs(ctx context.Context, jobID string) error {
	return s.client.rdb.Del(ctx, streamKey(jobID)).Err()
}

// CreateConsumerGroup sets up a consumer group for job processing
func (s *StreamSink) CreateConsumerGroup(ctx context.Context, streamName, groupName string) error {
	err := s.client.rdb.XGroupCreateMkStream(ctx, streamName, groupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}
	return nil
}