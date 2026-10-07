package main

import (
	"context"
	"gpu-runner/internal/api"
	"gpu-runner/internal/config"
	"gpu-runner/internal/controlplane"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/logger"
	"gpu-runner/internal/queue"
	"gpu-runner/internal/redis"
	"gpu-runner/internal/store"
	"log"
	"net/http"
	"os/signal"
	"sort"
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
		"gpu_enabled", cfg.GPU.Enabled,
	)

	ctx := context.Background()

	var cp *controlplane.ControlPlane
	if cfg.GPU.Enabled {
		serverLogger.Info("Initializing control plane", "nvidia_smi_path", cfg.GPU.NvidiaSMIPath, "poll_interval", cfg.GPU.PollInterval)
		cp, err = controlplane.NewControlPlane(ctx, cfg.GPU.NvidiaSMIPath, cfg.GPU.PollInterval)
		if err != nil {
			serverLogger.Error("Failed to initialize control plane", "error", err)
			log.Fatalf("Failed to initialize control plane: %v", err)
		}
		cp.UpdateGPUHealth(ctx)
		serverLogger.Info("Control plane started, polling GPU health")
	} else {
		serverLogger.Info("GPU disabled, skipping control plane")
	}

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

	results := make(chan *jobs.Job, cfg.Worker.ResultsBuffer)
	serverLogger.Info("Created results channel", "buffer_size", cfg.Worker.ResultsBuffer)

	// Where the Redis adapter delivers jobs: the GPU queue, or the CPU workers.
	var intake chan<- *jobs.Job

	if cfg.GPU.Enabled {
		// Kill jobs that use more GPU memory than they requested.
		jobQueue.Executor.SetMemoryLimits(
			executer.NvidiaSMIMemoryProbe(cfg.GPU.NvidiaSMIPath),
			cfg.GPU.MemoryCheckInterval,
			cfg.GPU.MemoryGraceChecks,
		)
		serverLogger.Info("GPU memory limits enforced", "check_interval", cfg.GPU.MemoryCheckInterval, "grace_checks", cfg.GPU.MemoryGraceChecks)

		dispatcher := controlplane.NewDispatcher(ctx, *cp)
		jobQ := queue.New(dispatcher, queue.FIFO{}, cfg.Worker.QueueCapacity, cfg.GPU.PollInterval)

		// One worker per GPU, started before the queue so every inbox has a
		// reader before the first Place.
		inboxes := dispatcher.Inbox()
		slotIDs := make([]string, 0, len(inboxes))
		for id := range inboxes {
			slotIDs = append(slotIDs, id)
		}
		sort.Strings(slotIDs)
		for i, slotID := range slotIDs {
			worker := jobs.NewWorker(i+1, jobQueue, results)
			worker.Inbox = inboxes[slotID]
			worker.Env = []string{"CUDA_VISIBLE_DEVICES=" + slotID}
			worker.OnIdle = func(job *jobs.Job) {
				dispatcher.Release(slotID, job)
				jobQ.Wake()
			}
			worker.Start(ctx)
			serverLogger.Info("Started GPU worker", "worker_id", i+1, "slot_id", slotID)
		}

		if err := dispatcher.Start(ctx); err != nil {
			serverLogger.Error("Failed to start dispatcher", "error", err)
			log.Fatalf("Failed to start dispatcher: %v", err)
		}
		dispatcher.Dispatch(ctx)
		jobQ.Start(ctx)
		intake = jobQ.Intake()
		serverLogger.Info("GPU scheduling started", "gpus", len(slotIDs))
	} else {
		serverLogger.Info("Starting workers", "count", cfg.Worker.Count)
		for i := 1; i <= cfg.Worker.Count; i++ {
			worker := jobs.NewWorker(i, jobQueue, results)
			worker.Start(ctx)
		}
		intake = jobQueue.Queue
		serverLogger.Info("All workers started successfully")
	}

	serverLogger.Info("Starting Redis adapter")
	if err := client.StartRedisAdapter(ctx, intake, streamSink); err != nil {
		serverLogger.Error("Failed to start Redis adapter", "error", err)
		log.Fatalf("Failed to start Redis adapter: %v", err)
	}
	quitCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)

	handlers := api.NewHandlers(jobQueue, js, ctx, streamSink, client, stop)
	if cp != nil {
		// Reject jobs bigger than the largest GPU at submit time.
		handlers.Capacity = cp
	}
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