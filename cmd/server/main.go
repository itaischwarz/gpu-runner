package main

import (
	"fmt"
	"gpu-runner/internal/api"
	"gpu-runner/internal/jobs"
	"gpu-runner/internal/store"
	"log"
	"net/http"
)

func main() {
    jobQueue := jobs.NewJobQueue(10)
    js, err := store.NewJobStore("/Users/itaischwarz/projects/gpu-runner/jobs.db")
    if err != nil {
        log.Fatalf("Unable to create Job")
    }




    // Start workers
    for i := 1; i <= 3; i++ {
        worker := jobs.NewWorker(i, jobQueue)
        worker.Start()
    }

    handlers := api.NewHandlers(jobQueue, js)
    router := api.NewRouter(handlers)

    fmt.Println("Server running on :8080")
    log.Fatal(http.ListenAndServe(":8080", router))
}
