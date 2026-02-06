package haproxy

import (
	"fmt"
	"os"
	"text/template"
)

const configTemplate = `# HAProxy Configuration for k8s-exposer
# Auto-generated - DO NOT EDIT MANUALLY

global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /var/run/haproxy.sock mode 660 level admin expose-fd listeners
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # Performance tuning
    maxconn 10000
    tune.bufsize 32768
    tune.maxrewrite 8192

    # Default SSL material locations
    ca-base /etc/ssl/certs
    crt-base /etc/ssl/private

    # Modern SSL configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-ciphersuites TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384:TLS_CHACHA20_POLY1305_SHA256
    ssl-default-bind-options ssl-min-ver TLSv1.2 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  3600000
    timeout server  3600000
    errorfile 400 /etc/haproxy/errors/400.http
    errorfile 403 /etc/haproxy/errors/403.http
    errorfile 408 /etc/haproxy/errors/408.http
    errorfile 500 /etc/haproxy/errors/500.http
    errorfile 502 /etc/haproxy/errors/502.http
    errorfile 503 /etc/haproxy/errors/503.http
    errorfile 504 /etc/haproxy/errors/504.http

# Stats page
frontend stats
    bind *:8404
    stats enable
    stats uri /stats
    stats refresh 10s
    stats admin if TRUE

# HTTP Frontend
frontend http_front
    bind *:80
    
    # ACME challenge exception (for Let's Encrypt)
    acl is_acme_challenge path_beg /.well-known/acme-challenge/
    use_backend backend_acme if is_acme_challenge
    
    # Redirect to HTTPS
    http-request redirect scheme https code 301 if !is_acme_challenge
    
    # Use domain map for dynamic routing (fallback)
    use_backend %[req.hdr(host),lower,map({{.MapFile}},backend_default)]

# ACME challenge backend
backend backend_acme
    mode http
    server acme localhost:8888

# HTTPS Frontend{{if .HasSSL}}
frontend https_front
    bind *:443 ssl crt /etc/ssl/private/ alpn h2,http/1.1
    mode http
    
    # Use SNI to route to backends
    use_backend %[ssl_fc_sni,lower,map({{.MapFile}},backend_default)]
{{end}}

# Default backend (404)
backend backend_default
    mode http
    http-request return status 404 content-type text/html string "<html><body><h1>404 Not Found</h1><p>Service not configured</p></body></html>"

{{range .Backends}}
# Backend for {{.Name}} (port {{.Port}})
backend backend_{{.Port}}
    mode http
    {{if eq .Port 2283}}# Connection limit for Immich uploads (max 3 concurrent per IP)
    stick-table type ip size 100k expire 30s store conn_cur
    acl too_many_uploads src_conn_cur gt 3
    http-request deny deny_status 429 if too_many_uploads
    {{end}}
    server {{.Name}} 127.0.0.1:{{.Port}}
{{end}}
`

// BackendConfig represents a HAProxy backend configuration
type BackendConfig struct {
	Name string
	Port int
}

// ConfigGenerator generates HAProxy configuration
type ConfigGenerator struct {
	mapFile string
}

// NewConfigGenerator creates a new config generator
func NewConfigGenerator(mapFile string) *ConfigGenerator {
	return &ConfigGenerator{
		mapFile: mapFile,
	}
}

// Generate generates HAProxy configuration with backends
func (g *ConfigGenerator) Generate(backends []BackendConfig, outputPath string) error {
	tmpl, err := template.New("haproxy").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Check if SSL certificates exist
	hasSSL := false
	if _, err := os.Stat("/etc/ssl/private"); err == nil {
		// Check if there's at least one .pem file
		entries, err := os.ReadDir("/etc/ssl/private")
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && len(entry.Name()) > 4 && entry.Name()[len(entry.Name())-4:] == ".pem" {
					hasSSL = true
					break
				}
			}
		}
	}

	data := struct {
		MapFile  string
		Backends []BackendConfig
		HasSSL   bool
	}{
		MapFile:  g.mapFile,
		Backends: backends,
		HasSSL:   hasSSL,
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

// ValidateConfig validates HAProxy configuration file
func (g *ConfigGenerator) ValidateConfig(configPath string) error {
	// Run haproxy -c -f <config>
	// For now, just check if file exists
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("config file not found: %w", err)
	}
	return nil
}
