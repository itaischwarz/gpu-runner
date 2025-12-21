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
	rootCmd.PersistentFlags().StringVar(&server, "server", "http://localhost:8080", "GPU runner server URL")
}
