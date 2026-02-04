package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/noahjeana/k8s-exposer/pkg/client"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status",
	Long:  "Display k8s-exposer system status and health",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	c := client.NewClient(serverURL)
	
	health, err := c.GetHealth()
	if err != nil {
		return fmt.Errorf("failed to get health: %w", err)
	}

	metrics, err := c.GetMetrics()
	if err != nil {
		return fmt.Errorf("failed to get metrics: %w", err)
	}

	if jsonOutput {
		data := map[string]interface{}{
			"health":  health,
			"metrics": metrics,
		}
		return printJSON(data)
	}

	// Print formatted status
	green := color.New(color.FgGreen, color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(cyan("=== k8s-exposer Status ==="))
	fmt.Println()

	// Health
	statusColor := green
	if health.Status != "healthy" {
		statusColor = color.New(color.FgRed, color.Bold).SprintFunc()
	}
	fmt.Printf("Status: %s\n", statusColor(health.Status))
	fmt.Printf("Version: %s\n", health.Version)
	fmt.Printf("Services: %d\n", health.ServiceCount)
	fmt.Println()

	// Metrics
	fmt.Println(cyan("=== Metrics ==="))
	
	if total, ok := metrics.Services["total"].(float64); ok {
		fmt.Printf("Total Services: %s\n", yellow(fmt.Sprintf("%.0f", total)))
	}
	if ports, ok := metrics.Services["total_ports"].(float64); ok {
		fmt.Printf("Total Ports: %s\n", yellow(fmt.Sprintf("%.0f", ports)))
	}
	fmt.Println()

	if alloc, ok := metrics.Memory["alloc_mb"].(float64); ok {
		fmt.Println(cyan("Memory:"))
		fmt.Printf("  Allocated: %.1f MB\n", alloc)
		if sys, ok := metrics.Memory["sys_mb"].(float64); ok {
			fmt.Printf("  System: %.1f MB\n", sys)
		}
		if gc, ok := metrics.Memory["num_gc"].(float64); ok {
			fmt.Printf("  GC runs: %.0f\n", gc)
		}
	}
	fmt.Println()

	if goroutines, ok := metrics.Runtime["goroutines"].(float64); ok {
		fmt.Println(cyan("Runtime:"))
		fmt.Printf("  Goroutines: %.0f\n", goroutines)
		if goVersion, ok := metrics.Runtime["go_version"].(string); ok {
			fmt.Printf("  Go Version: %s\n", goVersion)
		}
	}

	return nil
}
