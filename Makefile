.PHONY: build build-server build-agent clean test deploy-server deploy-agent docker-build docker-push

BINARY_SERVER=k8s-exposer-server
BINARY_AGENT=k8s-exposer-agent
BUILD_DIR=build
DOCKER_REGISTRY=ghcr.io/noahjeana
VERSION?=latest

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

deploy-agent: docker-build docker-push
	@echo "Deploying agent to K8s..."
	@kubectl apply -f deploy/kubernetes/rbac.yaml
	@kubectl apply -f deploy/kubernetes/agent.yaml

docker-build:
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_REGISTRY)/$(BINARY_AGENT):$(VERSION) -f Dockerfile.agent .

docker-push:
	@echo "Pushing Docker image..."
	@docker push $(DOCKER_REGISTRY)/$(BINARY_AGENT):$(VERSION)

fmt:
	@go fmt ./...

vet:
	@go vet ./...

lint: fmt vet
	@echo "Linting complete"
