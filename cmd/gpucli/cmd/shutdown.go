package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var shutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Gracefully Shutdown GPU Runner Server",
	RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv("SHUTDOWN_TOKEN")
		req, _ := http.NewRequest("DELETE", server+"/server", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if _, err := http.DefaultClient.Do(req); err != nil{
			return fmt.Errorf("Unable to shutdown server: %s", err)
		}
		return nil


	},
}

func init() {
	submitCmd.Flags().String("reason", "", "Reason for Shutdown")
	shutdownCmd.MarkFlagRequired("reason")
} 
