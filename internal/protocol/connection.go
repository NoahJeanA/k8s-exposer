package protocol

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/noahjeana/k8s-exposer/pkg/types"
)

// Connection represents a persistent TCP connection between agent and server
type Connection struct {
	addr       string
	conn       net.Conn
	mu         sync.Mutex
	reconnectDelay time.Duration
	maxReconnectDelay time.Duration
	logger     *slog.Logger
}

// NewConnection creates a new connection to the specified address
func NewConnection(addr string, logger *slog.Logger) *Connection {
	return &Connection{
		addr:              addr,
		reconnectDelay:    1 * time.Second,
		maxReconnectDelay: 60 * time.Second,
		logger:            logger,
	}
}

// Connect establishes a connection to the server
func (c *Connection) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return fmt.Errorf("already connected")
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.addr, err)
	}

	c.conn = conn
	c.logger.Info("Connected to server", "addr", c.addr)
	return nil
}

// Send sends a message over the connection
func (c *Connection) Send(msg *types.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := SendMessage(c.conn, msg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// Receive receives a message from the connection
func (c *Connection) Receive() (*types.Message, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	msg, err := ReceiveMessage(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to receive message: %w", err)
	}

	return msg, nil
}

// Close closes the connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	c.logger.Info("Connection closed")
	return err
}

// Reconnect attempts to reconnect to the server with exponential backoff
func (c *Connection) Reconnect(ctx context.Context) error {
	c.Close()

	delay := c.reconnectDelay
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			c.logger.Info("Attempting to reconnect", "addr", c.addr, "delay", delay)

			if err := c.Connect(ctx); err != nil {
				c.logger.Warn("Reconnection failed", "error", err)
				// Exponential backoff
				delay *= 2
				if delay > c.maxReconnectDelay {
					delay = c.maxReconnectDelay
				}
				continue
			}

			c.logger.Info("Reconnected successfully")
			return nil
		}
	}
}

// IsConnected returns true if the connection is established
func (c *Connection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}
