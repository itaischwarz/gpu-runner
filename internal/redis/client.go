package redis

import (
	"context"
	"fmt"
	"gpu-runner/internal/config"
	"gpu-runner/internal/logger"

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
