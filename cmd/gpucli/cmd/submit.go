package cmd

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "github.com/spf13/cobra"
)

var submitCmd = &cobra.Command{
    Use:   "submit",
    Short: "Submit a GPU job",
    Run: func(cmd *cobra.Command, args []string) {
        command, _ := cmd.Flags().GetString("cmd")

        body := map[string]string{"command": command}
        data, _ := json.Marshal(body)

        resp, _ := http.Post("http://localhost:8080/jobs", "application/json", bytes.NewBuffer(data))

        var job map[string]interface{}
        json.NewDecoder(resp.Body).Decode(&job)

        fmt.Println("Job submitted:", job["id"])
    },
}

func init() {
    submitCmd.Flags().String("cmd", "", "Command to run")
    submitCmd.MarkFlagRequired("cmd")
    rootCmd.AddCommand(submitCmd)
}
