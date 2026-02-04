package api

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
)

// handleHealth returns system health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	services := s.registry.GetServices()

	response := map[string]interface{}{
		"status":        "healthy",
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
		"service_count": len(services),
		"version":       "1.0.0",
	}

	s.respondJSON(w, http.StatusOK, response)
}

// handleMetrics returns basic system metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	services := s.registry.GetServices()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Count total ports
	totalPorts := 0
	for _, svc := range services {
		totalPorts += len(svc.Ports)
	}

	metrics := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"services": map[string]interface{}{
			"total":       len(services),
			"total_ports": totalPorts,
		},
		"memory": map[string]interface{}{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
		"runtime": map[string]interface{}{
			"goroutines": runtime.NumGoroutine(),
			"go_version": runtime.Version(),
		},
	}

	s.respondJSON(w, http.StatusOK, metrics)
}

// handleListServices returns all services
func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	services := s.registry.GetServices()

	// Convert to response format
	serviceList := make([]map[string]interface{}, 0, len(services))
	for _, svc := range services {
		serviceList = append(serviceList, map[string]interface{}{
			"name":      svc.Name,
			"namespace": svc.Namespace,
			"subdomain": svc.Subdomain,
			"target_ip": svc.TargetIP,
			"ports":     svc.Ports,
		})
	}

	response := map[string]interface{}{
		"services": serviceList,
		"count":    len(serviceList),
	}

	s.respondJSON(w, http.StatusOK, response)
}

// handleGetService returns details for a specific service
func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		s.respondError(w, http.StatusBadRequest, "service name required")
		return
	}

	// Find service by name (search all namespaces)
	services := s.registry.GetServices()
	var found *map[string]interface{}

	for _, svc := range services {
		if svc.Name == name {
			serviceData := map[string]interface{}{
				"name":      svc.Name,
				"namespace": svc.Namespace,
				"subdomain": svc.Subdomain,
				"target_ip": svc.TargetIP,
				"node_ip":   svc.NodeIP,
				"ports":     svc.Ports,
				"fqdn":      fmt.Sprintf("%s.neverup.at", svc.Subdomain), // TODO: Get domain from config
			}
			found = &serviceData
			break
		}
	}

	if found == nil {
		s.respondError(w, http.StatusNotFound, "service not found")
		return
	}

	s.respondJSON(w, http.StatusOK, *found)
}

// handleSync forces a reconciliation
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	if s.automation == nil {
		s.respondError(w, http.StatusServiceUnavailable, "automation not available")
		return
	}

	// Trigger sync by getting current services and calling reconcile
	services := s.registry.GetServices()
	if err := s.automation.Reconcile(services); err != nil {
		s.logger.Error("Manual reconciliation failed", "error", err)
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("reconciliation failed: %v", err))
		return
	}

	response := map[string]interface{}{
		"status":        "success",
		"message":       "reconciliation triggered",
		"service_count": len(services),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	s.respondJSON(w, http.StatusOK, response)
}

// handleHAProxyStatus returns HAProxy status
func (s *Server) handleHAProxyStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Query HAProxy stats socket
	// For now, return basic info
	response := map[string]interface{}{
		"status": "unknown",
		"message": "HAProxy stats not yet implemented",
	}

	s.respondJSON(w, http.StatusOK, response)
}

// handleHAProxyReload triggers HAProxy reload
func (s *Server) handleHAProxyReload(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement HAProxy reload
	// systemctl reload haproxy
	response := map[string]interface{}{
		"status": "not_implemented",
		"message": "HAProxy reload not yet implemented - use systemctl reload haproxy",
	}

	s.respondJSON(w, http.StatusNotImplemented, response)
}
