package firewall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client manages Hetzner Cloud Firewall
type Client struct {
	token      string
	firewallID string
	httpClient *http.Client
}

// NewClient creates a new Hetzner Firewall client
func NewClient(token, firewallID string) *Client {
	return &Client{
		token:      token,
		firewallID: firewallID,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FirewallRule represents a Hetzner firewall rule
type FirewallRule struct {
	Direction   string   `json:"direction"`
	SourceIPs   []string `json:"source_ips,omitempty"`
	Protocol    string   `json:"protocol"`
	Port        string   `json:"port,omitempty"`
	Description string   `json:"description,omitempty"`
}

// GetRules retrieves current firewall rules
func (c *Client) GetRules() ([]FirewallRule, error) {
	if c.token == "" || c.firewallID == "" {
		return nil, fmt.Errorf("firewall management disabled (no token or firewall ID)")
	}

	url := fmt.Sprintf("https://api.hetzner.cloud/v1/firewalls/%s", c.firewallID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall rules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Firewall struct {
			Rules []FirewallRule `json:"rules"`
		} `json:"firewall"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Firewall.Rules, nil
}

// SetRules updates firewall rules
func (c *Client) SetRules(rules []FirewallRule) error {
	if c.token == "" || c.firewallID == "" {
		return fmt.Errorf("firewall management disabled (no token or firewall ID)")
	}

	url := fmt.Sprintf("https://api.hetzner.cloud/v1/firewalls/%s/actions/set_rules", c.firewallID)

	payload := map[string]interface{}{
		"rules": rules,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set firewall rules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// EnsurePortsOpen ensures the specified ports are open in the firewall
func (c *Client) EnsurePortsOpen(ports []int) error {
	if c.token == "" || c.firewallID == "" {
		// Firewall management disabled
		return nil
	}

	// Get current rules
	currentRules, err := c.GetRules()
	if err != nil {
		return err
	}

	// Build desired rules (keep existing non-k8s-exposer rules)
	var newRules []FirewallRule

	// Keep existing rules that are not managed by k8s-exposer
	for _, rule := range currentRules {
		if rule.Description != "" && rule.Description != "k8s-exposer" {
			newRules = append(newRules, rule)
		}
	}

	// Add SSH rule (always keep)
	sshExists := false
	for _, rule := range currentRules {
		if rule.Port == "22" && rule.Protocol == "tcp" {
			sshExists = true
			newRules = append(newRules, rule)
			break
		}
	}
	if !sshExists {
		newRules = append(newRules, FirewallRule{
			Direction:   "in",
			Protocol:    "tcp",
			Port:        "22",
			SourceIPs:   []string{"0.0.0.0/0", "::/0"},
			Description: "SSH",
		})
	}

	// Add HTTP/HTTPS (always keep)
	newRules = append(newRules, FirewallRule{
		Direction:   "in",
		Protocol:    "tcp",
		Port:        "80",
		SourceIPs:   []string{"0.0.0.0/0", "::/0"},
		Description: "HTTP",
	})
	newRules = append(newRules, FirewallRule{
		Direction:   "in",
		Protocol:    "tcp",
		Port:        "443",
		SourceIPs:   []string{"0.0.0.0/0", "::/0"},
		Description: "HTTPS",
	})

	// Add k8s-exposer managed ports
	for _, port := range ports {
		newRules = append(newRules, FirewallRule{
			Direction:   "in",
			Protocol:    "tcp",
			Port:        fmt.Sprintf("%d", port),
			SourceIPs:   []string{"0.0.0.0/0", "::/0"},
			Description: "k8s-exposer",
		})
	}

	// Update rules
	return c.SetRules(newRules)
}

// Validate checks if firewall management is configured
func (c *Client) Validate() error {
	if c.token == "" {
		return fmt.Errorf("firewall token not configured")
	}
	if c.firewallID == "" {
		return fmt.Errorf("firewall ID not configured")
	}
	return nil
}

// Enabled returns true if firewall management is enabled
func (c *Client) Enabled() bool {
	return c.token != "" && c.firewallID != ""
}
