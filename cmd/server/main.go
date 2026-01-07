package main

import (
	"context"
	"fmt"
	"gpu-runner/internal/api"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/store"
	"gpu-runner/internal/redis"
	"log"
	"net/http"
)


func main() {
    client, err := redis.New()
    streamSink := redis.NewStreamSink(client)
    jobQueue := jobs.NewJobQueue(10)
    jobQueue.Executor = executer.NewExecutor()
    js, err := store.NewJobStore("/Users/itaischwarz/projects/gpu-runner/jobs.db")
    if err != nil {
        log.Fatalf("Unable to create Job")
    }
    fmt.Print("CREATED JOB")
    // Start workers
    ctx := context.Background()
    client.StartRedisAdapter(ctx, jobQueue, streamSink)
    results := make(chan *jobs.Job, 100)
    for i := 1; i <= 3; i++ {
        worker := jobs.NewWorker(i, jobQueue, results)
        worker.Start(ctx)
        
    }
    client.StartRedisAcknowledger(ctx, results)


    handlers := api.NewHandlers(jobQueue, js, ctx, streamSink, client)
    router := api.NewRouter(handlers)

    fmt.Println("Server running on :8080")
    log.Fatal(http.ListenAndServe(":8080", router))
}
