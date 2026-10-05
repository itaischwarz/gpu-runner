package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestClient(addr string) *Client {
	return &Client{rdb: redis.NewClient(&redis.Options{
		Addr:        addr,
		DialTimeout: 200 * time.Millisecond,
		MaxRetries:  -1,
	})}
}

func TestHealthFailsWhenRedisUnreachable(t *testing.T) {
	c := newTestClient("127.0.0.1:1")
	t.Cleanup(func() { c.rdb.Close() })

	if err := c.Health(context.Background()); err == nil {
		t.Error("expected Client.Health to fail for an unreachable Redis")
	}
	if err := NewStreamSink(c).Health(context.Background()); err == nil {
		t.Error("expected StreamSink.Health to fail for an unreachable Redis")
	}
}

// Runs against a real Redis when one is reachable (REDIS_TEST_ADDR, default
// localhost:6379); skipped otherwise, e.g. in CI.
func TestHealthPassesAgainstRealRedis(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	c := newTestClient(addr)
	t.Cleanup(func() { c.rdb.Close() })

	if err := c.rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("no Redis at %s: %v", addr, err)
	}

	if err := c.Health(context.Background()); err != nil {
		t.Errorf("Client.Health failed: %v", err)
	}
	if err := NewStreamSink(c).Health(context.Background()); err != nil {
		t.Errorf("StreamSink.Health failed: %v", err)
	}
}
