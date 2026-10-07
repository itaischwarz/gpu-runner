package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any existing env vars
	clearEnvVars()

	// Set HOME for logger path
	os.Setenv("HOME", "/tmp")
	defer os.Unsetenv("HOME")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check server defaults
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("expected server address '0.0.0.0', got '%s'", cfg.Server.Address)
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("expected server port '8080', got '%s'", cfg.Server.Port)
	}

	// Check Redis defaults
	if cfg.Redis.Address != "redis:6379" {
		t.Errorf("expected redis address 'redis:6379', got '%s'", cfg.Redis.Address)
	}
	if cfg.Redis.DB != 0 {
		t.Errorf("expected redis DB 0, got %d", cfg.Redis.DB)
	}

	// Check worker defaults
	if cfg.Worker.Count != 3 {
		t.Errorf("expected worker count 3, got %d", cfg.Worker.Count)
	}
	if cfg.Worker.JobTimeout != 30*time.Second {
		t.Errorf("expected job timeout 30s, got %s", cfg.Worker.JobTimeout)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	clearEnvVars()

	os.Setenv("HOME", "/tmp")
	os.Setenv("SERVER_PORT", "9000")
	os.Setenv("REDIS_ADDRESS", "localhost:6380")
	os.Setenv("WORKER_COUNT", "5")
	os.Setenv("WORKER_JOB_TIMEOUT", "1m")
	os.Setenv("DATABASE_PATH", "/custom/path/jobs.db")

	defer clearEnvVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != "9000" {
		t.Errorf("expected server port '9000', got '%s'", cfg.Server.Port)
	}
	if cfg.Redis.Address != "localhost:6380" {
		t.Errorf("expected redis address 'localhost:6380', got '%s'", cfg.Redis.Address)
	}
	if cfg.Worker.Count != 5 {
		t.Errorf("expected worker count 5, got %d", cfg.Worker.Count)
	}
	if cfg.Worker.JobTimeout != 1*time.Minute {
		t.Errorf("expected job timeout 1m, got %s", cfg.Worker.JobTimeout)
	}
	if cfg.Database.Path != "/custom/path/jobs.db" {
		t.Errorf("expected database path '/custom/path/jobs.db', got '%s'", cfg.Database.Path)
	}
}

func TestValidate_InvalidWorkerCount(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Port: "8080"},
		Redis:  RedisConfig{Address: "redis:6379"},
		Database: DatabaseConfig{Path: "/data/jobs.db"},
		Worker: WorkerConfig{
			Count:      0, // Invalid
			JobTimeout: 30 * time.Second,
		},
		Storage: StorageConfig{
			Volume10MBPath: "/path/10mb",
			Volume25MBPath: "/path/25mb",
			Volume50MBPath: "/path/50mb",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for worker count 0, got nil")
	}
}

func TestValidate_InvalidJobTimeout(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Port: "8080"},
		Redis:  RedisConfig{Address: "redis:6379"},
		Database: DatabaseConfig{Path: "/data/jobs.db"},
		Worker: WorkerConfig{
			Count:      3,
			JobTimeout: 500 * time.Millisecond, // Invalid (< 1s)
		},
		Storage: StorageConfig{
			Volume10MBPath: "/path/10mb",
			Volume25MBPath: "/path/25mb",
			Volume50MBPath: "/path/50mb",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for job timeout < 1s, got nil")
	}
}

func TestLoad_GPUDefaultsAndOverrides(t *testing.T) {
	clearEnvVars()
	defer clearEnvVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if cfg.GPU.Enabled {
		t.Error("expected GPU to be disabled by default")
	}
	if cfg.GPU.NvidiaSMIPath != "nvidia-smi" {
		t.Errorf("expected nvidia-smi path 'nvidia-smi', got '%s'", cfg.GPU.NvidiaSMIPath)
	}
	if cfg.GPU.PollInterval != 10*time.Second {
		t.Errorf("expected GPU poll interval 10s, got %s", cfg.GPU.PollInterval)
	}
	if cfg.GPU.MemoryCheckInterval != 2*time.Second || cfg.GPU.MemoryGraceChecks != 2 {
		t.Errorf("expected memory checks every 2s with 2 grace checks, got %s / %d", cfg.GPU.MemoryCheckInterval, cfg.GPU.MemoryGraceChecks)
	}

	os.Setenv("GPU_ENABLED", "true")
	os.Setenv("NVIDIA_SMI_PATH", "/usr/bin/nvidia-smi")
	os.Setenv("GPU_POLL_INTERVAL", "5s")

	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if !cfg.GPU.Enabled || cfg.GPU.NvidiaSMIPath != "/usr/bin/nvidia-smi" || cfg.GPU.PollInterval != 5*time.Second {
		t.Errorf("unexpected GPU config: %+v", cfg.GPU)
	}
}

func TestValidate_InvalidGPUPollInterval(t *testing.T) {
	clearEnvVars()
	defer clearEnvVars()

	os.Setenv("GPU_ENABLED", "true")
	os.Setenv("GPU_POLL_INTERVAL", "100ms")

	if _, err := Load(); err == nil {
		t.Error("expected validation error for GPU poll interval < 1s, got nil")
	}
}

func TestServerAddr(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Address: "127.0.0.1",
			Port:    "9090",
		},
	}

	expected := "127.0.0.1:9090"
	if got := cfg.ServerAddr(); got != expected {
		t.Errorf("ServerAddr() = %s, want %s", got, expected)
	}
}

func clearEnvVars() {
	vars := []string{
		"SERVER_ADDRESS", "SERVER_PORT",
		"REDIS_ADDRESS", "REDIS_PASSWORD", "REDIS_DB",
		"REDIS_DIAL_TIMEOUT", "REDIS_READ_TIMEOUT", "REDIS_WRITE_TIMEOUT", "REDIS_MAX_RETRIES",
		"DATABASE_PATH",
		"WORKER_COUNT", "WORKER_JOB_TIMEOUT", "WORKER_QUEUE_CAPACITY", "WORKER_RESULTS_BUFFER",
		"STORAGE_VOLUME_10MB_PATH", "STORAGE_VOLUME_25MB_PATH", "STORAGE_VOLUME_50MB_PATH",
		"LOG_LEVEL", "LOG_DIR", "LOG_FILE",
		"GPU_ENABLED", "NVIDIA_SMI_PATH", "GPU_POLL_INTERVAL",
		"GPU_MEMORY_CHECK_INTERVAL", "GPU_MEMORY_GRACE_CHECKS",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}
