package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// APIServer provides HTTP API for querying service status
type APIServer struct {
	registry *ServiceRegistry
	logger   *slog.Logger
}

// NewAPIServer creates a new API server
func NewAPIServer(registry *ServiceRegistry, logger *slog.Logger) *APIServer {
	return &APIServer{
		registry: registry,
		logger:   logger,
	}
}

// Start starts the HTTP API server
func (a *APIServer) Start(addr string) error {
	mux := http.NewServeMux()
	
	// Health check endpoint
	mux.HandleFunc("/health", a.handleHealth)
	
	// Services endpoint
	mux.HandleFunc("/services", a.handleServices)
	
	a.logger.Info("Starting API server", "addr", addr)
	return http.ListenAndServe(addr, a.loggingMiddleware(mux))
}

// handleHealth returns health status
func (a *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	a.registry.mu.RLock()
	serviceCount := len(a.registry.services)
	a.registry.mu.RUnlock()
	
	response := map[string]interface{}{
		"status":   "healthy",
		"services": serviceCount,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleServices returns list of all exposed services
func (a *APIServer) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	a.registry.mu.RLock()
	services := make([]interface{}, 0, len(a.registry.services))
	for _, svc := range a.registry.services {
		services = append(services, map[string]interface{}{
			"name":      svc.Name,
			"namespace": svc.Namespace,
			"subdomain": svc.Subdomain,
			"ports":     svc.Ports,
		})
	}
	a.registry.mu.RUnlock()
	
	response := map[string]interface{}{
		"services": services,
		"count":    len(services),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// loggingMiddleware logs HTTP requests
func (a *APIServer) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.logger.Debug("API request", 
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
