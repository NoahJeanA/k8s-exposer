package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/noahjeana/k8s-exposer/internal/automation"
	"github.com/noahjeana/k8s-exposer/internal/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server provides HTTP API for management and monitoring
type Server struct {
	registry   *server.ServiceRegistry
	automation *automation.Controller
	logger     *slog.Logger
	router     chi.Router
}

// NewServer creates a new API server
func NewServer(registry *server.ServiceRegistry, automation *automation.Controller, logger *slog.Logger) *Server {
	s := &Server{
		registry:   registry,
		automation: automation,
		logger:     logger.With("component", "api"),
		router:     chi.NewRouter(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	r := s.router

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.loggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Services
		r.Get("/services", s.handleListServices)
		r.Get("/services/{name}", s.handleGetService)

		// System
		r.Get("/health", s.handleHealth)
		r.Get("/metrics", s.handleMetrics)
		r.Post("/sync", s.handleSync)

		// HAProxy
		r.Route("/haproxy", func(r chi.Router) {
			r.Get("/status", s.handleHAProxyStatus)
			r.Post("/reload", s.handleHAProxyReload)
		})
	})

	// Legacy routes (backwards compatibility)
	r.Get("/health", s.handleHealth)
	r.Get("/services", s.handleListServices)

	// Prometheus metrics endpoint (standard path)
	r.Handle("/metrics", promhttp.Handler())
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
	s.logger.Info("Starting API server", "addr", addr)
	
	// Start background goroutine to update service metrics
	go s.updateServiceMetrics()
	
	return http.ListenAndServe(addr, s.router)
}

// updateServiceMetrics periodically updates Prometheus service gauges
func (s *Server) updateServiceMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	
	for {
		services := s.registry.GetServices()
		servicesTotal.Set(float64(len(services)))
		
		totalPorts := 0
		for _, svc := range services {
			totalPorts += len(svc.Ports)
		}
		portsTotal.Set(float64(totalPorts))
		
		<-ticker.C
	}
}

// loggingMiddleware logs HTTP requests and records metrics
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a response writer wrapper to capture status code
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		next.ServeHTTP(ww, r)
		
		duration := time.Since(start)
		
		s.logger.Info("API request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.statusCode,
			"duration_ms", duration.Milliseconds(),
			"remote", r.RemoteAddr,
		)

		// Record Prometheus metrics (skip /metrics endpoint itself)
		if r.URL.Path != "/metrics" {
			httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, fmt.Sprintf("%d", ww.statusCode)).Inc()
			httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration.Seconds())
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Helper functions

func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{
		"error": message,
	})
}
