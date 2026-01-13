package redis

import (
	"context"
	"fmt"
	"gpu-runner/internal/logger"
	"time"

	"github.com/redis/go-redis/v9"
)


type Client struct {
	rdb *redis.Client
}






func New()(*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
		Password: "",
		DB: 0,
		DialTimeout: 5 * time.Second,
		ReadTimeout: 3 * time.Second,
		WriteTimeout: 3 * time.Second,
		MaxRetries: 3,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis unavaialable: %w", err)
	}

	logger.Server.Info("✅ Redis connected")
	return &Client{rdb: rdb}, nil

}
func (c *Client) Close() error {
	logger.Server.Info("Closing Redis connection ...")
	return c.rdb.Close()
}

func (c *Client) Raw() *redis.Client {
	return c.rdb
}