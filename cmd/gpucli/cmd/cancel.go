package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"encoding/json"
	"github.com/spf13/cobra"
)

var cancelCmd = &cobra.Command{
    Use:   "cancel",
    Short: "Cancel a GPU job",
    Run: func(cmd *cobra.Command, args []string) {
				jobID, _ := cmd.Flags().GetString("id")
				reason, _ := cmd.Flags().GetString("reason")
				body := map[string]string{"id": jobID, "reason": reason}
				data, _ := json.Marshal(body)
        _, err := http.Post(fmt.Sprintf("http://localhost:8080/endjobs/%s", jobID), "application/json", bytes.NewBuffer(data))
        if err != nil {
            fmt.Println("Unable to Print Job: ", err)
            return
        }

    },
}

func init() {
    submitCmd.Flags().String("id", "", "Job to Cancel")
    submitCmd.MarkFlagRequired("id")
    submitCmd.Flags().String("reason", "", "Reason for Cancellation")
    rootCmd.AddCommand(cancelCmd)
}
