package cmd

import (
	"fmt"
	"net/http"
	"os"
	"github.com/spf13/cobra"
	"io"
)

var shutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Gracefully Shutdown GPU Runner Server",
	RunE: func(cmd *cobra.Command, args []string) error {
		token := os.Getenv("SHUTDOWN_TOKEN")
		base := "http://0.0.0.0:8080"
		fmt.Println("server", server)
		req, err := http.NewRequest("DELETE", base+"/server", nil)
		if err != nil {
    	fmt.Print("Error", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)

		if err != nil{
			return fmt.Errorf("Unable to shutdown server: %s", err)
		}
		body, err := io.ReadAll(resp.Body)
		fmt.Println(string(body))
		return nil


	},
}

func init() {
	shutdownCmd.Flags().String("reason", "", "Reason for Shutdown")
	shutdownCmd.MarkFlagRequired("reason")

	rootCmd.AddCommand(shutdownCmd)
} 
