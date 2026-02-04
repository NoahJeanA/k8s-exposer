# k8s-exposer

**Kubernetes Service Exposer** - Automatically expose Kubernetes services via WireGuard with HAProxy load balancing.

## Features

✅ **Automatic Service Discovery** - Annotate services, get instant exposure  
✅ **Pod-IP Forwarding** - Direct routing to pods via WireGuard  
✅ **HAProxy Integration** - Automatic backend generation and domain routing  
✅ **Firewall Management** - Auto-open ports (Hetzner Cloud)  
✅ **Zero Dependencies** - Single static Go binary  
✅ **Production Ready** - Systemd integration, graceful shutdown  

## Quick Start

### 1. Deploy Agent in Kubernetes

```bash
kubectl apply -f deploy/agent.yaml
```

### 2. Install Server on Edge Node

```bash
# Download binary
wget https://github.com/noahjeana/k8s-exposer/releases/latest/k8s-exposer-server

# Install
sudo mv k8s-exposer-server /usr/local/bin/
sudo chmod +x /usr/local/bin/k8s-exposer-server

# Configure systemd
sudo systemctl enable k8s-exposer
sudo systemctl start k8s-exposer
```

### 3. Expose a Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app
  annotations:
    expose.neverup.at/subdomain: "app"
    expose.neverup.at/ports: "8080/tcp"
spec:
  selector:
    app: my-app
  ports:
    - port: 8080
      targetPort: 80
```

**Done!** Your service is now available at `app.neverup.at`

## Architecture

```
Internet → HAProxy → k8s-exposer-server → WireGuard → Kubernetes Pods
```

## Configuration

### Server Environment Variables

```bash
EXPOSER_LISTEN_ADDR=10.0.0.1:9090          # Agent connection endpoint
EXPOSER_API_LISTEN_ADDR=0.0.0.0:8090       # REST API endpoint
DOMAIN=neverup.at                          # Your domain
HAPROXY_SOCKET=/var/run/haproxy.sock       # HAProxy admin socket
HAPROXY_MAP=/etc/haproxy/domains.map       # Domain mapping file
HAPROXY_CONFIG=/etc/haproxy/haproxy.cfg    # HAProxy config (auto-generated)
RECONCILE_INTERVAL=30s                     # Automation interval
```

### Optional: Firewall Automation

```bash
HETZNER_CLOUD_TOKEN=your_token             # Hetzner Cloud API token
HETZNER_FIREWALL_ID=your_firewall_id       # Firewall ID
```

## API

k8s-exposer provides a REST API for monitoring and management.

**Base URL:** `http://server:8090/api/v1`

### Endpoints

```bash
# System health
curl http://localhost:8090/api/v1/health

# System metrics
curl http://localhost:8090/api/v1/metrics

# List services
curl http://localhost:8090/api/v1/services

# Get service details
curl http://localhost:8090/api/v1/services/nginx-test

# Force reconciliation
curl -X POST http://localhost:8090/api/v1/sync
```

See [API Documentation](api-documentation.md) for complete reference.

## CLI Tool

k8s-exposer includes a professional CLI for management and monitoring.

### Installation

```bash
# Build from source
CGO_ENABLED=0 go build -o k8s-exposer ./cmd/cli

# Install to system
sudo mv k8s-exposer /usr/local/bin/
```

### Usage

```bash
# Show system status
k8s-exposer --server http://your-server:8090 status

# List exposed services
k8s-exposer services

# Get service details
k8s-exposer services get nginx-test

# Show system metrics
k8s-exposer metrics

# Force reconciliation
k8s-exposer sync

# Version info
k8s-exposer version

# JSON output for scripting
k8s-exposer --json services
```

See [CLI Documentation](CLI.md) for complete reference.

## Requirements

- **Kubernetes 1.20+**
- **WireGuard** configured between edge node and cluster
- **HAProxy 2.0+** on edge node
- **Go 1.21+** (for building from source)

## Development

### Build

```bash
# Build agent
CGO_ENABLED=0 go build -o k8s-exposer-agent ./cmd/agent

# Build server
CGO_ENABLED=0 go build -o k8s-exposer-server ./cmd/server
```

### Test

```bash
go test ./...
```

## License

MIT

## Author

Noah Jeana - [github.com/noahjeana](https://github.com/noahjeana)
