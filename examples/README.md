# k8s-exposer Examples

This directory contains example Kubernetes manifests for testing k8s-exposer.

## Quick Start

### 1. Deploy Example Service

```bash
kubectl apply -f whoami-demo.yaml
```

### 2. Verify Service is Discovered

```bash
# Check k8s-exposer agent logs
kubectl logs -n kube-system deployment/k8s-exposer-agent

# Check server (on edge node)
k8s-exposer --server http://localhost:8090 services
```

### 3. Add DNS Record

Add an A record for your subdomain:
```
whoami.neverup.at  A  49.12.191.184
```

### 4. Test Access

```bash
curl http://whoami.neverup.at/

# Should return something like:
# Hostname: whoami-xxxx
# IP: 10.42.x.x
# RemoteAddr: x.x.x.x:xxxxx
# GET / HTTP/1.1
# Host: whoami.neverup.at
# ...
```

## Available Examples

### whoami-demo.yaml

A simple HTTP service that returns request information.

**Features:**
- Uses traefik/whoami image
- Shows hostname, IP, headers
- Perfect for testing routing

**Exposed as:**
- Subdomain: `whoami.neverup.at`
- Port: 8083 → 80

**Annotations:**
```yaml
expose.neverup.at/subdomain: "whoami"
expose.neverup.at/ports: "8083/tcp"
```

## Creating Your Own Service

To expose your own service:

1. **Ensure it's a ClusterIP service** (or LoadBalancer/NodePort):
   ```yaml
   spec:
     type: ClusterIP
   ```

2. **Add k8s-exposer annotations**:
   ```yaml
   metadata:
     annotations:
       expose.neverup.at/subdomain: "myapp"
       expose.neverup.at/ports: "8084/tcp"
   ```

3. **Deploy**:
   ```bash
   kubectl apply -f your-service.yaml
   ```

4. **Add DNS record**:
   ```
   myapp.neverup.at  A  <your-edge-server-ip>
   ```

5. **Verify**:
   ```bash
   k8s-exposer services get myapp
   curl http://myapp.neverup.at/
   ```

## Port Assignment

Each service needs a unique port on the edge server:

- 8080 - nginx-test
- 8081 - vaultwarden
- 8082 - hello-world
- 8083 - whoami (example)
- 8084+ - Your services

Choose an unused port for each new service.

## Troubleshooting

### Service not discovered

```bash
# Check agent logs
kubectl logs -n kube-system deployment/k8s-exposer-agent

# Verify annotations
kubectl get svc whoami -o yaml | grep expose.neverup.at
```

### Service not accessible

```bash
# Check if service is registered
k8s-exposer services

# Check HAProxy backend
ssh root@<edge-server> cat /etc/haproxy/domains.map

# Test from edge server
ssh root@<edge-server> curl http://localhost:8083/
```

### DNS issues

```bash
# Verify DNS record
dig whoami.neverup.at +short

# Should return your edge server IP
```

## Cleanup

```bash
kubectl delete -f whoami-demo.yaml
```

The service will be automatically removed from k8s-exposer within 30 seconds (next reconciliation cycle).
