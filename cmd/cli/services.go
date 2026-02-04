package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/noahjeana/k8s-exposer/pkg/client"
	"github.com/spf13/cobra"
)

var servicesCmd = &cobra.Command{
	Use:   "services",
	Short: "Manage exposed services",
	Long:  "List and inspect exposed Kubernetes services",
	RunE:  runServicesList, // Default action: list services
}

var servicesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all exposed services",
	RunE:  runServicesList,
}

var servicesGetCmd = &cobra.Command{
	Use:   "get <name>",
	Short: "Get details for a specific service",
	Args:  cobra.ExactArgs(1),
	RunE:  runServicesGet,
}

func init() {
	rootCmd.AddCommand(servicesCmd)
	servicesCmd.AddCommand(servicesListCmd)
	servicesCmd.AddCommand(servicesGetCmd)
}

func runServicesList(cmd *cobra.Command, args []string) error {
	c := client.NewClient(serverURL)
	services, err := c.ListServices()
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	if jsonOutput {
		return printJSON(services)
	}

	if len(services) == 0 {
		color.Yellow("No services found")
		return nil
	}

	// Print table
	cyan := color.New(color.FgCyan, color.Bold).SprintFunc()
	fmt.Printf("%s\n", cyan("NAME         NAMESPACE    SUBDOMAIN         TARGET IP      PORTS"))
	fmt.Println("─────────────────────────────────────────────────────────────────────────")

	for _, svc := range services {
		ports := ""
		for i, p := range svc.Ports {
			if i > 0 {
				ports += ", "
			}
			ports += fmt.Sprintf("%d→%d/%s", p.Port, p.TargetPort, p.Protocol)
		}
		
		fmt.Printf("%-12s %-12s %-17s %-14s %s\n",
			svc.Name,
			svc.Namespace,
			svc.Subdomain,
			svc.TargetIP,
			ports,
		)
	}

	fmt.Printf("\nTotal: %d services\n", len(services))

	return nil
}

func runServicesGet(cmd *cobra.Command, args []string) error {
	c := client.NewClient(serverURL)
	service, err := c.GetService(args[0])
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	if jsonOutput {
		return printJSON(service)
	}

	// Print formatted output
	green := color.New(color.FgGreen, color.Bold).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("%s: %s\n", cyan("Name"), green(service.Name))
	fmt.Printf("%s: %s\n", cyan("Namespace"), service.Namespace)
	fmt.Printf("%s: %s\n", cyan("Subdomain"), service.Subdomain)
	if service.FQDN != "" {
		fmt.Printf("%s: %s\n", cyan("FQDN"), green(service.FQDN))
	}
	fmt.Printf("%s: %s\n", cyan("Target IP"), service.TargetIP)
	
	fmt.Printf("\n%s:\n", cyan("Ports"))
	for _, p := range service.Ports {
		fmt.Printf("  • %d → %d (%s)\n", p.Port, p.TargetPort, p.Protocol)
	}

	return nil
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
