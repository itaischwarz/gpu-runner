package cmd

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "sort"
    "strings"
    "time"

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

    fmt.Printf("Job: %s\nCommand: %s\nStatus: %s\nLogs:\n%s\n", job.ID, job.Command, job.Status, formatLogs(job.Log))
    return nil
}

// formatLogs turns the API's JSON log entries into one readable line each:
// "15:04:05 INFO  message key=value ...". It falls back to the raw text if the
// logs aren't in the expected format.
func formatLogs(raw string) string {
    var entries []struct {
        Level     string         `json:"level"`
        Message   string         `json:"message"`
        Timestamp time.Time      `json:"timestamp"`
        Fields    map[string]any `json:"fields"`
    }
    if err := json.Unmarshal([]byte(raw), &entries); err != nil {
        return raw
    }
    if len(entries) == 0 {
        return "  (no logs yet)"
    }

    var b strings.Builder
    for _, e := range entries {
        fmt.Fprintf(&b, "  %s %-5s %s", e.Timestamp.Local().Format("15:04:05"), strings.ToUpper(e.Level), e.Message)
        keys := make([]string, 0, len(e.Fields))
        for k := range e.Fields {
            if k != "job_id" { // already shown above
                keys = append(keys, k)
            }
        }
        sort.Strings(keys)
        for _, k := range keys {
            fmt.Fprintf(&b, " %s=%v", k, e.Fields[k])
        }
        b.WriteString("\n")
    }
    return strings.TrimRight(b.String(), "\n")
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