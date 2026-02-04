package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/noahjeana/k8s-exposer/internal/api"
	"github.com/noahjeana/k8s-exposer/internal/automation"
	"github.com/noahjeana/k8s-exposer/internal/protocol"
	"github.com/noahjeana/k8s-exposer/internal/server"
	"github.com/noahjeana/k8s-exposer/pkg/types"
)

func main() {
	// Parse environment variables
	listenAddr := getEnv("EXPOSER_LISTEN_ADDR", "10.0.0.1:9090")
	apiListenAddr := getEnv("EXPOSER_API_LISTEN_ADDR", "0.0.0.0:8090")
	logLevel := getEnv("EXPOSER_LOG_LEVEL", "INFO")
	wireguardInterface := getEnv("EXPOSER_WIREGUARD_INTERFACE", "wg0")
	portRangeStart := getEnvInt32("EXPOSER_PORT_RANGE_START", 30000)
	portRangeEnd := getEnvInt32("EXPOSER_PORT_RANGE_END", 32767)

	// Automation configuration
	domain := getEnv("DOMAIN", "neverup.at")
	haproxySocket := getEnv("HAPROXY_SOCKET", "/var/run/haproxy.sock")
	haproxyMap := getEnv("HAPROXY_MAP", "/etc/haproxy/domains.map")
	haproxyConfig := getEnv("HAPROXY_CONFIG", "/etc/haproxy/haproxy.cfg")
	firewallToken := getEnv("HETZNER_CLOUD_TOKEN", "")
	firewallID := getEnv("HETZNER_FIREWALL_ID", "")
	reconcileInterval := getEnvDuration("RECONCILE_INTERVAL", 30*time.Second)

	// Setup logger
	logger := setupLogger(logLevel)
	logger.Info("Starting k8s-exposer server",
		"listen_addr", listenAddr,
		"api_listen_addr", apiListenAddr,
		"wireguard_interface", wireguardInterface,
		"port_range", fmt.Sprintf("%d-%d", portRangeStart, portRangeEnd))

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

	// Initialize forwarder
	forwarder := server.NewForwarder(wireguardInterface, logger)
	defer forwarder.Close()

	// Initialize service registry
	registry := server.NewServiceRegistry(portRangeStart, portRangeEnd, forwarder, logger)
	defer registry.Close()

	// Initialize automation controller
	automationConfig := automation.Config{
		HAProxySocket:     haproxySocket,
		HAProxyMap:        haproxyMap,
		HAProxyConfig:     haproxyConfig,
		FirewallToken:     firewallToken,
		FirewallID:        firewallID,
		Domain:            domain,
		ReconcileInterval: reconcileInterval,
	}
	automationController := automation.NewController(automationConfig, logger)

	// Start automation controller in background
	go func() {
		logger.Info("Starting automation controller")
		if err := automationController.Run(ctx, func() []types.ExposedService {
			return registry.GetServices()
		}); err != nil && err != context.Canceled {
			logger.Error("Automation controller failed", "error", err)
		}
	}()

	// Start new API server in background
	apiServer := api.NewServer(registry, automationController, logger)
	go func() {
		logger.Info("Starting API server", "addr", apiListenAddr)
		if err := apiServer.Start(apiListenAddr); err != nil {
			logger.Error("API server failed", "error", err)
			cancel() // Stop the whole server if API fails
		}
	}()

	// Start listening for agent connections
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Error("Failed to start listener", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Info("Server listening for agent connections", "addr", listenAddr)

	// Accept connections in a goroutine
	connCh := make(chan net.Conn)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					logger.Error("Failed to accept connection", "error", err)
					continue
				}
			}
			connCh <- conn
		}
	}()

	// Main loop
	for {
		select {
		case <-ctx.Done():
			logger.Info("Shutting down gracefully")
			return

		case conn := <-connCh:
			logger.Info("Agent connected", "remote", conn.RemoteAddr())
			go handleAgentConnection(ctx, conn, registry, logger)
		}
	}
}

func handleAgentConnection(ctx context.Context, conn net.Conn, registry *server.ServiceRegistry, logger *slog.Logger) {
	defer conn.Close()

	logger = logger.With("agent", conn.RemoteAddr())
	logger.Info("Handling agent connection")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Context canceled, closing agent connection")
			return
		default:
		}

		// Receive message
		msg, err := protocol.ReceiveMessage(conn)
		if err != nil {
			logger.Error("Failed to receive message", "error", err)
			return
		}

		// Process message
		switch msg.Type {
		case types.MessageTypeServiceUpdate:
			logger.Info("Received service update", "count", len(msg.Services))
			if err := registry.Update(msg.Services); err != nil {
				logger.Error("Failed to update registry", "error", err)
			}

		case types.MessageTypeServiceDelete:
			logger.Info("Received service delete", "count", len(msg.Services))
			for _, svc := range msg.Services {
				if err := registry.RemoveService(svc.Subdomain); err != nil {
					logger.Error("Failed to remove service", "subdomain", svc.Subdomain, "error", err)
				}
			}

		case types.MessageTypeHeartbeat:
			logger.Debug("Received heartbeat")

		default:
			logger.Warn("Received unknown message type", "type", msg.Type)
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt32(key string, defaultValue int32) int32 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 32); err == nil {
			return int32(intVal)
		}
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
	return slog.New(handler).With("component", "server")
}
