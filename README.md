# k8s-exposer

A Go-based service exposer that makes Kubernetes services accessible from the internet via a Hetzner server using Wireguard VPN and subdomain-based routing.

## Architecture

```
Internet → Hetzner Server (Public IP) → Wireguard VPN (10.0.0.0/24) → K8s Cluster (Home)
```

### Components

1. **k8s-exposer-server**: Binary running on Hetzner (10.0.0.1)
   - Receives service configurations from agent
   - Opens dynamic ports for each service
   - Forwards traffic over Wireguard to K8s services

2. **k8s-exposer-agent**: Deployment in K8s cluster
   - Watches K8s API for annotated services
   - Sends service updates to server
   - Maintains persistent connection with heartbeat

## Prerequisites

- Go 1.21 or later
- Kubernetes cluster with kubectl access
- Hetzner server (or any VPS) with public IP
- Wireguard VPN configured between server and K8s cluster
- Docker (for building container images)

## Installation

### 1. Build Binaries

```bash
# Clone the repository
git clone https://github.com/noahjeana/k8s-exposer.git
cd k8s-exposer

# Build both server and agent
make build

# Or build individually
make build-server
make build-agent
```

### 2. Deploy Server to Hetzner

```bash
# Copy environment configuration
cp configs/.env.example /etc/k8s-exposer/.env

# Edit the configuration
nano /etc/k8s-exposer/.env

# Create working directory
mkdir -p /var/lib/k8s-exposer

# Deploy using Makefile (requires SSH access configured)
make deploy-server

# Or manually
scp build/k8s-exposer-server root@hetzner:/usr/local/bin/
scp deploy/systemd/k8s-exposer.service root@hetzner:/etc/systemd/system/
ssh root@hetzner "systemctl daemon-reload && systemctl enable k8s-exposer && systemctl start k8s-exposer"
```

### 3. Deploy Agent to Kubernetes

```bash
# Deploy RBAC and agent
make deploy-agent

# Or manually
kubectl apply -f deploy/kubernetes/rbac.yaml
kubectl apply -f deploy/kubernetes/agent.yaml

# Check agent logs
kubectl logs -n kube-system -l app=k8s-exposer-agent -f
```

## Configuration

### Server Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `EXPOSER_LISTEN_ADDR` | `10.0.0.1:9090` | Address to listen for agent connections |
| `EXPOSER_LOG_LEVEL` | `INFO` | Log level (DEBUG, INFO, WARN, ERROR) |
| `EXPOSER_WIREGUARD_INTERFACE` | `wg0` | Wireguard interface name |
| `EXPOSER_PORT_RANGE_START` | `30000` | Start of dynamic port allocation range |
| `EXPOSER_PORT_RANGE_END` | `32767` | End of dynamic port allocation range |

### Agent Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_ADDR` | `10.0.0.1:9090` | Server address |
| `CLUSTER_DOMAIN` | `neverup.at` | Domain for services |
| `LOG_LEVEL` | `INFO` | Log level (DEBUG, INFO, WARN, ERROR) |
| `SYNC_INTERVAL` | `30s` | Full sync interval |
| `WATCH_NAMESPACES` | *(all)* | Comma-separated namespaces to watch |

## Usage

### Exposing a Kubernetes Service

To expose a Kubernetes service, add the following annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: minecraft
  namespace: default
  annotations:
    expose.neverup.at/subdomain: "minecraft"
    expose.neverup.at/ports: "25565/tcp,25565/udp"
spec:
  selector:
    app: minecraft
  ports:
  - name: game-tcp
    port: 25565
    targetPort: 25565
    protocol: TCP
  - name: game-udp
    port: 25565
    targetPort: 25565
    protocol: UDP
```

### Annotations

- **`expose.neverup.at/subdomain`** (required): Defines the subdomain for the service
  - Must be a valid DNS label (lowercase alphanumeric and hyphens)
  - Example: `minecraft`, `web-app`, `api`

- **`expose.neverup.at/ports`** (required): Defines ports to expose
  - Format: `port/protocol[,port/protocol...]`
  - Protocol can be: `tcp`, `udp`, or `tcp+udp`
  - Example: `80/tcp`, `25565/tcp,25565/udp`, `53/tcp+udp`

### DNS Configuration

After deploying a service, create a DNS A record pointing to your Hetzner server:

```
minecraft.neverup.at → <HETZNER_PUBLIC_IP>
```

## Example Services

### Simple HTTP Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx
  annotations:
    expose.neverup.at/subdomain: "web"
    expose.neverup.at/ports: "80/tcp"
spec:
  selector:
    app: nginx
  ports:
  - port: 80
    targetPort: 80
    protocol: TCP
```

### Game Server (TCP + UDP)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: valheim
  annotations:
    expose.neverup.at/subdomain: "valheim"
    expose.neverup.at/ports: "2456/tcp+udp,2457/tcp+udp,2458/tcp+udp"
spec:
  selector:
    app: valheim
  ports:
  - name: game1
    port: 2456
    protocol: TCP
  - name: game2
    port: 2457
    protocol: TCP
  - name: game3
    port: 2458
    protocol: TCP
```

## Troubleshooting

### Agent not connecting to server

1. Check agent logs:
   ```bash
   kubectl logs -n kube-system -l app=k8s-exposer-agent
   ```

2. Verify Wireguard connectivity:
   ```bash
   # From K8s cluster
   ping 10.0.0.1
   ```

3. Check server logs:
   ```bash
   journalctl -u k8s-exposer -f
   ```

### Services not being discovered

1. Check if annotations are correct:
   ```bash
   kubectl get svc -A -o yaml | grep "expose.neverup.at"
   ```

2. Verify RBAC permissions:
   ```bash
   kubectl auth can-i list services --as=system:serviceaccount:kube-system:k8s-exposer-agent
   ```

3. Check agent logs for errors

### Port conflicts

If a port is already in use, the server will automatically allocate an alternative port from the dynamic range (30000-32767). Check server logs for the allocated port.

### Traffic not reaching pods

1. Verify service ClusterIP is accessible from the server over Wireguard:
   ```bash
   # From Hetzner server
   curl http://<ClusterIP>:<port>
   ```

2. Check listener status in server logs

3. Verify firewall rules on Hetzner server

## Development

### Running Tests

```bash
make test
```

### Building Docker Images

```bash
# Build agent image
make docker-build

# Build with specific version
VERSION=v1.0.0 make docker-build

# Push to registry
make docker-push
```

### Code Formatting

```bash
make fmt
make vet
make lint
```

## Project Structure

```
k8s-exposer/
├── cmd/
│   ├── server/main.go        # Server entry point
│   └── agent/main.go         # Agent entry point
├── internal/
│   ├── protocol/             # Communication protocol
│   │   ├── messages.go       # Message encoding/decoding
│   │   └── connection.go     # Connection management
│   ├── server/               # Server components
│   │   ├── listener.go       # Port listeners
│   │   ├── forwarder.go      # Traffic forwarder
│   │   └── registry.go       # Service registry
│   └── agent/                # Agent components
│       ├── watcher.go        # K8s service watcher
│       ├── client.go         # Server client
│       └── discovery.go      # Service discovery
├── pkg/
│   └── types/service.go      # Shared types
├── deploy/
│   ├── kubernetes/           # K8s manifests
│   │   ├── agent.yaml
│   │   ├── rbac.yaml
│   │   └── configmap.yaml
│   └── systemd/              # Systemd service
│       └── k8s-exposer.service
├── configs/
│   └── .env.example          # Environment configuration
├── Dockerfile.agent          # Agent Dockerfile
├── Dockerfile.server         # Server Dockerfile
├── Makefile                  # Build automation
└── README.md                 # This file
```

## Security Considerations

- The agent has read-only access to Kubernetes services
- All communication is over a secure Wireguard VPN
- The server runs with minimal privileges (systemd hardening)
- Input validation on all annotations
- No sensitive data is logged

## Future Enhancements

- [ ] TLS support for agent-server communication
- [ ] Authentication/authorization for agents
- [ ] Metrics and monitoring (Prometheus)
- [ ] Rate limiting
- [ ] Health checks and readiness probes
- [ ] Support for multiple agents
- [ ] Web UI for management
- [ ] Automatic DNS record creation

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run `make lint` and `make test`
6. Submit a pull request

## License

MIT License - see LICENSE file for details

## Support

For issues, questions, or contributions, please open an issue on GitHub:
https://github.com/noahjeana/k8s-exposer/issues
