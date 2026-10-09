package redis

import (
	"context"
	"os"
	"testing"

	"gpu-runner/internal/jobs"

	"github.com/redis/go-redis/v9"
)

// newQueueTestClient connects to a real Redis on DB 15 and empties it after
// the test. It skips when Redis isn't reachable (e.g. CI) or DB 15 already
// holds data, so it never touches anything it didn't create.
func newQueueTestClient(t *testing.T) *Client {
	t.Helper()
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	c := &Client{rdb: redis.NewClient(&redis.Options{Addr: addr, DB: 15})}
	t.Cleanup(func() { _ = c.rdb.Close() })

	ctx := context.Background()
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis at %s: %v", addr, err)
	}
	if n, err := c.rdb.DBSize(ctx).Result(); err != nil || n != 0 {
		t.Skipf("Redis DB 15 is not empty (%d keys); not touching it", n)
	}
	t.Cleanup(func() { _ = c.rdb.FlushDB(context.Background()).Err() })
	return c
}

func listLen(t *testing.T, c *Client, key string) int64 {
	t.Helper()
	n, err := c.rdb.LLen(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("LLEN %s: %v", key, err)
	}
	return n
}

func TestAcknowledgeRemovesJobEvenAfterItChanged(t *testing.T) {
	c := newQueueTestClient(t)
	ctx := context.Background()

	if err := c.Enqueue(ctx, jobs.Job{ID: "1", Command: "echo hi", Status: jobs.StatusPending, MemoryMB: 8000}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	job, err := c.Dequeue(ctx, 0)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if listLen(t, c, JobProcessingKey) != 1 {
		t.Fatal("expected the job in the processing list after Dequeue")
	}

	// The worker changes the job while it runs; this used to break acknowledgement.
	job.Status = jobs.StatusSuccess
	job.Error = "something"

	if err := c.Acknowledge(ctx, *job); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
	if n := listLen(t, c, JobProcessingKey); n != 0 {
		t.Fatalf("expected an empty processing list, got %d", n)
	}

	// A second acknowledgement finds nothing and says so.
	if err := c.Acknowledge(ctx, *job); err == nil {
		t.Error("expected an error when the job is no longer in the processing list")
	}
}

func TestAcknowledgeWithoutPayloadFails(t *testing.T) {
	c := &Client{rdb: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})}
	t.Cleanup(func() { _ = c.rdb.Close() })

	if err := c.Acknowledge(context.Background(), jobs.Job{ID: "1"}); err == nil {
		t.Error("expected an error for a job that was never dequeued")
	}
}

func TestRequeueStaleJobsReturnsRunningJobsToPending(t *testing.T) {
	c := newQueueTestClient(t)
	ctx := context.Background()

	for _, id := range []string{"1", "2"} {
		if err := c.Enqueue(ctx, jobs.Job{ID: id, MemoryMB: 8000}); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
		if _, err := c.Dequeue(ctx, 0); err != nil {
			t.Fatalf("Dequeue: %v", err)
		}
	}
	// Simulated crash: both jobs are stuck in the processing list.

	n, err := c.RequeueStaleJobs(ctx)
	if err != nil {
		t.Fatalf("RequeueStaleJobs: %v", err)
	}
	if n != 2 || listLen(t, c, JobQueueKey) != 2 || listLen(t, c, JobProcessingKey) != 0 {
		t.Fatalf("expected 2 jobs moved back to pending, got n=%d pending=%d processing=%d",
			n, listLen(t, c, JobQueueKey), listLen(t, c, JobProcessingKey))
	}
}
