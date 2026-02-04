package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client for k8s-exposer API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new API client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Service represents an exposed service
type Service struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Subdomain string        `json:"subdomain"`
	TargetIP  string        `json:"target_ip"`
	NodeIP    string        `json:"node_ip,omitempty"`
	FQDN      string        `json:"fqdn,omitempty"`
	Ports     []PortMapping `json:"ports"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	Port       int32  `json:"port"`
	TargetPort int32  `json:"target_port"`
	Protocol   string `json:"protocol"`
}

// Health represents health status
type Health struct {
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
	ServiceCount int    `json:"service_count"`
	Version      string `json:"version"`
}

// Metrics represents system metrics
type Metrics struct {
	Timestamp string                 `json:"timestamp"`
	Services  map[string]interface{} `json:"services"`
	Memory    map[string]interface{} `json:"memory"`
	Runtime   map[string]interface{} `json:"runtime"`
}

// GetHealth returns health status
func (c *Client) GetHealth() (*Health, error) {
	var health Health
	if err := c.get("/api/v1/health", &health); err != nil {
		return nil, err
	}
	return &health, nil
}

// GetMetrics returns system metrics
func (c *Client) GetMetrics() (*Metrics, error) {
	var metrics Metrics
	if err := c.get("/api/v1/metrics", &metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}

// ListServices returns all services
func (c *Client) ListServices() ([]Service, error) {
	var response struct {
		Services []Service `json:"services"`
		Count    int       `json:"count"`
	}
	if err := c.get("/api/v1/services", &response); err != nil {
		return nil, err
	}
	return response.Services, nil
}

// GetService returns a specific service
func (c *Client) GetService(name string) (*Service, error) {
	var service Service
	if err := c.get(fmt.Sprintf("/api/v1/services/%s", name), &service); err != nil {
		return nil, err
	}
	return &service, nil
}

// Sync triggers reconciliation
func (c *Client) Sync() error {
	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/sync", "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to sync: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync failed: %s", string(body))
	}

	return nil
}

// get performs a GET request
func (c *Client) get(path string, target interface{}) error {
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}
