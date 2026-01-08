package main

import (
	"context"
	"gpu-runner/internal/api"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"gpu-runner/internal/redis"
	"gpu-runner/internal/store"
	"log"
	"net/http"
)

var serverLogger = logger.Server


func main() {
    serverLogger.Info("Starting GPU Runner server")

    serverLogger.Info("Initializing Redis client")
    client, err := redis.New()
    if err != nil {
        serverLogger.Error("Failed to create Redis client", "error", err)
        log.Fatalf("Failed to create Redis client: %v", err)
    }
    serverLogger.Info("Redis client initialized successfully")

    streamSink := redis.NewStreamSink(client)
    serverLogger.Info("Stream sink created")

    jobQueue := jobs.NewJobQueue(10)
    serverLogger.Info("Job queue created", "capacity", 10)

    jobQueue.Executor = executer.NewExecutor()
    serverLogger.Info("Job executor created")

    serverLogger.Info("Initializing job store database", "path", "/Users/itaischwarz/projects/gpu-runner/jobs.db")
    js, err := store.NewJobStore("/Users/itaischwarz/projects/gpu-runner/jobs.db")
    if err != nil {
        serverLogger.Error("Failed to create job store", "error", err)
        log.Fatalf("Unable to create job store: %v", err)
    }

    ctx := context.Background()

    serverLogger.Info("Starting Redis adapter")
    if err := client.StartRedisAdapter(ctx, jobQueue, streamSink); err != nil {
        serverLogger.Error("Failed to start Redis adapter", "error", err)
        log.Fatalf("Failed to start Redis adapter: %v", err)
    }

    results := make(chan *jobs.Job, 100)
    serverLogger.Info("Created results channel", "buffer_size", 100)

    numWorkers := 3
    serverLogger.Info("Starting workers", "count", numWorkers)
    for i := 1; i <= numWorkers; i++ {
        worker := jobs.NewWorker(i, jobQueue, results)
        worker.Start(ctx)
    }
    serverLogger.Info("All workers started successfully")

    handlers := api.NewHandlers(jobQueue, js, ctx, streamSink, client)
    serverLogger.Info("API handlers initialized")

    handlers.StartRedisAcknowledger(ctx, results)
    serverLogger.Info("Redis acknowledger started")

    router := api.NewRouter(handlers)
    serverLogger.Info("HTTP router configured")

    serverAddr := ":8080"
    serverLogger.Info("Starting HTTP server", "address", serverAddr)
    if err := http.ListenAndServe(serverAddr, router); err != nil {
        serverLogger.Error("Server failed", "error", err)
        log.Fatal(err)
    }
}
