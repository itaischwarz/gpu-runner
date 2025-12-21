package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

var cancelCmd = &cobra.Command{
	Use:   "cancel",
	Short: "Cancel a GPU job",
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, _ := cmd.Flags().GetString("id")
		reason, _ := cmd.Flags().GetString("reason")
		body := map[string]string{"id": jobID, "reason": reason}
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}

		base := strings.TrimRight(server, "/")
		resp, err := http.Post(fmt.Sprintf("%s/endjobs/%s", base, jobID), "application/json", bytes.NewBuffer(data))
		if err != nil {
			return fmt.Errorf("cancel request failed: %w", err)
		}
		defer resp.Body.Close()

		payload, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return fmt.Errorf("cancel failed (%s): %s", resp.Status, strings.TrimSpace(string(payload)))
		}

		var job struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(payload, &job); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}

		fmt.Printf("Cancelled job: %s (status: %s)\n", job.ID, job.Status)
		return nil
	},
}

func init() {
	cancelCmd.Flags().String("id", "", "Job to Cancel")
	cancelCmd.MarkFlagRequired("id")
	cancelCmd.Flags().String("reason", "", "Reason for Cancellation")
	rootCmd.AddCommand(cancelCmd)
}
