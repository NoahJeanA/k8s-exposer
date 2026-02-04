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
		exposedSvc, err := extractServiceInfo(clientset, &svc)
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
func extractServiceInfo(clientset kubernetes.Interface, svc *corev1.Service) (*types.ExposedService, error) {
	// Check if service has required annotations
	subdomain, hasSubdomain := svc.Annotations[SubdomainAnnotation]
	portsAnnotation, hasPorts := svc.Annotations[PortsAnnotation]

	if !hasSubdomain || !hasPorts {
		return nil, nil // Not an exposed service
	}

	// Parse ports annotation
	requestedPorts, err := parsePorts(portsAnnotation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ports annotation: %w", err)
	}

	// Get endpoints to find pod IPs (pod IPs are routable over WireGuard, ClusterIPs are not)
	endpoints, err := clientset.CoreV1().Endpoints(svc.Namespace).Get(context.Background(), svc.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	// Get first ready pod IP from endpoints
	var podIP string
	if len(endpoints.Subsets) > 0 && len(endpoints.Subsets[0].Addresses) > 0 {
		podIP = endpoints.Subsets[0].Addresses[0].IP
	}
	
	if podIP == "" {
		return nil, fmt.Errorf("no ready pods found for service")
	}
	
	var ports []types.PortMapping
	
	// Map requested external ports to endpoint ports
	for _, requestedPort := range requestedPorts {
		// Use the first endpoint port as the target (most services have only one port)
		if len(endpoints.Subsets) > 0 && len(endpoints.Subsets[0].Ports) > 0 {
			endpointPort := endpoints.Subsets[0].Ports[0].Port
			
			ports = append(ports, types.PortMapping{
				Port:       requestedPort.Port, // External port (e.g., 8080)
				TargetPort: endpointPort,        // Pod port from endpoint (e.g., 80)
				Protocol:   requestedPort.Protocol,
			})
			break // Only process first requested port for now
		}
	}

	if len(ports) == 0 {
		return nil, fmt.Errorf("no valid ports found for service")
	}

	exposedSvc := &types.ExposedService{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		Subdomain: subdomain,
		Ports:     ports,
		TargetIP:  podIP, // Use pod IP for direct routing over WireGuard
		NodeIP:    podIP,
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
