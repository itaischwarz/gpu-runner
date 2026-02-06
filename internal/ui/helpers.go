package ui

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
)

type Job struct {
    ID     string `json:"id"`
    Status string `json:"status"`
}

// FetchActiveJobs fetches jobs with "running" or "pending" status from the server
func FetchActiveJobs(serverURL string) ([]string, error) {
    base := strings.TrimRight(serverURL, "/")
    url := base + "/jobs?status=running,pending"

    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("fetch active jobs failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 300 {
        body, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("fetch failed (%s): %s", resp.Status, strings.TrimSpace(string(body)))
    }

    var jobs []Job
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    if err := json.Unmarshal(body, &jobs); err != nil {
        return nil, fmt.Errorf("parse response: %w", err)
    }

    jobIDs := make([]string, len(jobs))
    for i, job := range jobs {
        jobIDs[i] = job.ID
    }

    return jobIDs, nil
}