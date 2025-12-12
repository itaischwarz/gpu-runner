package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
    Use:   "submit",
    Short: "Submit a GPU job",
    Run: func(cmd *cobra.Command, args []string) {
        command, _ := cmd.Flags().GetString("cmd")
        storage, _ := cmd.Flags().GetString("storage")
        body := map[string]string{"command": command, "storage": storage}
        fmt.Println("Body at start", body)
        data, _ := json.Marshal(body)
        resp, err := http.Post("http://localhost:8080/jobs", "application/json", bytes.NewBuffer(data))

        if err != nil {
            fmt.Println("Error: ", err)
            return
        }

        bodyBytes, _ := io.ReadAll(resp.Body)

        fmt.Println("RAW RESPONSE BODY:\n", string(bodyBytes))

        var job map[string]interface{}

        err = json.Unmarshal(bodyBytes, &job)
    
        resp.Body.Close()

        if err != nil{
            fmt.Println("Unable to extract body contents", err)
        }

        // fmt.Println("Job submit",job)
        // fmt.Println("job submitted:", job["id"])
    },
}

func init() {
    submitCmd.Flags().String("cmd", "", "Command to run")
    submitCmd.MarkFlagRequired("cmd")
    submitCmd.Flags().String("storage", "", "Storage for Job")
    rootCmd.AddCommand(submitCmd)
}
