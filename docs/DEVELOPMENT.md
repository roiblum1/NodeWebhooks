# Development Guide

This guide covers setting up your development environment and contributing to the project.

## Prerequisites

- Go 1.21 or later
- Docker or Podman
- kubectl
- A Kubernetes cluster (minikube, kind, or remote)
- (Optional) golangci-lint for linting
- (Optional) Helm 3+ for chart development

## Setting Up Development Environment

### 1. Clone the Repository

```bash
git clone https://github.com/894/node-cleanup-webhook.git
cd node-cleanup-webhook
```

### 2. Install Dependencies

```bash
go mod download
```

### 3. Run Locally

```bash
# Automatically generates self-signed certs if needed
make run-local
```

This will:
- Generate self-signed TLS certificates in `certs/`
- Start the webhook on port 8443
- Connect to your cluster using `~/.kube/config`

### 4. Test in Cluster

```bash
# Create a test node
kubectl create -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  name: dev-test-node
spec:
  podCIDR: 10.244.1.0/24
EOF

# Verify finalizer was added
kubectl get node dev-test-node -o jsonpath='{.metadata.finalizers}'

# Delete and watch cleanup
kubectl delete node dev-test-node
```

## Project Structure

```
.
├── cmd/
│   └── webhook/
│       └── main.go              # Entry point - starts webhook and watcher
├── pkg/
│   ├── webhook/
│   │   └── server.go           # Admission webhook handler
│   └── watcher/
│       └── watcher.go          # Node cleanup watcher
├── deploy/
│   ├── helm/                    # Helm chart
│   └── manifests/               # Raw Kubernetes manifests
├── docs/                        # Documentation
├── scripts/                     # Utility scripts
├── .github/workflows/           # CI/CD pipelines
├── Dockerfile                   # Multi-stage build
├── Makefile                     # Build automation
└── go.mod                       # Go dependencies
```

## Making Changes

### Code Style

- Follow standard Go conventions
- Run `go fmt ./...` before committing
- Use meaningful variable and function names
- Add comments for exported functions

### Adding Cleanup Logic

1. Open [`pkg/watcher/watcher.go`](../pkg/watcher/watcher.go)
2. Locate the `runCleanup()` function
3. Add your cleanup logic:

```go
func (w *Watcher) runCleanup(ctx context.Context, node *corev1.Node) error {
    klog.Infof("Running cleanup for node: %s", node.Name)

    // Your cleanup logic here
    if err := myCleanupFunction(ctx, node); err != nil {
        return fmt.Errorf("cleanup failed: %w", err)
    }

    return nil
}
```

**Important**: Cleanup functions must be idempotent (safe to run multiple times).

### Adding Tests

Create test files with `_test.go` suffix:

```go
package watcher

import (
    "testing"
)

func TestContainsFinalizer(t *testing.T) {
    finalizers := []string{"foo", "bar", "baz"}

    if !containsFinalizer(finalizers, "bar") {
        t.Error("Expected to find 'bar' in finalizers")
    }

    if containsFinalizer(finalizers, "qux") {
        t.Error("Did not expect to find 'qux' in finalizers")
    }
}
```

Run tests:

```bash
make test
```

## Building

### Local Binary

```bash
make build-local
# Output: bin/webhook
```

### Container Image

```bash
# Build with default registry
make build

# Build with custom registry and tag
make build IMAGE_REGISTRY=myregistry.com IMAGE_TAG=v1.0.0

# Build and push
make push
```

### Helm Chart

```bash
# Lint the chart
helm lint deploy/helm/node-cleanup-webhook

# Template the chart (dry-run)
helm template test deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --set image.tag=dev

# Package the chart
helm package deploy/helm/node-cleanup-webhook
```

## Testing

### Unit Tests

```bash
make test
```

### Integration Tests

1. Deploy to test cluster:

```bash
helm install test deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace \
  --set image.tag=dev
```

2. Create test node:

```bash
kubectl create -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  name: integration-test-node
spec:
  podCIDR: 10.244.99.0/24
EOF
```

3. Verify finalizer:

```bash
kubectl get node integration-test-node -o jsonpath='{.metadata.finalizers}'
# Should include: infra.894.io/node-cleanup
```

4. Delete and verify cleanup:

```bash
kubectl delete node integration-test-node
# Check logs to see cleanup running
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook -f
```

### Manual Testing

```bash
# Watch webhook logs
make logs

# Check deployment status
make status

# Describe pods
make describe
```

## Debugging

### Enable Verbose Logging

```bash
# Locally
make run-local  # Already runs with -v=2

# In cluster
helm upgrade test deploy/helm/node-cleanup-webhook \
  --set log.verbosity=4
```

Verbosity levels:
- 0: Errors only
- 1: Important info
- 2: Detailed info (default)
- 3: Debug info
- 4: Trace level

### Common Issues

**Webhook not adding finalizers**:

```bash
# Check webhook is registered
kubectl get mutatingwebhookconfiguration

# Check TLS certificates
kubectl get certificate -n node-cleanup-system
kubectl describe certificate -n node-cleanup-system

# Check webhook endpoint
kubectl describe mutatingwebhookconfiguration node-cleanup-webhook
```

**Cleanup not running**:

```bash
# Check watcher is running
kubectl get pods -n node-cleanup-system

# Check logs for errors
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook

# Verify finalizer is present
kubectl get node <node-name> -o jsonpath='{.metadata.finalizers}'
```

## Code Quality

### Linting

```bash
# Run golangci-lint
make lint

# Auto-fix issues
golangci-lint run --fix
```

### Formatting

```bash
make fmt
```

### Dependency Management

```bash
# Add new dependency
go get <package>

# Update dependencies
go get -u ./...

# Tidy dependencies
make tidy
```

## CI/CD

The project uses GitHub Actions for CI/CD:

**On PR / Push to main/develop**:
- Lint code
- Run tests
- Build binary and container
- Validate Helm chart

**On Tag (v\*)**:
- Build multi-arch images
- Push to container registry
- Package Helm chart
- Create GitHub release

## Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Commit your changes: `git commit -am 'Add my feature'`
7. Push to branch: `git push origin feature/my-feature`
8. Create Pull Request

### Commit Messages

Follow conventional commits:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Adding tests
- `chore:` - Maintenance tasks

Example: `feat: add Portworx decommission logic`

## Release Process

1. Update version in [`deploy/helm/node-cleanup-webhook/Chart.yaml`](../deploy/helm/node-cleanup-webhook/Chart.yaml)
2. Create and push tag:

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

3. GitHub Actions will automatically:
   - Build and push container images
   - Package Helm chart
   - Create GitHub release
