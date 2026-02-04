package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	serverURL string
	jsonOutput bool
	
	// Version info
	version = "1.0.0"
	commit  = "dev"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "k8s-exposer",
	Short: "CLI for k8s-exposer management",
	Long: `k8s-exposer CLI - Manage and monitor your k8s-exposer deployment.

This CLI provides commands to:
  - List and inspect exposed services
  - View system status and metrics
  - Trigger reconciliation
  - Debug connectivity issues

Examples:
  k8s-exposer services           # List all services
  k8s-exposer status             # Show system status
  k8s-exposer sync               # Force reconciliation
  k8s-exposer services get app   # Get service details`,
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8090", "k8s-exposer server URL")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
