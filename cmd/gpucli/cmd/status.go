package cmd

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/spf13/cobra"
)

// checkJobStatus contains the actual logic for checking job status
func checkJobStatus(jobID string) error {
    base := strings.TrimRight(server, "/")
    resp, err := http.Get(base + "/jobs/" + jobID)

    if err != nil {
        fmt.Print("Failed sending to this url", base+"/jobs/"+jobID)
        return fmt.Errorf("status request failed: %w", err)
    }
    defer resp.Body.Close()

    payload, _ := io.ReadAll(resp.Body)
    if resp.StatusCode >= 300 {
        return fmt.Errorf("status failed (%s): %s", resp.Status, strings.TrimSpace(string(payload)))
    }

    var job struct {
        ID      string `json:"id"`
        Status  string `json:"status"`
        Log     string `json:"log"`
        Command string `json:"command"`
    }

    if err := json.Unmarshal(payload, &job); err != nil {
        return fmt.Errorf("parse response: %w", err)
    }

    fmt.Printf("Job: %s\nCommand: %s\nStatus: %s\nLogs:\n%s\n", job.ID, job.Command, job.Status, job.Log)
    return nil
}

var statusCmd = &cobra.Command{
    Use:   "status [jobID]",
    Short: "Check job status",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return checkJobStatus(args[0])
    },
}

func init() {
    rootCmd.AddCommand(statusCmd)
}