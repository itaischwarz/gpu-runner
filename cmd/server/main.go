package main

import (
	"context"
	"fmt"
	"gpu-runner/internal/api"
	"gpu-runner/internal/executer"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/store"
	"log"
	"net/http"
)

func main() {
    jobQueue := jobs.NewJobQueue(10)
    jobQueue.Executor = executer.NewExecutor()
    js, err := store.NewJobStore("/Users/itaischwarz/projects/gpu-runner/jobs.db")
    if err != nil {
        log.Fatalf("Unable to create Job")
    }




    // Start workers
    ctx := context.Background()
    for i := 1; i <= 3; i++ {
        worker := jobs.NewWorker(i, jobQueue)
        worker.Start(ctx)
    }

    handlers := api.NewHandlers(jobQueue, js)
    router := api.NewRouter(handlers)

    fmt.Println("Server running on :8080")
    log.Fatal(http.ListenAndServe(":8080", router))
}
