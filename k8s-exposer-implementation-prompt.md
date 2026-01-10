# K8s-Exposer Implementation Guide for Code Agents

## Project Overview
Build a Go-based service exposer that makes Kubernetes services accessible from the internet via a Hetzner server using Wireguard VPN and subdomain-based routing.

## Core Requirements
- **Language**: Go
- **Architecture**: Agent-Server model
- **Communication**: TCP/UDP over Wireguard VPN
- **Service Discovery**: Kubernetes annotations
- **Routing**: Subdomain-based (e.g., minecraft.neverup.at)

## Network Topology
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

## Implementation Steps

### Step 1: Project Initialization
```bash
# Initialize Go module
mkdir k8s-exposer && cd k8s-exposer
go mod init github.com/noahjeana/k8s-exposer

# Install dependencies
go get k8s.io/client-go@latest
go get k8s.io/api@latest
go get k8s.io/apimachinery@latest
```

### Step 2: Directory Structure
Create the following structure:
```
k8s-exposer/
├── cmd/
│   ├── server/main.go
│   └── agent/main.go
├── internal/
│   ├── protocol/
│   │   ├── messages.go
│   │   └── connection.go
│   ├── server/
│   │   ├── listener.go
│   │   ├── forwarder.go
│   │   └── registry.go
│   └── agent/
│       ├── watcher.go
│       ├── client.go
│       └── discovery.go
├── pkg/
│   └── types/service.go
├── deploy/
│   ├── kubernetes/
│   │   ├── agent.yaml
│   │   ├── rbac.yaml
│   │   └── configmap.yaml
│   └── systemd/k8s-exposer.service
├── configs/.env.example
├── Makefile
├── go.mod
└── README.md
```

### Step 3: Core Types (pkg/types/service.go)

**Task**: Implement shared types for service configuration

**File**: `pkg/types/service.go`

**Requirements**:
```go
package types

// ExposedService represents a Kubernetes service that should be exposed externally
type ExposedService struct {
    Name        string        `json:"name"`
    Namespace   string        `json:"namespace"`
    Subdomain   string        `json:"subdomain"`   // From annotation: expose.neverup.at/subdomain
    Ports       []PortMapping `json:"ports"`       // From annotation: expose.neverup.at/ports
    TargetIP    string        `json:"target_ip"`   // K8s ClusterIP
    NodeIP      string        `json:"node_ip"`     // For NodePort fallback
}

// PortMapping defines a port and protocol to expose
type PortMapping struct {
    Port     int32  `json:"port"`
    Protocol string `json:"protocol"` // "tcp", "udp", or "tcp+udp"
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
```

**Validation Requirements**:
- Add `Validate()` methods for each type
- Subdomain must be valid DNS label
- Port must be between 1-65535
- Protocol must be "tcp", "udp", or "tcp+udp"

### Step 4: Protocol Implementation (internal/protocol/)

**Task**: Implement connection and message handling

**File**: `internal/protocol/connection.go`

**Requirements**:
- Implement persistent TCP connection with reconnection logic
- Add connection pooling
- Implement heartbeat mechanism (30s interval)
- Handle connection errors gracefully
- Support TLS optional (future-proof)

**File**: `internal/protocol/messages.go`

**Requirements**:
- JSON encoding/decoding for messages
- Message framing with length prefix
- Error handling for malformed messages
- Message validation before processing

**Key Functions**:
```go
func NewConnection(addr string) (*Connection, error)
func (c *Connection) Send(msg *types.Message) error
func (c *Connection) Receive() (*types.Message, error)
func (c *Connection) Close() error
func (c *Connection) Reconnect() error
```

### Step 5: Agent - Service Watcher (internal/agent/watcher.go)

**Task**: Watch Kubernetes services with specific annotations

**Requirements**:
- Use Kubernetes informers for efficient watching
- Filter services with annotation `expose.neverup.at/subdomain`
- Parse annotation `expose.neverup.at/ports` (format: "25565/tcp,25565/udp")
- Trigger callback on service add/update/delete
- Handle errors without crashing

**Annotations to watch**:
- `expose.neverup.at/subdomain`: Required, defines subdomain
- `expose.neverup.at/ports`: Required, defines ports (comma-separated list)

**Key Functions**:
```go
func NewServiceWatcher(clientset kubernetes.Interface, onChange func([]types.ExposedService)) *ServiceWatcher
func (w *ServiceWatcher) Start(ctx context.Context) error
func (w *ServiceWatcher) parseServiceAnnotations(svc *corev1.Service) (*types.ExposedService, error)
```

**Edge Cases**:
- Service without required annotations (skip)
- Invalid port format (log error, skip)
- Duplicate subdomains (log warning, use first)

### Step 6: Agent - Discovery Logic (internal/agent/discovery.go)

**Task**: Extract service information from Kubernetes API

**Requirements**:
- Get ClusterIP from service spec
- Detect NodePort if available
- Parse port definitions
- Handle multi-port services
- Support LoadBalancer services

**Key Functions**:
```go
func DiscoverServices(clientset kubernetes.Interface) ([]types.ExposedService, error)
func extractServiceInfo(svc *corev1.Service) (*types.ExposedService, error)
func parsePorts(portsAnnotation string) ([]types.PortMapping, error)
```

### Step 7: Agent - Server Client (internal/agent/client.go)

**Task**: Maintain connection to server and send updates

**Requirements**:
- Connect to server via Wireguard network (10.0.0.1:9090)
- Reconnect automatically on connection loss
- Send full service list on connect
- Send incremental updates on changes
- Send heartbeat every 30s
- Handle backpressure

**Key Functions**:
```go
func NewServerClient(serverAddr string) *ServerClient
func (c *ServerClient) Connect(ctx context.Context) error
func (c *ServerClient) SendUpdate(services []types.ExposedService) error
func (c *ServerClient) SendHeartbeat() error
func (c *ServerClient) Close() error
```

### Step 8: Agent - Main Entry Point (cmd/agent/main.go)

**Task**: Wire everything together for the agent

**Requirements**:
- Parse environment variables:
  - `SERVER_ADDR`: Server address (default: "10.0.0.1:9090")
  - `CLUSTER_DOMAIN`: Domain for services (default: "neverup.at")
  - `WATCH_NAMESPACES`: Comma-separated namespaces (default: all)
  - `SYNC_INTERVAL`: Sync interval (default: "30s")
  - `LOG_LEVEL`: Log level (default: "INFO")
- Initialize Kubernetes client (in-cluster config)
- Start service watcher
- Connect to server
- Handle graceful shutdown (SIGINT, SIGTERM)
- Implement structured logging

**Main Loop**:
1. Initialize K8s client
2. Start service watcher
3. Connect to server
4. On service change: send update
5. Periodic full sync (every SYNC_INTERVAL)
6. Handle errors and reconnect

### Step 9: Server - Service Registry (internal/server/registry.go)

**Task**: Maintain registry of exposed services and their listeners

**Requirements**:
- Thread-safe service registry
- Map subdomain to service configuration
- Track active listeners per port/protocol
- Detect and handle port conflicts
- Support dynamic port allocation (30000-32767 range)
- Implement service reconciliation (add/update/delete)

**Key Functions**:
```go
func NewServiceRegistry() *ServiceRegistry
func (r *ServiceRegistry) Update(services []types.ExposedService) error
func (r *ServiceRegistry) AllocatePort(port int32, protocol string) (int32, error)
func (r *ServiceRegistry) IsPortAvailable(port int32, protocol string) bool
func (r *ServiceRegistry) GetService(subdomain string) (*types.ExposedService, bool)
func (r *ServiceRegistry) RemoveService(subdomain string) error
```

**Port Allocation Strategy**:
1. Try to use service's original port if available
2. If conflict, allocate from high range (30000-32767)
3. Track allocated ports per protocol (TCP/UDP separate)

### Step 10: Server - Port Listener (internal/server/listener.go)

**Task**: Dynamic port listeners for TCP and UDP

**Requirements**:
- Support both TCP and UDP listeners
- Accept connections and forward to target
- Handle connection errors gracefully
- Support graceful shutdown
- Log connection statistics
- Implement connection pooling for efficiency

**Key Structures**:
```go
type PortListener struct {
    port     int32
    protocol string
    listener net.Listener    // for TCP
    conn     *net.UDPConn    // for UDP
    target   types.ExposedService
    forwarder *Forwarder
    stopCh   chan struct{}
}
```

**Key Functions**:
```go
func NewPortListener(port int32, protocol string, target types.ExposedService) *PortListener
func (pl *PortListener) Start() error
func (pl *PortListener) Stop() error
func (pl *PortListener) handleTCPConnection(conn net.Conn)
func (pl *PortListener) handleUDPPacket(data []byte, addr *net.UDPAddr)
```

**Special Handling**:
- TCP: Create new goroutine per connection
- UDP: Implement connection tracking (pseudo-sessions)
- Both: Timeout inactive connections

### Step 11: Server - Traffic Forwarder (internal/server/forwarder.go)

**Task**: Forward network traffic through Wireguard to K8s services

**Requirements**:
- Bidirectional TCP forwarding with io.Copy
- UDP packet forwarding with connection tracking
- Handle connection timeouts
- Support multiple concurrent connections
- Bind to Wireguard interface (wg0)
- Implement metrics (bytes transferred, connection count)

**Key Functions**:
```go
func NewForwarder(wireguardInterface string) *Forwarder
func (f *Forwarder) ForwardTCP(client net.Conn, targetIP string, targetPort int32) error
func (f *Forwarder) ForwardUDP(clientAddr *net.UDPAddr, data []byte, targetIP string, targetPort int32) error
func (f *Forwarder) dialViaWireguard(network, address string) (net.Conn, error)
```

**TCP Forwarding**:
```go
func (f *Forwarder) ForwardTCP(client net.Conn, targetIP string, targetPort int32) error {
    // 1. Dial target via Wireguard interface
    target, err := f.dialViaWireguard("tcp", fmt.Sprintf("%s:%d", targetIP, targetPort))
    if err != nil {
        return err
    }
    defer target.Close()
    
    // 2. Bidirectional copy
    errCh := make(chan error, 2)
    go func() {
        _, err := io.Copy(target, client)
        errCh <- err
    }()
    go func() {
        _, err := io.Copy(client, target)
        errCh <- err
    }()
    
    // 3. Wait for first error or completion
    return <-errCh
}
```

**UDP Forwarding**:
- Maintain session table (client addr → target conn)
- TTL-based session cleanup (5 min idle)
- NAT traversal support

### Step 12: Server - Main Entry Point (cmd/server/main.go)

**Task**: Wire everything together for the server

**Requirements**:
- Parse environment variables:
  - `EXPOSER_LISTEN_ADDR`: Address for agent connections (default: "10.0.0.1:9090")
  - `EXPOSER_LOG_LEVEL`: Log level (default: "INFO")
  - `EXPOSER_WIREGUARD_INTERFACE`: Wireguard interface name (default: "wg0")
  - `EXPOSER_PORT_RANGE_START`: Start of dynamic port range (default: 30000)
  - `EXPOSER_PORT_RANGE_END`: End of dynamic port range (default: 32767)
- Initialize service registry
- Start agent listener (accept connections)
- Process incoming messages
- Handle graceful shutdown
- Implement structured logging

**Main Loop**:
1. Start listening for agent connections
2. Accept agent connection
3. Authenticate agent (future: add auth)
4. Receive messages in goroutine
5. Process service updates
6. Update registry and listeners
7. Handle agent disconnect

### Step 13: Kubernetes Manifests (deploy/kubernetes/)

**Task**: Create K8s deployment resources

**File**: `deploy/kubernetes/rbac.yaml`
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: k8s-exposer-agent
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: k8s-exposer-agent
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: k8s-exposer-agent
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: k8s-exposer-agent
subjects:
- kind: ServiceAccount
  name: k8s-exposer-agent
  namespace: kube-system
```

**File**: `deploy/kubernetes/agent.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: k8s-exposer-agent
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: k8s-exposer-agent
  template:
    metadata:
      labels:
        app: k8s-exposer-agent
    spec:
      serviceAccountName: k8s-exposer-agent
      containers:
      - name: agent
        image: ghcr.io/noahjeana/k8s-exposer-agent:latest
        imagePullPolicy: Always
        env:
        - name: SERVER_ADDR
          value: "10.0.0.1:9090"
        - name: CLUSTER_DOMAIN
          value: "neverup.at"
        - name: LOG_LEVEL
          value: "INFO"
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
```

**File**: `deploy/kubernetes/configmap.yaml`
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: k8s-exposer-agent-config
  namespace: kube-system
data:
  config.yaml: |
    server_addr: "10.0.0.1:9090"
    cluster_domain: "neverup.at"
    sync_interval: "30s"
    watch_namespaces: []  # empty = all namespaces
```

### Step 14: Systemd Service (deploy/systemd/k8s-exposer.service)

**Task**: Create systemd service for server

```ini
[Unit]
Description=K8s Service Exposer
After=network.target wg-quick@wg0.service
Requires=wg-quick@wg0.service
Documentation=https://github.com/noahjeana/k8s-exposer

[Service]
Type=simple
User=root
WorkingDirectory=/var/lib/k8s-exposer
EnvironmentFile=/etc/k8s-exposer/.env
ExecStart=/usr/local/bin/k8s-exposer-server
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/k8s-exposer

[Install]
WantedBy=multi-user.target
```

### Step 15: Makefile

**Task**: Create build and deployment automation

```makefile
.PHONY: build build-server build-agent clean test deploy-server deploy-agent docker-build

BINARY_SERVER=k8s-exposer-server
BINARY_AGENT=k8s-exposer-agent
BUILD_DIR=build
DOCKER_REGISTRY=ghcr.io/noahjeana

build: build-server build-agent

build-server:
	@echo "Building server..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_SERVER) ./cmd/server

build-agent:
	@echo "Building agent..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY_AGENT) ./cmd/agent

clean:
	@rm -rf $(BUILD_DIR)

test:
	@go test -v -race ./...

deploy-server: build-server
	@echo "Deploying server to Hetzner..."
	@scp $(BUILD_DIR)/$(BINARY_SERVER) root@hetzner:/usr/local/bin/
	@scp deploy/systemd/k8s-exposer.service root@hetzner:/etc/systemd/system/
	@ssh root@hetzner "systemctl daemon-reload && systemctl restart k8s-exposer"

deploy-agent: build-agent docker-build
	@echo "Deploying agent to K8s..."
	@kubectl apply -f deploy/kubernetes/rbac.yaml
	@kubectl apply -f deploy/kubernetes/agent.yaml

docker-build:
	@docker build -t $(DOCKER_REGISTRY)/k8s-exposer-agent:latest -f Dockerfile.agent .
	@docker push $(DOCKER_REGISTRY)/k8s-exposer-agent:latest
```

### Step 16: Configuration Files

**File**: `configs/.env.example`
```bash
# Server Configuration
EXPOSER_LISTEN_ADDR=10.0.0.1:9090
EXPOSER_LOG_LEVEL=INFO
EXPOSER_WIREGUARD_INTERFACE=wg0

# Port Range for dynamic allocation
EXPOSER_PORT_RANGE_START=30000
EXPOSER_PORT_RANGE_END=32767

# Optional: TLS
# EXPOSER_TLS_CERT=/etc/k8s-exposer/tls.crt
# EXPOSER_TLS_KEY=/etc/k8s-exposer/tls.key
```

### Step 17: Dockerfiles

**File**: `Dockerfile.agent`
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o agent ./cmd/agent

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/agent .
USER nobody
ENTRYPOINT ["./agent"]
```

### Step 18: README.md

**Task**: Create comprehensive documentation

**Required Sections**:
1. Overview and architecture diagram
2. Prerequisites (Go, K8s, Wireguard)
3. Installation instructions
4. Configuration reference
5. Usage examples
6. Troubleshooting guide
7. Contributing guidelines

**Example Service Usage**:
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

### Step 19: Testing Strategy

**Unit Tests Required**:
1. Port parsing from annotations
2. Service validation
3. Port allocation logic
4. Message encoding/decoding
5. Connection management

**Integration Tests Required**:
1. Agent → Server communication
2. K8s service discovery
3. TCP forwarding
4. UDP forwarding
5. Reconnection logic

**Test Files**:
- `pkg/types/service_test.go`
- `internal/protocol/messages_test.go`
- `internal/agent/watcher_test.go`
- `internal/server/registry_test.go`
- `internal/server/forwarder_test.go`

### Step 20: Logging Standards

**Requirements**:
- Use structured logging (e.g., `log/slog` or `logrus`)
- Log levels: DEBUG, INFO, WARN, ERROR
- Include context in logs (service name, subdomain, port)
- Avoid logging sensitive data

**Standard Log Fields**:
- `component`: "agent" or "server"
- `subdomain`: Service subdomain
- `service`: Service name/namespace
- `port`: Port number
- `protocol`: "tcp" or "udp"
- `action`: "start", "stop", "forward", "error"

**Example**:
```go
log.Info("Starting TCP listener",
    "component", "server",
    "subdomain", service.Subdomain,
    "port", port,
    "protocol", "tcp",
    "target", fmt.Sprintf("%s:%d", service.TargetIP, port))
```

## Critical Implementation Notes

### Error Handling
- Never panic in production code
- Always wrap errors with context
- Log errors before returning
- Implement exponential backoff for retries

### Performance Considerations
- Use connection pooling for high-traffic services
- Implement buffering for UDP packets
- Use goroutines efficiently (don't leak)
- Close resources properly (defer Close())

### Security Best Practices
- Validate all input from network
- Sanitize annotations before parsing
- Implement rate limiting (future enhancement)
- Use secure defaults

### Edge Cases to Handle
1. Agent loses connection: Automatic reconnection
2. Port conflict: Dynamic allocation to high range
3. Service deleted: Stop listeners, remove from registry
4. Invalid annotations: Skip service, log error
5. Wireguard interface down: Retry with backoff
6. UDP connection tracking: TTL-based cleanup
7. Duplicate subdomains: First-come-first-served

## Validation Checklist

Before considering implementation complete:

- [ ] All files compile without errors
- [ ] Unit tests pass
- [ ] Integration tests with real K8s cluster
- [ ] Server can forward TCP traffic
- [ ] Server can forward UDP traffic
- [ ] Agent discovers annotated services
- [ ] Agent reconnects after connection loss
- [ ] Port conflicts are handled gracefully
- [ ] Services can be added/removed dynamically
- [ ] Graceful shutdown works for both components
- [ ] Logging is comprehensive and structured
- [ ] README with complete examples
- [ ] Deployment manifests are valid
- [ ] Makefile builds both binaries
- [ ] Docker image builds successfully

## Deployment Order

1. Build both binaries
2. Deploy server to Hetzner
3. Configure environment variables
4. Start systemd service
5. Deploy RBAC to K8s
6. Deploy agent to K8s
7. Annotate test service
8. Verify connectivity
9. Create DNS records
10. Test from internet

## Example Test Service

Create this service to test the implementation:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nginx-test
  namespace: default
  annotations:
    expose.neverup.at/subdomain: "test"
    expose.neverup.at/ports: "80/tcp"
spec:
  selector:
    app: nginx
  ports:
  - port: 80
    targetPort: 80
    protocol: TCP
---
apiVersion: v1
kind: Pod
metadata:
  name: nginx-test
  namespace: default
  labels:
    app: nginx
spec:
  containers:
  - name: nginx
    image: nginx:latest
    ports:
    - containerPort: 80
```

After deployment:
1. Check agent logs: Should see service discovered
2. Check server logs: Should see listener started on port 80
3. Create DNS: test.neverup.at → HETZNER_IP
4. Test: `curl http://test.neverup.at` should return nginx page

## Success Criteria

The implementation is successful when:
1. Annotated services are automatically discovered
2. Traffic reaches K8s pods from internet
3. Both TCP and UDP protocols work
4. Agent reconnects automatically
5. Multiple services can be exposed simultaneously
6. No memory leaks during long-running operation
7. Logs provide clear troubleshooting information
