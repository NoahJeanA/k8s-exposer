package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/noahjeana/k8s-exposer/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	SubdomainAnnotation = "expose.neverup.at/subdomain"
	PortsAnnotation     = "expose.neverup.at/ports"
)

// DiscoverServices discovers all services with exposure annotations
func DiscoverServices(ctx context.Context, clientset kubernetes.Interface, logger *slog.Logger) ([]types.ExposedService, error) {
	// List all services across all namespaces
	serviceList, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var exposedServices []types.ExposedService
	for _, svc := range serviceList.Items {
		exposedSvc, err := extractServiceInfo(&svc)
		if err != nil {
			// Skip services without annotations or with invalid configuration
			logger.Debug("Skipping service", "name", svc.Name, "namespace", svc.Namespace, "error", err)
			continue
		}
		if exposedSvc != nil {
			exposedServices = append(exposedServices, *exposedSvc)
		}
	}

	logger.Info("Discovered exposed services", "count", len(exposedServices))
	return exposedServices, nil
}

// extractServiceInfo extracts exposed service information from a Kubernetes service
func extractServiceInfo(svc *corev1.Service) (*types.ExposedService, error) {
	// Check if service has required annotations
	subdomain, hasSubdomain := svc.Annotations[SubdomainAnnotation]
	portsAnnotation, hasPorts := svc.Annotations[PortsAnnotation]

	if !hasSubdomain || !hasPorts {
		return nil, nil // Not an exposed service
	}

	// Parse ports annotation
	ports, err := parsePorts(portsAnnotation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ports annotation: %w", err)
	}

	// Get target IP (ClusterIP)
	targetIP := svc.Spec.ClusterIP
	if targetIP == "" || targetIP == "None" {
		return nil, fmt.Errorf("service has no ClusterIP")
	}

	// Get NodeIP if available (for NodePort services)
	nodeIP := ""
	// NodeIP would be discovered from the node, but for simplicity we'll leave it empty
	// The server can use the ClusterIP over Wireguard

	exposedSvc := &types.ExposedService{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Subdomain: subdomain,
		Ports:     ports,
		TargetIP:  targetIP,
		NodeIP:    nodeIP,
	}

	// Validate the service
	if err := exposedSvc.Validate(); err != nil {
		return nil, fmt.Errorf("service validation failed: %w", err)
	}

	return exposedSvc, nil
}

// parsePorts parses the ports annotation (format: "25565/tcp,25565/udp,80/tcp")
func parsePorts(portsAnnotation string) ([]types.PortMapping, error) {
	if portsAnnotation == "" {
		return nil, fmt.Errorf("ports annotation is empty")
	}

	portStrings := strings.Split(portsAnnotation, ",")
	var ports []types.PortMapping

	for _, portStr := range portStrings {
		portStr = strings.TrimSpace(portStr)
		if portStr == "" {
			continue
		}

		// Split by '/' to get port and protocol
		parts := strings.Split(portStr, "/")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid port format: %q (expected format: port/protocol)", portStr)
		}

		// Parse port number
		portNum, err := strconv.ParseInt(parts[0], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid port number: %q", parts[0])
		}

		protocol := strings.ToLower(parts[1])

		port := types.PortMapping{
			Port:     int32(portNum),
			Protocol: protocol,
		}

		// Validate port mapping
		if err := port.Validate(); err != nil {
			return nil, fmt.Errorf("invalid port mapping %q: %w", portStr, err)
		}

		ports = append(ports, port)
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no valid ports found")
	}

	return ports, nil
}
