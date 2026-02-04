package cmd

import (
    "fmt"
    "io"
    "net/http"
    "os"

    "github.com/spf13/cobra"
)

// shutdownServer contains the actual logic for shutting down the server
func shutdownServer(reason string) error {
    token := os.Getenv("SHUTDOWN_TOKEN")
    base := "http://0.0.0.0:8080"
    fmt.Println("server", server)
    
    req, err := http.NewRequest("DELETE", base+"/server", nil)
    if err != nil {
        return fmt.Errorf("create request: %w", err)
    }
    
    req.Header.Set("Authorization", "Bearer "+token)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("unable to shutdown server: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("read response: %w", err)
    }
    
    fmt.Println(string(body))
    return nil
}

var shutdownCmd = &cobra.Command{
    Use:   "shutdown",
    Short: "Gracefully Shutdown GPU Runner Server",
    RunE: func(cmd *cobra.Command, args []string) error {
        reason, _ := cmd.Flags().GetString("reason")
        return shutdownServer(reason)
    },
}

func init() {
    shutdownCmd.Flags().String("reason", "", "Reason for Shutdown")
    shutdownCmd.MarkFlagRequired("reason")
    rootCmd.AddCommand(shutdownCmd)
}