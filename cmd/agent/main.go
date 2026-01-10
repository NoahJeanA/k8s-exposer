package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/noahjeana/k8s-exposer/internal/agent"
	"github.com/noahjeana/k8s-exposer/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Parse environment variables
	serverAddr := getEnv("SERVER_ADDR", "10.0.0.1:9090")
	clusterDomain := getEnv("CLUSTER_DOMAIN", "neverup.at")
	logLevel := getEnv("LOG_LEVEL", "INFO")
	syncInterval := getEnvDuration("SYNC_INTERVAL", 30*time.Second)

	// Setup logger
	logger := setupLogger(logLevel)
	logger.Info("Starting k8s-exposer agent",
		"server_addr", serverAddr,
		"cluster_domain", clusterDomain,
		"sync_interval", syncInterval)

	// Create context that listens for shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("Received shutdown signal", "signal", sig)
		cancel()
	}()

	// Initialize Kubernetes client (in-cluster config)
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to get in-cluster config", "error", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create Kubernetes client", "error", err)
		os.Exit(1)
	}

	logger.Info("Kubernetes client initialized")

	// Create channel for service updates
	serviceUpdateCh := make(chan []types.ExposedService, 10)

	// Create server client
	serverClient := agent.NewServerClient(serverAddr, logger)

	// Start server client in background
	go func() {
		if err := serverClient.Run(ctx, serviceUpdateCh); err != nil && err != context.Canceled {
			logger.Error("Server client stopped with error", "error", err)
			cancel()
		}
	}()

	// Create service watcher
	watcher := agent.NewServiceWatcher(clientset, func(services []types.ExposedService) {
		logger.Info("Service change detected", "count", len(services))
		select {
		case serviceUpdateCh <- services:
		case <-ctx.Done():
		}
	}, logger)

	// Start periodic sync
	go func() {
		ticker := time.NewTicker(syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Debug("Performing periodic service discovery")
				services, err := agent.DiscoverServices(ctx, clientset, logger)
				if err != nil {
					logger.Error("Periodic discovery failed", "error", err)
					continue
				}
				select {
				case serviceUpdateCh <- services:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	// Start service watcher (blocks until context is canceled)
	logger.Info("Starting service watcher")
	if err := watcher.StartWithRetry(ctx); err != nil && err != context.Canceled {
		logger.Error("Service watcher failed", "error", err)
		os.Exit(1)
	}

	// Cleanup
	logger.Info("Shutting down gracefully")
	serverClient.Close()
	logger.Info("Agent stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler).With("component", "agent")
}
