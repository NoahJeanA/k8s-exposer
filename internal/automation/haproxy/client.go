package haproxy

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Client manages HAProxy via Runtime API socket
type Client struct {
	socketPath string
	mapFile    string
}

// NewClient creates a new HAProxy client
func NewClient(socketPath, mapFile string) *Client {
	return &Client{
		socketPath: socketPath,
		mapFile:    mapFile,
	}
}

// runCommand executes a command via HAProxy socket
func (c *Client) runCommand(command string) (string, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to connect to socket: %w", err)
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send command
	_, err = conn.Write([]byte(command + "\n"))
	if err != nil {
		return "", fmt.Errorf("failed to write command: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(conn)
	var response strings.Builder
	for scanner.Scan() {
		response.WriteString(scanner.Text())
		response.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return response.String(), nil
}

// GetCurrentMappings returns current domain to backend mappings from map file
func (c *Client) GetCurrentMappings() (map[string]string, error) {
	mappings := make(map[string]string)

	file, err := os.Open(c.mapFile)
	if err != nil {
		if os.IsNotExist(err) {
			return mappings, nil // Empty map if file doesn't exist yet
		}
		return nil, fmt.Errorf("failed to open map file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			mappings[parts[0]] = parts[1]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read map file: %w", err)
	}

	return mappings, nil
}

// AddMapping adds a domain to backend mapping via Runtime API
func (c *Client) AddMapping(domain, backend string) error {
	// Add to runtime map (live, no reload!)
	command := fmt.Sprintf("add map %s %s %s", c.mapFile, domain, backend)
	_, err := c.runCommand(command)
	if err != nil {
		return fmt.Errorf("failed to add mapping via Runtime API: %w", err)
	}

	// Persist to file
	file, err := os.OpenFile(c.mapFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open map file for writing: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(fmt.Sprintf("%s %s\n", domain, backend)); err != nil {
		return fmt.Errorf("failed to persist mapping: %w", err)
	}

	return nil
}

// RemoveMapping removes a domain mapping via Runtime API
func (c *Client) RemoveMapping(domain string) error {
	// Remove from runtime map
	command := fmt.Sprintf("del map %s %s", c.mapFile, domain)
	_, err := c.runCommand(command)
	if err != nil {
		return fmt.Errorf("failed to remove mapping via Runtime API: %w", err)
	}

	// Remove from file
	mappings, err := c.GetCurrentMappings()
	if err != nil {
		return err
	}
	delete(mappings, domain)

	// Rewrite file
	file, err := os.OpenFile(c.mapFile, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open map file for writing: %w", err)
	}
	defer file.Close()

	// Write header
	file.WriteString("# HAProxy domain to backend mapping\n")
	file.WriteString("# Format: domain backend_name\n")
	file.WriteString("# Managed by k8s-exposer automation\n\n")

	// Write mappings
	for domain, backend := range mappings {
		file.WriteString(fmt.Sprintf("%s %s\n", domain, backend))
	}

	return nil
}

// Validate checks if HAProxy socket is accessible
func (c *Client) Validate() error {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to HAProxy socket: %w", err)
	}
	conn.Close()
	return nil
}
