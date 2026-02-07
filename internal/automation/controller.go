package automation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/noahjeana/k8s-exposer/internal/automation/firewall"
	"github.com/noahjeana/k8s-exposer/internal/automation/haproxy"
	"github.com/noahjeana/k8s-exposer/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	reconciliationsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "k8s_exposer_reconciliations_total",
		Help: "Total number of reconciliation runs",
	})

	reconciliationErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "k8s_exposer_reconciliation_errors_total",
		Help: "Total number of reconciliation errors",
	})

	lastReconciliationTime = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "k8s_exposer_last_reconciliation_timestamp_seconds",
		Help: "Unix timestamp of last reconciliation",
	})
)

// Controller manages HAProxy and firewall automation
type Controller struct {
	haproxyClient    *haproxy.Client
	haproxyGenerator *haproxy.ConfigGenerator
	firewallClient   *firewall.Client
	domain           string
	haproxyConfig    string
	reconcileInterval time.Duration
	logger           *slog.Logger
}

// Config contains automation controller configuration
type Config struct {
	// HAProxy
	HAProxySocket string
	HAProxyMap    string
	HAProxyConfig string

	// Firewall
	FirewallToken string
	FirewallID    string

	// General
	Domain            string
	ReconcileInterval time.Duration
}

// NewController creates a new automation controller
func NewController(cfg Config, logger *slog.Logger) *Controller {
	return &Controller{
		haproxyClient:     haproxy.NewClient(cfg.HAProxySocket, cfg.HAProxyMap),
		haproxyGenerator:  haproxy.NewConfigGenerator(cfg.HAProxyMap),
		firewallClient:    firewall.NewClient(cfg.FirewallToken, cfg.FirewallID),
		domain:            cfg.Domain,
		haproxyConfig:     cfg.HAProxyConfig,
		reconcileInterval: cfg.ReconcileInterval,
		logger:            logger,
	}
}

// Reconcile performs a full reconciliation of HAProxy and firewall
func (c *Controller) Reconcile(services []types.ExposedService) error {
	c.logger.Info("Starting reconciliation", "service_count", len(services))

	// Collect desired state
	desiredMappings := make(map[string]string)
	desiredPorts := make([]int, 0)
	backendConfigs := make([]haproxy.BackendConfig, 0)

	for _, svc := range services {
		if len(svc.Ports) == 0 {
			continue
		}

		// Use first port
		port := svc.Ports[0].Port
		backend := fmt.Sprintf("backend_%d", port)
		fqdn := fmt.Sprintf("%s.%s", svc.Subdomain, c.domain)

		desiredMappings[fqdn] = backend
		desiredPorts = append(desiredPorts, int(port))
		backendConfigs = append(backendConfigs, haproxy.BackendConfig{
			Name: svc.Name,
			Port: int(port),
		})
	}

	// Update HAProxy configuration
	if err := c.reconcileHAProxy(desiredMappings, backendConfigs); err != nil {
		c.logger.Error("Failed to reconcile HAProxy", "error", err)
		reconciliationErrors.Inc()
		return err
	}

	// Update firewall rules
	if err := c.reconcileFirewall(desiredPorts); err != nil {
		c.logger.Error("Failed to reconcile firewall", "error", err)
		// Don't fail on firewall errors - continue
	}

	c.logger.Info("Reconciliation complete", "domains", len(desiredMappings), "ports", len(desiredPorts))
	
	// Record successful reconciliation
	reconciliationsTotal.Inc()
	lastReconciliationTime.SetToCurrentTime()
	
	return nil
}

// reconcileHAProxy updates HAProxy domain mappings and backends
func (c *Controller) reconcileHAProxy(desiredMappings map[string]string, backends []haproxy.BackendConfig) error {
	// Get current mappings
	currentMappings, err := c.haproxyClient.GetCurrentMappings()
	if err != nil {
		return fmt.Errorf("failed to get current mappings: %w", err)
	}

	// Add new mappings
	for domain, backend := range desiredMappings {
		if currentBackend, exists := currentMappings[domain]; exists {
			if currentBackend == backend {
				continue // Already correct
			}
			// Remove old mapping first
			if err := c.haproxyClient.RemoveMapping(domain); err != nil {
				c.logger.Warn("Failed to remove old mapping", "domain", domain, "error", err)
			}
		}

		// Add new mapping
		if err := c.haproxyClient.AddMapping(domain, backend); err != nil {
			return fmt.Errorf("failed to add mapping %s -> %s: %w", domain, backend, err)
		}
		c.logger.Info("Added domain mapping", "domain", domain, "backend", backend)
	}

	// Generate new HAProxy config with all backends
	if err := c.haproxyGenerator.Generate(backends, c.haproxyConfig); err != nil {
		return fmt.Errorf("failed to generate HAProxy config: %w", err)
	}
	c.logger.Info("Generated HAProxy config", "backends", len(backends))

	// TODO: Reload HAProxy gracefully
	// For now, manual reload required: systemctl reload haproxy

	return nil
}

// reconcileFirewall updates firewall rules
func (c *Controller) reconcileFirewall(ports []int) error {
	if !c.firewallClient.Enabled() {
		c.logger.Debug("Firewall management disabled")
		return nil
	}

	if err := c.firewallClient.EnsurePortsOpen(ports); err != nil {
		return fmt.Errorf("failed to update firewall: %w", err)
	}

	c.logger.Info("Updated firewall rules", "ports", ports)
	return nil
}

// Run starts the reconciliation loop
func (c *Controller) Run(ctx context.Context, serviceGetter func() []types.ExposedService) error {
	c.logger.Info("Starting automation controller",
		"domain", c.domain,
		"interval", c.reconcileInterval,
		"firewall_enabled", c.firewallClient.Enabled(),
	)

	// Wait for HAProxy to be ready (retry with backoff)
	for i := 0; i < 30; i++ {
		if err := c.haproxyClient.Validate(); err != nil {
			c.logger.Warn("HAProxy not ready, retrying...", "attempt", i+1, "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(i+1) * time.Second):
				continue
			}
		}
		break
	}
	
	// Final validation check
	if err := c.haproxyClient.Validate(); err != nil {
		return fmt.Errorf("HAProxy validation failed after retries: %w", err)
	}

	ticker := time.NewTicker(c.reconcileInterval)
	defer ticker.Stop()

	// Wait a bit for agents to connect before initial reconciliation
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	// Initial reconciliation
	services := serviceGetter()
	if err := c.Reconcile(services); err != nil {
		c.logger.Error("Initial reconciliation failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Automation controller stopping")
			return ctx.Err()
		case <-ticker.C:
			services := serviceGetter()
			if err := c.Reconcile(services); err != nil {
				c.logger.Error("Reconciliation failed", "error", err)
			}
		}
	}
}
