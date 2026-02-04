package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/noahjeana/k8s-exposer/internal/protocol"
	"github.com/noahjeana/k8s-exposer/pkg/types"
)

// ServerClient manages the connection to the server and sends updates
type ServerClient struct {
	serverAddr      string
	conn            *protocol.Connection
	heartbeatTicker *time.Ticker
	logger          *slog.Logger
	mu              sync.Mutex
	lastServices    []types.ExposedService
}

// NewServerClient creates a new server client
func NewServerClient(serverAddr string, logger *slog.Logger) *ServerClient {
	return &ServerClient{
		serverAddr: serverAddr,
		conn:       protocol.NewConnection(serverAddr, logger),
		logger:     logger,
	}
}

// Connect connects to the server and starts the heartbeat
func (c *ServerClient) Connect(ctx context.Context) error {
	c.logger.Info("Connecting to server", "addr", c.serverAddr)

	if err := c.conn.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Start heartbeat
	c.startHeartbeat(ctx)

	return nil
}

// SendUpdate sends a service update to the server
func (c *ServerClient) SendUpdate(services []types.ExposedService) error {
	c.mu.Lock()
	c.lastServices = services
	c.mu.Unlock()

	msg := &types.Message{
		Type:     types.MessageTypeServiceUpdate,
		Services: services,
	}

	c.logger.Info("Sending service update", "count", len(services))
	
	// Debug: Log the service data
	if len(services) > 0 {
		for _, svc := range services {
			c.logger.Info("Service details", 
				"name", svc.Name,
				"subdomain", svc.Subdomain,
				"target_ip", svc.TargetIP,
				"ports", len(svc.Ports))
			for i, port := range svc.Ports {
				c.logger.Info("Port mapping",
					"index", i,
					"port", port.Port,
					"target_port", port.TargetPort,
					"protocol", port.Protocol)
			}
		}
	}

	if err := c.conn.Send(msg); err != nil {
		return fmt.Errorf("failed to send update: %w", err)
	}

	c.logger.Info("Service update sent successfully")
	return nil
}

// SendHeartbeat sends a heartbeat message to the server
func (c *ServerClient) SendHeartbeat() error {
	msg := &types.Message{
		Type: types.MessageTypeHeartbeat,
	}

	if err := c.conn.Send(msg); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	c.logger.Debug("Heartbeat sent")
	return nil
}

// startHeartbeat starts the heartbeat ticker
func (c *ServerClient) startHeartbeat(ctx context.Context) {
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
	}

	c.heartbeatTicker = time.NewTicker(30 * time.Second)

	go func() {
		for {
			select {
			case <-ctx.Done():
				c.heartbeatTicker.Stop()
				return
			case <-c.heartbeatTicker.C:
				if err := c.SendHeartbeat(); err != nil {
					c.logger.Warn("Failed to send heartbeat", "error", err)
				}
			}
		}
	}()
}

// Close closes the connection to the server
func (c *ServerClient) Close() error {
	if c.heartbeatTicker != nil {
		c.heartbeatTicker.Stop()
	}
	return c.conn.Close()
}

// Reconnect attempts to reconnect to the server
func (c *ServerClient) Reconnect(ctx context.Context) error {
	c.logger.Info("Reconnecting to server")

	if err := c.conn.Reconnect(ctx); err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	// Restart heartbeat
	c.startHeartbeat(ctx)

	// Resend last known services
	c.mu.Lock()
	services := c.lastServices
	c.mu.Unlock()

	if len(services) > 0 {
		c.logger.Info("Resending service list after reconnect", "count", len(services))
		if err := c.SendUpdate(services); err != nil {
			c.logger.Error("Failed to resend services after reconnect", "error", err)
		}
	}

	return nil
}

// IsConnected returns true if connected to the server
func (c *ServerClient) IsConnected() bool {
	return c.conn.IsConnected()
}

// Run runs the client with automatic reconnection
func (c *ServerClient) Run(ctx context.Context, onServicesChange <-chan []types.ExposedService) error {
	// Initial connection
	if err := c.Connect(ctx); err != nil {
		c.logger.Error("Failed to connect to server", "error", err)
		// Try to reconnect
		if err := c.Reconnect(ctx); err != nil {
			return fmt.Errorf("failed to establish initial connection: %w", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case services := <-onServicesChange:
			if err := c.SendUpdate(services); err != nil {
				c.logger.Error("Failed to send service update", "error", err)
				// Try to reconnect
				if err := c.Reconnect(ctx); err != nil {
					c.logger.Error("Failed to reconnect after send error", "error", err)
					continue
				}
				// Retry sending after reconnect
				if err := c.SendUpdate(services); err != nil {
					c.logger.Error("Failed to send service update after reconnect", "error", err)
				}
			}
		}
	}
}
