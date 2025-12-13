# Node Cleanup Webhook Makefile

IMAGE_REGISTRY ?= registry.example.com
IMAGE_NAME ?= node-cleanup-webhook
IMAGE_TAG ?= latest
FULL_IMAGE = $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

NAMESPACE = node-cleanup-system

.PHONY: help build push deploy undeploy test clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## Build

build: ## Build the container image
	podman build -t $(FULL_IMAGE) .

push: build ## Build and push the image
	podman push $(FULL_IMAGE)

build-local: ## Build binary locally
	go build -o bin/webhook ./cmd/webhook

## Deploy

deploy: ## Deploy with Helm
	helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--set image.repository=$(IMAGE_REGISTRY)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG)

deploy-manifests: ## Deploy with raw manifests
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f deploy/manifests/rbac.yaml
	kubectl apply -f deploy/manifests/webhook-config.yaml
	kubectl apply -f deploy/manifests/deployment.yaml

upgrade: ## Upgrade Helm release
	helm upgrade node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
		--namespace $(NAMESPACE) \
		--set image.repository=$(IMAGE_REGISTRY)/$(IMAGE_NAME) \
		--set image.tag=$(IMAGE_TAG)

undeploy: ## Remove from cluster
	helm uninstall node-cleanup-webhook -n $(NAMESPACE) || true
	kubectl delete -f deploy/manifests/ --ignore-not-found || true

## Development

run-local: ## Run locally (requires kubeconfig and certs)
	@mkdir -p certs
	@if [ ! -f certs/tls.crt ] || [ ! -f certs/tls.key ]; then \
		echo "Generating self-signed certificates..."; \
		openssl req -x509 -newkey rsa:2048 -nodes \
			-keyout certs/tls.key -out certs/tls.crt \
			-days 365 -subj "/CN=localhost" 2>/dev/null; \
	fi
	go run ./cmd/webhook \
		--kubeconfig=$(HOME)/.kube/config \
		--tls-cert=certs/tls.crt \
		--tls-key=certs/tls.key \
		--port=8443 \
		-v=2

test: ## Run tests
	go test -v ./...

fmt: ## Format code
	go fmt ./...

vet: ## Run go vet (static analysis)
	go vet ./...

check: fmt vet test ## Run all checks (fmt, vet, test)

tidy: ## Tidy go.mod
	go mod tidy

vendor: ## Vendor dependencies for air-gapped environments
	go mod vendor

release-bundle: vendor ## Create air-gapped release bundle with vendor/
	@echo "Creating air-gapped release bundle..."
	@VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo "dev"); \
	BUNDLE_NAME="node-cleanup-webhook-$$VERSION-airgap"; \
	mkdir -p releases; \
	tar -czf releases/$$BUNDLE_NAME.tar.gz \
		--exclude='.git' \
		--exclude='releases' \
		--exclude='bin' \
		--exclude='certs' \
		.; \
	echo "✅ Created releases/$$BUNDLE_NAME.tar.gz"; \
	ls -lh releases/$$BUNDLE_NAME.tar.gz

## Debugging

logs: ## Show webhook logs
	kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/name=node-cleanup-webhook -f

describe: ## Describe webhook pods
	kubectl describe pod -n $(NAMESPACE) -l app.kubernetes.io/name=node-cleanup-webhook

status: ## Show deployment status
	@echo "=== Deployment ==="
	kubectl get deployment -n $(NAMESPACE)
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -n $(NAMESPACE) -o wide
	@echo ""
	@echo "=== Webhook Config ==="
	kubectl get mutatingwebhookconfiguration node-cleanup-webhook -o yaml | grep -A5 "webhooks:" || echo "Webhook config not found"

test-webhook: ## Test webhook by creating a dummy node (dry-run)
	@echo "Testing webhook with dry-run node creation..."
	@kubectl create -f - --dry-run=server -o yaml <<< '{"apiVersion":"v1","kind":"Node","metadata":{"name":"test-webhook-node"}}'

## Cleanup

clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf certs/
	go clean -cache

clean-cluster: ## Remove webhook and all resources from cluster
	kubectl delete mutatingwebhookconfiguration node-cleanup-webhook --ignore-not-found
	kubectl delete namespace $(NAMESPACE) --ignore-not-found
