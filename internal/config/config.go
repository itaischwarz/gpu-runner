package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	Redis    RedisConfig
	Database DatabaseConfig
	Worker   WorkerConfig
	Storage  StorageConfig
	Logger   LoggerConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Address string
	Port    string
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Address      string
	Password     string
	DB           int
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	MaxRetries   int
}

// DatabaseConfig holds SQLite database configuration
type DatabaseConfig struct {
	Path string
}

// WorkerConfig holds job worker configuration
type WorkerConfig struct {
	Count          int
	JobTimeout     time.Duration
	QueueCapacity  int
	ResultsBuffer  int
}

// StorageConfig holds volume storage configuration
type StorageConfig struct {
	Volume10MBPath string
	Volume25MBPath string
	Volume50MBPath string
}

// LoggerConfig holds logging configuration
type LoggerConfig struct {
	Level   string
	LogDir  string
	LogFile string
}

// Load reads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Address: getEnv("SERVER_ADDRESS", "0.0.0.0"),
			Port:    getEnv("SERVER_PORT", "8080"),
		},
		Redis: RedisConfig{
			Address:      getEnv("REDIS_ADDRESS", "redis:6379"),
			Password:     getEnv("REDIS_PASSWORD", ""),
			DB:           getEnvAsInt("REDIS_DB", 0),
			DialTimeout:  getEnvAsDuration("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  getEnvAsDuration("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout: getEnvAsDuration("REDIS_WRITE_TIMEOUT", 3*time.Second),
			MaxRetries:   getEnvAsInt("REDIS_MAX_RETRIES", 3),
		},
		Database: DatabaseConfig{
			Path: getEnv("DATABASE_PATH", "/data/jobs.db"),
		},
		Worker: WorkerConfig{
			Count:         getEnvAsInt("WORKER_COUNT", 3),
			JobTimeout:    getEnvAsDuration("WORKER_JOB_TIMEOUT", 30*time.Second),
			QueueCapacity: getEnvAsInt("WORKER_QUEUE_CAPACITY", 10),
			ResultsBuffer: getEnvAsInt("WORKER_RESULTS_BUFFER", 100),
		},
		Storage: StorageConfig{
			Volume10MBPath: getEnv("STORAGE_VOLUME_10MB_PATH", "/var/lib/jobrunner/volumes/10mb"),
			Volume25MBPath: getEnv("STORAGE_VOLUME_25MB_PATH", "/var/lib/jobrunner/volumes/25mb"),
			Volume50MBPath: getEnv("STORAGE_VOLUME_50MB_PATH", "/var/lib/jobrunner/volumes/50mb"),
		},
		Logger: LoggerConfig{
			Level:   getEnv("LOG_LEVEL", "info"),
			LogDir:  getEnv("LOG_DIR", filepath.Join(os.Getenv("HOME"), "log", "gpu-runner")),
			LogFile: getEnv("LOG_FILE", "server.log"),
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("SERVER_PORT cannot be empty")
	}

	if c.Redis.Address == "" {
		return fmt.Errorf("REDIS_ADDRESS cannot be empty")
	}

	if c.Database.Path == "" {
		return fmt.Errorf("DATABASE_PATH cannot be empty")
	}

	if c.Worker.Count < 1 {
		return fmt.Errorf("WORKER_COUNT must be at least 1, got %d", c.Worker.Count)
	}

	if c.Worker.JobTimeout < 1*time.Second {
		return fmt.Errorf("WORKER_JOB_TIMEOUT must be at least 1s, got %s", c.Worker.JobTimeout)
	}

	if c.Storage.Volume10MBPath == "" || c.Storage.Volume25MBPath == "" || c.Storage.Volume50MBPath == "" {
		return fmt.Errorf("storage volume paths cannot be empty")
	}

	return nil
}

// ServerAddr returns the full server address (host:port)
func (c *Config) ServerAddr() string {
	return c.Server.Address + ":" + c.Server.Port
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
