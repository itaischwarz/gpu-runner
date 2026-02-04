package cmd

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "text/tabwriter"

    "github.com/spf13/cobra"
)

var statusFilter string

// listJobs contains the actual logic for listing jobs
func listJobs(statusFilter string) error {
    base := strings.TrimRight(server, "/")
    url := base + "/jobs"
    if statusFilter != "" {
        url += "?status=" + statusFilter
    }

    resp, err := http.Get(url)
    if err != nil {
        return fmt.Errorf("list request failed: %w", err)
    }
    defer resp.Body.Close()

    payload, _ := io.ReadAll(resp.Body)
    if resp.StatusCode >= 300 {
        return fmt.Errorf("list failed (%s): %s", resp.Status, strings.TrimSpace(string(payload)))
    }

    var jobs []struct {
        ID         string `json:"id"`
        Command    string `json:"command"`
        Status     string `json:"status"`
        CreatedAt  string `json:"created_at"`
        FinishedAt string `json:"finished_at"`
    }

    if err := json.Unmarshal(payload, &jobs); err != nil {
        return fmt.Errorf("parse response: %w", err)
    }

    if len(jobs) == 0 {
        fmt.Println("No jobs found.")
        return nil
    }

    w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
    fmt.Fprintln(w, "ID\tSTATUS\tCOMMAND\tCREATED\tFINISHED")
    for _, j := range jobs {
        command := j.Command
        if len(command) > 40 {
            command = command[:37] + "..."
        }
        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", j.ID, j.Status, command, j.CreatedAt, j.FinishedAt)
    }
    w.Flush()
    return nil
}

var listCmd = &cobra.Command{
    Use:   "list",
    Short: "List all jobs",
    Long:  "List all jobs on the server. Optionally filter by status (pending, running, success, failed, cancelled).",
    RunE: func(cmd *cobra.Command, args []string) error {
        return listJobs(statusFilter)
    },
}

func init() {
    listCmd.Flags().StringVar(&statusFilter, "status", "", "Filter by job status (pending, running, success, failed, cancelled)")
    rootCmd.AddCommand(listCmd)
}
