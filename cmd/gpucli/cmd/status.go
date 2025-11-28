package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
    Use:   "status [jobID]",
    Short: "Check job status",
    Args:  cobra.ExactArgs(1),
    Run: func(cmd *cobra.Command, args []string) {
        jobID := args[0]

        resp, _ := http.Get("http://localhost:8080/jobs/" + jobID)

        if resp.StatusCode != 200 {
            fmt.Println("Job not found")
            return
        }

        bodyBytes, err := json.Marshal(resp.Body)
        if err != nil{
            fmt.Println("Unable to Extract Respones Status Body")
        }


        fmt.Println("RAW RESPONSE BODY:\n", string(bodyBytes))

        var job map[string]interface{}
        if err := json.Unmarshal(bodyBytes, &job); err != nil{
            fmt.Println("Unable to Unmarshall JSON Status Body data ")
        }

        json.NewDecoder(resp.Body).Decode(&job)

        fmt.Printf("Status: %s\nLogs:\n%s\n", job["status"], job["log"])
    },
}

func init() {
    rootCmd.AddCommand(statusCmd)
}
