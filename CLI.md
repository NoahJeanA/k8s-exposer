# k8s-exposer CLI Documentation

## Overview

The k8s-exposer CLI is a command-line tool for managing and monitoring your k8s-exposer deployment.

## Installation

```bash
# Build the CLI
cd /path/to/k8s-exposer
CGO_ENABLED=0 go build -o k8s-exposer ./cmd/cli

# Install (optional)
sudo mv k8s-exposer /usr/local/bin/
```

### Shell Completion

The CLI includes auto-completion support for bash, zsh, fish, and PowerShell.

**Bash (temporary):**
```bash
source <(k8s-exposer completion bash)
```

**Bash (permanent):**
```bash
k8s-exposer completion bash > /etc/bash_completion.d/k8s-exposer
```

**Zsh (permanent):**
```bash
k8s-exposer completion zsh > /usr/local/share/zsh/site-functions/_k8s-exposer
```

**Fish (permanent):**
```bash
k8s-exposer completion fish > ~/.config/fish/completions/k8s-exposer.fish
```

After installation, start a new shell and test with TAB:
```bash
k8s-exposer [TAB]       # Shows all commands
k8s-exposer s[TAB]      # Shows services, status, sync
k8s-exposer --[TAB]     # Shows all flags
```

## Configuration

### Server URL

The CLI needs to know where your k8s-exposer server is running. You can specify this in three ways:

1. **Flag** (highest priority):
   ```bash
   k8s-exposer --server http://your-server:8090 status
   ```

2. **Environment variable**:
   ```bash
   export K8S_EXPOSER_SERVER="http://your-server:8090"
   k8s-exposer status
   ```

3. **Default**: `http://localhost:8090`

### Output Format

All commands support JSON output via the `--json` flag:

```bash
k8s-exposer --json services list
```

## Commands

### Global Flags

- `--server <url>` - k8s-exposer server URL (default: http://localhost:8090)
- `--json` - Output as JSON instead of formatted text
- `-h, --help` - Show help

### `k8s-exposer services`

List and inspect exposed services.

**Subcommands:**
- `list` - List all exposed services (default)
- `get <name>` - Get details for a specific service

**Examples:**

```bash
# List all services
k8s-exposer services
k8s-exposer services list

# Get details for a specific service
k8s-exposer services get nginx-test

# JSON output
k8s-exposer --json services
```

**Output:**

```
NAME         NAMESPACE    SUBDOMAIN         TARGET IP      PORTS
─────────────────────────────────────────────────────────────────────────
nginx-test   default      test              10.42.0.15     8080→80/tcp
hello-world  default      hello             10.42.0.16     8080→8080/tcp

Total: 2 services
```

### `k8s-exposer status`

Show system status and health.

**Examples:**

```bash
# Show status
k8s-exposer status

# JSON output
k8s-exposer --json status
```

**Output:**

```
=== k8s-exposer Status ===

Status: healthy
Version: 1.0.0
Services: 3

=== Metrics ===
Total Services: 3
Total Ports: 3

Memory:
  Allocated: 3.2 MB
  System: 12.5 MB
  GC runs: 5

Runtime:
  Goroutines: 12
  Go Version: go1.21.5
```

### `k8s-exposer metrics`

Show detailed system metrics.

**Examples:**

```bash
# Show metrics
k8s-exposer metrics

# JSON output
k8s-exposer --json metrics
```

**Output:**

```
=== System Metrics ===

Services:
  Total: 3
  Total Ports: 3

Memory:
  Allocated: 3.2 MB
  Total Allocated: 15.7 MB
  System: 12.5 MB
  GC Runs: 5

Runtime:
  Goroutines: 12
  Go Version: go1.21.5
```

### `k8s-exposer sync`

Force immediate reconciliation of HAProxy and firewall rules.

**Examples:**

```bash
# Trigger sync
k8s-exposer sync
```

**Output:**

```
✓ Reconciliation triggered successfully
```

### `k8s-exposer version`

Show CLI version information.

**Examples:**

```bash
k8s-exposer version
```

**Output:**

```
k8s-exposer CLI
Version: 1.0.0
Commit: dev
Built: unknown
```

## Exit Codes

- `0` - Success
- `1` - Error (connection failed, service not found, etc.)

## Examples

### Monitor system health

```bash
# Quick status check
k8s-exposer status

# Get detailed metrics
k8s-exposer metrics

# Check if specific service is running
k8s-exposer services get my-service
```

### Trigger reconciliation

```bash
# After manually editing HAProxy config or firewall rules
k8s-exposer sync
```

### JSON output for scripting

```bash
# Get all services as JSON
k8s-exposer --json services > services.json

# Check if system is healthy
if k8s-exposer --json status | jq -e '.health.status == "healthy"'; then
  echo "System is healthy"
fi

# Count services
k8s-exposer --json services | jq '. | length'
```

### Remote server

```bash
# Connect to remote server
k8s-exposer --server http://49.12.191.184:8090 status

# Or use environment variable
export K8S_EXPOSER_SERVER="http://49.12.191.184:8090"
k8s-exposer status
k8s-exposer services
```

## Troubleshooting

### Connection refused

**Error:**
```
Error: failed to get health: request failed: dial tcp :8090: connect: connection refused
```

**Solution:**
1. Check if server is running: `systemctl status k8s-exposer-server`
2. Verify server URL: `--server http://correct-host:8090`
3. Check firewall rules

### Timeout

**Error:**
```
Error: context deadline exceeded
```

**Solution:**
1. Check network connectivity to server
2. Verify server is responding: `curl http://server:8090/api/v1/health`
3. Check firewall allows port 8090

### Service not found

**Error:**
```
Error: failed to get service: service not found
```

**Solution:**
1. List all services: `k8s-exposer services`
2. Check service name is correct (case-sensitive)
3. Verify service has proper annotations in Kubernetes

## API Endpoints Used

The CLI communicates with the k8s-exposer REST API:

- `GET /api/v1/health` - System health
- `GET /api/v1/metrics` - System metrics
- `GET /api/v1/services` - List services
- `GET /api/v1/services/{name}` - Get service details
- `POST /api/v1/sync` - Trigger reconciliation

For API documentation, see `api-documentation.md`.

## Development

### Building

```bash
# Development build
go build -o k8s-exposer ./cmd/cli

# Static build (for distribution)
CGO_ENABLED=0 go build -o k8s-exposer ./cmd/cli

# With version info
go build -ldflags="-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o k8s-exposer ./cmd/cli
```

### Testing

```bash
# Start a local server for testing
go run ./cmd/server

# In another terminal, test the CLI
./k8s-exposer status
./k8s-exposer services
./k8s-exposer sync
```

## Future Enhancements

Planned features:
- Config file support (`~/.k8s-exposer/config.yaml`)
- Service logs streaming (`k8s-exposer logs <service>`)
- HAProxy management (`k8s-exposer haproxy status/reload`)
- DNS management (`k8s-exposer dns sync/list`)
- Interactive mode (`k8s-exposer shell`)
- Auto-completion for bash/zsh
