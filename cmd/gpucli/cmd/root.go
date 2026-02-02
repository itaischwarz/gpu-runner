package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var server string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gpucli",
	Short: "CLI to submit, check, and cancel GPU jobs",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Get default from environment variable, fallback to localhost
	defaultServer := os.Getenv("GPU_RUNNER_SERVER")
	if defaultServer == "" {
		defaultServer = "http://0.0.0.0:8080"
	}
	rootCmd.PersistentFlags().StringVar(&server, "server", defaultServer, "GPU runner server URL (env: GPU_RUNNER_SERVER)")
}
