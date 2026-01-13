package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit a GPU job",
	RunE: func(cmd *cobra.Command, args []string) error {
		command, _ := cmd.Flags().GetString("cmd")
		storage, _ := cmd.Flags().GetString("storage")
		storageInt := 0
		var err error = nil
		if len(storage) != 0 {
			storageInt, err = strconv.Atoi(storage)
			if err != nil {
					return fmt.Errorf("invalid storage value '%s': must be an integer", storage)
			}
		} 

		maxRetriesStr, _ := cmd.Flags().GetString("maxRetries")
		maxRetries := 0
		if maxRetriesStr != "" {
			maxRetries, err = strconv.Atoi(maxRetriesStr)
			if err != nil {
				return fmt.Errorf("invalid maxRetries value '%s': must be an integer", maxRetriesStr)
			}
		}

		body := map[string]any{"command": command, "storage": storageInt, "max_retries": maxRetries}
		
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}

		base := strings.TrimRight(server, "/")
		resp, err := http.Post(base+"/jobs", "application/json", bytes.NewBuffer(data))
		if err != nil {
			return fmt.Errorf("submit request failed: %w", err)
		}
		defer resp.Body.Close()

		payload, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return fmt.Errorf("submit failed (%s): %s", resp.Status, strings.TrimSpace(string(payload)))
		}

		var job struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(payload, &job); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		fmt.Printf("Job submitted: %s (status: %s)\n", job.ID, job.Status)
		return nil
	},
}

func init() {
	submitCmd.Flags().String("cmd", "", "Command to run")
	submitCmd.MarkFlagRequired("cmd")
	submitCmd.Flags().String("storage", "", "Storage for Job")
	submitCmd.Flags().String("maxRetries", "", "Attempts running a job")

	rootCmd.AddCommand(submitCmd)
}
