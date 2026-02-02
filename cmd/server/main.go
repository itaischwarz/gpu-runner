package main

import (
	"context"
	"gpu-runner/internal/api"
	"gpu-runner/internal/config"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"gpu-runner/internal/redis"
	"gpu-runner/internal/store"
	"log"
	"net/http"
	"os/signal"
	"syscall"
)

var serverLogger = logger.Server

func main() {
	// Load configuration from environment variables
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger with configuration
	if err := logger.InitLogger(&cfg.Logger); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	serverLogger = logger.Server

	serverLogger.Info("Starting GPU Runner server")
	serverLogger.Info("Configuration loaded",
		"server_addr", cfg.ServerAddr(),
		"redis_addr", cfg.Redis.Address,
		"database_path", cfg.Database.Path,
		"worker_count", cfg.Worker.Count,
		"job_timeout", cfg.Worker.JobTimeout,
	)

	serverLogger.Info("Initializing Redis client")
	client, err := redis.New(&cfg.Redis)
	if err != nil {
		serverLogger.Error("Failed to create Redis client", "error", err)
		log.Fatalf("Failed to create Redis client: %v", err)
	}
	serverLogger.Info("Redis client initialized successfully")

	streamSink := redis.NewStreamSink(client)
	serverLogger.Info("Stream sink created")

	jobQueue := jobs.NewJobQueue(cfg.Worker.QueueCapacity)
	serverLogger.Info("Job queue created", "capacity", cfg.Worker.QueueCapacity)

	jobQueue.Executor = executer.NewExecutor(cfg.Worker.JobTimeout)
	serverLogger.Info("Job executor created")

	serverLogger.Info("Initializing job store database", "path", cfg.Database.Path)
	js, err := store.NewJobStore(cfg.Database.Path)
	if err != nil {
		serverLogger.Error("Failed to create job store", "error", err)
		log.Fatalf("Unable to create job store: %v", err)
	}

	// Initialize volume paths
	jobs.InitVolumePaths(&cfg.Storage)

	ctx := context.Background()

	serverLogger.Info("Starting Redis adapter")
	if err := client.StartRedisAdapter(ctx, jobQueue, streamSink); err != nil {
		serverLogger.Error("Failed to start Redis adapter", "error", err)
		log.Fatalf("Failed to start Redis adapter: %v", err)
	}

	results := make(chan *jobs.Job, cfg.Worker.ResultsBuffer)
	serverLogger.Info("Created results channel", "buffer_size", cfg.Worker.ResultsBuffer)

	serverLogger.Info("Starting workers", "count", cfg.Worker.Count)
	for i := 1; i <= cfg.Worker.Count; i++ {
		worker := jobs.NewWorker(i, jobQueue, results)
		worker.Start(ctx)
	}
	serverLogger.Info("All workers started successfully")
	quitCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)

	handlers := api.NewHandlers(jobQueue, js, ctx, streamSink, client, stop)
	serverLogger.Info("API handlers initialized")

	handlers.StartRedisAcknowledger(ctx, results)
	serverLogger.Info("Redis acknowledger started")

	router := api.NewRouter(handlers)
	serverLogger.Info("HTTP router configured")

	server := &http.Server{
		Addr:    cfg.ServerAddr(),
		Handler: router,
	}
	go func() {
		serverLogger.Info("Server listening", "address", cfg.ServerAddr())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverLogger.Error("Server failed", "error", err)
			log.Fatal(err)
		}
	}()

	<-quitCtx.Done()

	ctx = context.Background()
	serverLogger.Info("Server shutting down gracefully")
	if err := server.Shutdown(ctx); err != nil {
		serverLogger.Error("Error during shutdown", "error", err)
	} else {
		serverLogger.Info("Server shutdown complete")
	}

}