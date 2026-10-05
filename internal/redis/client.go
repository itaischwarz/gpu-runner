package redis

import (
	"context"
	"fmt"
	"gpu-runner/internal/config"
	"gpu-runner/internal/logger"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func New(cfg *config.RedisConfig) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Address,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxRetries:   cfg.MaxRetries,
	})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis unavaialable: %w", err)
	}

	logger.Server.Info("✅ Redis connected", "address", cfg.Address)
	return &Client{rdb: rdb}, nil

}
func (c *Client) Close() error {
	logger.Server.Info("Closing Redis connection ...")
	return c.rdb.Close()
}

func (c *Client) Raw() *redis.Client {
	return c.rdb
}

// healthTimeout bounds a health check so a hung Redis can't hang the caller.
const healthTimeout = 2 * time.Second

// Health returns an error if Redis doesn't answer a PING.
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis unreachable: %w", err)
	}
	return nil
}
