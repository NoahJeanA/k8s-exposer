package types

import (
	"fmt"
	"regexp"
)

// ExposedService represents a Kubernetes service that should be exposed externally
type ExposedService struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Subdomain string        `json:"subdomain"`  // From annotation: expose.neverup.at/subdomain
	Ports     []PortMapping `json:"ports"`      // From annotation: expose.neverup.at/ports
	TargetIP  string        `json:"target_ip"`  // K8s ClusterIP or Node IP
	NodeIP    string        `json:"node_ip"`    // For NodePort fallback
}

// PortMapping defines a port and protocol to expose
type PortMapping struct {
	Port       int32  `json:"port"`        // Port to expose externally
	TargetPort int32  `json:"target_port"` // Internal target port
	Protocol   string `json:"protocol"`    // "tcp", "udp", or "tcp+udp"
}

// MessageType defines the type of message sent between agent and server
type MessageType string

const (
	MessageTypeServiceUpdate MessageType = "service_update"
	MessageTypeServiceDelete MessageType = "service_delete"
	MessageTypeHeartbeat     MessageType = "heartbeat"
)

// Message is the wrapper for all communications between agent and server
type Message struct {
	Type     MessageType      `json:"type"`
	Services []ExposedService `json:"services,omitempty"`
}

// Validate validates an ExposedService
func (s *ExposedService) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if s.Namespace == "" {
		return fmt.Errorf("service namespace cannot be empty")
	}
	if err := ValidateSubdomain(s.Subdomain); err != nil {
		return fmt.Errorf("invalid subdomain: %w", err)
	}
	if len(s.Ports) == 0 {
		return fmt.Errorf("at least one port mapping is required")
	}
	for i, port := range s.Ports {
		if err := port.Validate(); err != nil {
			return fmt.Errorf("invalid port mapping at index %d: %w", i, err)
		}
	}
	if s.TargetIP == "" {
		return fmt.Errorf("target IP cannot be empty")
	}
	return nil
}

// Validate validates a PortMapping
func (p *PortMapping) Validate() error {
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", p.Port)
	}
	if p.Protocol != "tcp" && p.Protocol != "udp" && p.Protocol != "tcp+udp" {
		return fmt.Errorf("protocol must be 'tcp', 'udp', or 'tcp+udp', got %q", p.Protocol)
	}
	return nil
}

// ValidateSubdomain validates a subdomain string
func ValidateSubdomain(subdomain string) error {
	if subdomain == "" {
		return fmt.Errorf("subdomain cannot be empty")
	}
	// DNS label validation: alphanumeric and hyphens, cannot start/end with hyphen
	validSubdomain := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	if !validSubdomain.MatchString(subdomain) {
		return fmt.Errorf("subdomain %q is not a valid DNS label", subdomain)
	}
	return nil
}

// Validate validates a Message
func (m *Message) Validate() error {
	if m.Type != MessageTypeServiceUpdate &&
	   m.Type != MessageTypeServiceDelete &&
	   m.Type != MessageTypeHeartbeat {
		return fmt.Errorf("invalid message type: %q", m.Type)
	}
	if m.Type == MessageTypeServiceUpdate || m.Type == MessageTypeServiceDelete {
		for i, svc := range m.Services {
			if err := svc.Validate(); err != nil {
				return fmt.Errorf("invalid service at index %d: %w", i, err)
			}
		}
	}
	return nil
}
