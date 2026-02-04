package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/noahjeana/k8s-exposer/pkg/client"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Force reconciliation",
	Long:  "Trigger immediate reconciliation of HAProxy and firewall rules",
	RunE:  runSync,
}

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Show system metrics",
	Long:  "Display detailed system metrics",
	RunE:  runMetrics,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run:   runVersion,
}

func init() {
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(metricsCmd)
	rootCmd.AddCommand(versionCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	c := client.NewClient(serverURL)
	
	if err := c.Sync(); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold).SprintFunc()
	fmt.Printf("%s Reconciliation triggered successfully\n", green("✓"))

	return nil
}

func runMetrics(cmd *cobra.Command, args []string) error {
	c := client.NewClient(serverURL)
	
	metrics, err := c.GetMetrics()
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	if jsonOutput {
		return printJSON(metrics)
	}

	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	
	fmt.Println(cyan("=== System Metrics ==="))
	fmt.Println()
	
	// Services
	if services, ok := metrics.Services["total"]; ok {
		if total, ok := services.(float64); ok {
			fmt.Println(cyan("Services:"))
			fmt.Printf("  Total: %.0f\n", total)
		}
		if ports, ok := metrics.Services["total_ports"].(float64); ok {
			fmt.Printf("  Total Ports: %.0f\n", ports)
		}
		fmt.Println()
	}

	// Memory
	if alloc, ok := metrics.Memory["alloc_mb"].(float64); ok {
		fmt.Println(cyan("Memory:"))
		fmt.Printf("  Allocated: %.1f MB\n", alloc)
		if totalAlloc, ok := metrics.Memory["total_alloc_mb"].(float64); ok {
			fmt.Printf("  Total Allocated: %.1f MB\n", totalAlloc)
		}
		if sys, ok := metrics.Memory["sys_mb"].(float64); ok {
			fmt.Printf("  System: %.1f MB\n", sys)
		}
		if gc, ok := metrics.Memory["num_gc"].(float64); ok {
			fmt.Printf("  GC Runs: %.0f\n", gc)
		}
		fmt.Println()
	}

	// Runtime
	if goroutines, ok := metrics.Runtime["goroutines"].(float64); ok {
		fmt.Println(cyan("Runtime:"))
		fmt.Printf("  Goroutines: %.0f\n", goroutines)
		if goVersion, ok := metrics.Runtime["go_version"].(string); ok {
			fmt.Printf("  Go Version: %s\n", goVersion)
		}
	}

	return nil
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("k8s-exposer CLI\n")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Commit: %s\n", commit)
	fmt.Printf("Built: %s\n", date)
}
