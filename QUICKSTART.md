# Quick Start Guide

This guide will help you get k8s-exposer up and running in under 10 minutes.

## Prerequisites

- A Kubernetes cluster (local or remote)
- A VPS server (e.g., Hetzner) with public IP
- Wireguard VPN configured between server and K8s cluster
- Docker installed (for building agent image)
- kubectl configured to access your cluster

## Step 1: Setup Wireguard VPN

On your VPS server:

```bash
# Install Wireguard
apt install wireguard

# Configure Wireguard (adjust as needed)
cat > /etc/wireguard/wg0.conf <<EOF
[Interface]
Address = 10.0.0.1/24
PrivateKey = <SERVER_PRIVATE_KEY>
ListenPort = 51820

[Peer]
PublicKey = <K8S_PUBLIC_KEY>
AllowedIPs = 10.0.0.2/32, 10.96.0.0/12  # K8s pod and service CIDR
EOF

# Start Wireguard
systemctl enable wg-quick@wg0
systemctl start wg-quick@wg0
```

On your K8s cluster (adjust based on your setup):

```bash
# Configure Wireguard to connect to VPS
# AllowedIPs should include the VPS Wireguard IP
```

Test connectivity:

```bash
# From VPS
ping 10.0.0.2

# From K8s cluster
ping 10.0.0.1
```

## Step 2: Build and Deploy Server

On your VPS server:

```bash
# Clone repository (on your development machine)
git clone https://github.com/noahjeana/k8s-exposer.git
cd k8s-exposer

# Build server binary
make build-server

# Copy to VPS
scp build/k8s-exposer-server root@your-vps:/usr/local/bin/
scp deploy/systemd/k8s-exposer.service root@your-vps:/etc/systemd/system/

# SSH into VPS
ssh root@your-vps

# Create configuration
mkdir -p /etc/k8s-exposer
cat > /etc/k8s-exposer/.env <<EOF
EXPOSER_LISTEN_ADDR=10.0.0.1:9090
EXPOSER_LOG_LEVEL=INFO
EXPOSER_WIREGUARD_INTERFACE=wg0
EXPOSER_PORT_RANGE_START=30000
EXPOSER_PORT_RANGE_END=32767
EOF

# Create working directory
mkdir -p /var/lib/k8s-exposer

# Start service
systemctl daemon-reload
systemctl enable k8s-exposer
systemctl start k8s-exposer

# Check status
systemctl status k8s-exposer
journalctl -u k8s-exposer -f
```

## Step 3: Build and Deploy Agent

On your development machine:

```bash
cd k8s-exposer

# Update agent deployment with your server address
# Edit deploy/kubernetes/agent.yaml if needed
# Change SERVER_ADDR to your VPS Wireguard IP

# Build and push Docker image
# You need to push to a registry accessible by your K8s cluster
docker build -t your-registry/k8s-exposer-agent:latest -f Dockerfile.agent .
docker push your-registry/k8s-exposer-agent:latest

# Update the image in deploy/kubernetes/agent.yaml
# Then deploy
kubectl apply -f deploy/kubernetes/rbac.yaml
kubectl apply -f deploy/kubernetes/agent.yaml

# Check agent logs
kubectl logs -n kube-system -l app=k8s-exposer-agent -f
```

You should see logs indicating the agent connected to the server successfully.

## Step 4: Deploy a Test Service

```bash
# Deploy a simple nginx service
kubectl apply -f examples/nginx-test.yaml

# Check if the service is exposed
# Look at agent logs - should show service discovered
# Look at server logs - should show listener started on port 80
```

## Step 5: Configure DNS

Create a DNS A record pointing to your VPS public IP:

```
test.neverup.at → <VPS_PUBLIC_IP>
```

## Step 6: Test Access

```bash
# Wait for DNS propagation (1-5 minutes)
# Test access from internet
curl http://test.neverup.at

# You should see the nginx welcome page!
```

## Step 7: Expose Your Own Services

Now you can expose any Kubernetes service by adding annotations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-app
  annotations:
    expose.neverup.at/subdomain: "myapp"
    expose.neverup.at/ports: "8080/tcp"
spec:
  selector:
    app: my-app
  ports:
  - port: 8080
    targetPort: 8080
```

Don't forget to create the DNS record: `myapp.neverup.at → <VPS_IP>`

## Troubleshooting

### Agent can't connect to server

```bash
# From K8s cluster, test connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -- sh
# Inside the pod
ping 10.0.0.1
nc -zv 10.0.0.1 9090
```

### Service not being discovered

```bash
# Check if annotations are present
kubectl get svc -o yaml | grep expose.neverup.at

# Check agent logs
kubectl logs -n kube-system -l app=k8s-exposer-agent
```

### Traffic not flowing

```bash
# Check server logs
journalctl -u k8s-exposer -f

# Test from VPS
curl http://<SERVICE_CLUSTER_IP>:<PORT>

# Check firewall on VPS
iptables -L -n
```

## Next Steps

- Set up monitoring and alerting
- Configure multiple services
- Review security best practices
- Consider setting up automated DNS updates
- Explore advanced configurations

For more details, see the full [README.md](README.md).
