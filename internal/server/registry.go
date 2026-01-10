package server

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/noahjeana/k8s-exposer/pkg/types"
)

// ServiceRegistry maintains a registry of exposed services and their listeners
type ServiceRegistry struct {
	services       map[string]*types.ExposedService // subdomain -> service
	listeners      map[string]*PortListener         // "port:protocol" -> listener
	allocatedPorts map[string]bool                  // "port:protocol" -> allocated
	portRangeStart int32
	portRangeEnd   int32
	mu             sync.RWMutex
	logger         *slog.Logger
	forwarder      *Forwarder
}

// NewServiceRegistry creates a new service registry
func NewServiceRegistry(portRangeStart, portRangeEnd int32, forwarder *Forwarder, logger *slog.Logger) *ServiceRegistry {
	return &ServiceRegistry{
		services:       make(map[string]*types.ExposedService),
		listeners:      make(map[string]*PortListener),
		allocatedPorts: make(map[string]bool),
		portRangeStart: portRangeStart,
		portRangeEnd:   portRangeEnd,
		logger:         logger,
		forwarder:      forwarder,
	}
}

// Update updates the registry with new service configurations
func (r *ServiceRegistry) Update(services []types.ExposedService) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Updating service registry", "count", len(services))

	// Build a map of new services
	newServices := make(map[string]*types.ExposedService)
	for i := range services {
		svc := &services[i]
		newServices[svc.Subdomain] = svc
	}

	// Stop and remove listeners for services that no longer exist
	for subdomain, oldSvc := range r.services {
		if _, exists := newServices[subdomain]; !exists {
			r.logger.Info("Removing service", "subdomain", subdomain)
			r.removeServiceLocked(subdomain)
		} else {
			// Check if service configuration changed
			newSvc := newServices[subdomain]
			if !r.servicesEqual(oldSvc, newSvc) {
				r.logger.Info("Service configuration changed", "subdomain", subdomain)
				r.removeServiceLocked(subdomain)
			}
		}
	}

	// Add or update services
	for subdomain, svc := range newServices {
		if _, exists := r.services[subdomain]; !exists {
			r.logger.Info("Adding new service", "subdomain", subdomain)
			if err := r.addServiceLocked(svc); err != nil {
				r.logger.Error("Failed to add service", "subdomain", subdomain, "error", err)
				continue
			}
		}
	}

	r.logger.Info("Service registry updated", "active_services", len(r.services))
	return nil
}

// addServiceLocked adds a service and starts listeners (must be called with lock held)
func (r *ServiceRegistry) addServiceLocked(svc *types.ExposedService) error {
	// Add to registry
	r.services[svc.Subdomain] = svc

	// Start listeners for each port
	for _, portMapping := range svc.Ports {
		// Try to allocate the requested port
		allocatedPort, err := r.allocatePortLocked(portMapping.Port, portMapping.Protocol)
		if err != nil {
			r.logger.Error("Failed to allocate port", "port", portMapping.Port, "protocol", portMapping.Protocol, "error", err)
			continue
		}

		// Start listener
		listener := NewPortListener(allocatedPort, portMapping.Protocol, *svc, r.forwarder, r.logger)
		if err := listener.Start(); err != nil {
			r.logger.Error("Failed to start listener", "port", allocatedPort, "protocol", portMapping.Protocol, "error", err)
			r.deallocatePortLocked(allocatedPort, portMapping.Protocol)
			continue
		}

		listenerKey := r.portKey(allocatedPort, portMapping.Protocol)
		r.listeners[listenerKey] = listener

		r.logger.Info("Listener started",
			"subdomain", svc.Subdomain,
			"port", allocatedPort,
			"protocol", portMapping.Protocol,
			"target", fmt.Sprintf("%s:%d", svc.TargetIP, portMapping.Port))
	}

	return nil
}

// removeServiceLocked removes a service and stops its listeners (must be called with lock held)
func (r *ServiceRegistry) removeServiceLocked(subdomain string) {
	svc, exists := r.services[subdomain]
	if !exists {
		return
	}

	// Stop all listeners for this service
	for _, portMapping := range svc.Ports {
		listenerKey := r.portKey(portMapping.Port, portMapping.Protocol)
		if listener, exists := r.listeners[listenerKey]; exists {
			listener.Stop()
			delete(r.listeners, listenerKey)
			r.deallocatePortLocked(portMapping.Port, portMapping.Protocol)
		}
	}

	delete(r.services, subdomain)
}

// RemoveService removes a service from the registry
func (r *ServiceRegistry) RemoveService(subdomain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.removeServiceLocked(subdomain)
	return nil
}

// allocatePortLocked allocates a port for a protocol (must be called with lock held)
func (r *ServiceRegistry) allocatePortLocked(port int32, protocol string) (int32, error) {
	// Try requested port first
	if r.isPortAvailableLocked(port, protocol) {
		key := r.portKey(port, protocol)
		r.allocatedPorts[key] = true
		return port, nil
	}

	// Port conflict - allocate from high range
	for p := r.portRangeStart; p <= r.portRangeEnd; p++ {
		if r.isPortAvailableLocked(p, protocol) {
			key := r.portKey(p, protocol)
			r.allocatedPorts[key] = true
			r.logger.Warn("Port conflict, allocated alternative", "requested", port, "allocated", p, "protocol", protocol)
			return p, nil
		}
	}

	return 0, fmt.Errorf("no available ports in range %d-%d", r.portRangeStart, r.portRangeEnd)
}

// AllocatePort allocates a port for a protocol
func (r *ServiceRegistry) AllocatePort(port int32, protocol string) (int32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.allocatePortLocked(port, protocol)
}

// deallocatePortLocked deallocates a port (must be called with lock held)
func (r *ServiceRegistry) deallocatePortLocked(port int32, protocol string) {
	key := r.portKey(port, protocol)
	delete(r.allocatedPorts, key)
}

// isPortAvailableLocked checks if a port is available (must be called with lock held)
func (r *ServiceRegistry) isPortAvailableLocked(port int32, protocol string) bool {
	key := r.portKey(port, protocol)
	return !r.allocatedPorts[key]
}

// IsPortAvailable checks if a port is available for a protocol
func (r *ServiceRegistry) IsPortAvailable(port int32, protocol string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isPortAvailableLocked(port, protocol)
}

// GetService retrieves a service by subdomain
func (r *ServiceRegistry) GetService(subdomain string) (*types.ExposedService, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	svc, exists := r.services[subdomain]
	return svc, exists
}

// portKey creates a unique key for port and protocol
func (r *ServiceRegistry) portKey(port int32, protocol string) string {
	return fmt.Sprintf("%d:%s", port, protocol)
}

// servicesEqual checks if two services have the same configuration
func (r *ServiceRegistry) servicesEqual(a, b *types.ExposedService) bool {
	if a.Name != b.Name || a.Namespace != b.Namespace || a.Subdomain != b.Subdomain || a.TargetIP != b.TargetIP {
		return false
	}
	if len(a.Ports) != len(b.Ports) {
		return false
	}
	for i := range a.Ports {
		if a.Ports[i].Port != b.Ports[i].Port || a.Ports[i].Protocol != b.Ports[i].Protocol {
			return false
		}
	}
	return true
}

// Close stops all listeners and clears the registry
func (r *ServiceRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Info("Closing service registry")

	for _, listener := range r.listeners {
		listener.Stop()
	}

	r.services = make(map[string]*types.ExposedService)
	r.listeners = make(map[string]*PortListener)
	r.allocatedPorts = make(map[string]bool)
}
