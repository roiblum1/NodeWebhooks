# Node Cleanup Webhook

A Kubernetes mutating admission webhook that automatically manages node cleanup before deletion using finalizers. This ensures that critical cleanup tasks (like Portworx decommissioning, storage cleanup, and external system notifications) complete successfully before nodes are removed from the cluster.

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/894/node-cleanup-webhook)](https://goreportcard.com/report/github.com/894/node-cleanup-webhook)

## Features

- **Plugin-Based Architecture**: Ordered execution of cleanup plugins
- **Structured Logging**: Machine-parseable logs for better observability
- **Air-Gapped Ready**: Vendored dependencies for disconnected environments
- **Automatic Finalizer Management**: Adds finalizers to all nodes (existing and new)
- **Cleanup Orchestration**: Runs custom cleanup logic before node deletion
- **Webhook-Based**: Simpler than full operator pattern
- **Production Ready**: Includes Helm charts, RBAC, monitoring, and more
- **Highly Available**: Runs with 2+ replicas and pod disruption budgets
- **Flexible Deployment**: Supports manual certificates (no cert-manager needed)
- **Emergency Override**: Skip cleanup with annotation when needed

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       Kubernetes API Server                              │
│                                                                         │
│   Node CREATE ─────► Webhook adds finalizer atomically                  │
│   Node DELETE ─────► Sets deletionTimestamp (blocked by finalizer)      │
└─────────────────────────────────────────────────────────────────────────┘
                                                       │
                                                       ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     Cleanup Watcher (same binary)                        │
│                                                                         │
│   1. Detects node with deletionTimestamp + finalizer                    │
│   2. Runs cleanup tasks (Portworx, storage, notifications)              │
│   3. Removes finalizer                                                  │
│   4. Kubernetes completes deletion                                      │
└─────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes 1.20+
- Helm 3.0+ (for Helm installation)
- cert-manager (optional, for automatic TLS certificate management)

### Installation

#### Option 1: Helm (Recommended)

```bash
# Add cert-manager if not already installed
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Create self-signed issuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
EOF

# Install the webhook
helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace
```

#### Option 2: kubectl with manifests

```bash
kubectl apply -f deploy/manifests/rbac.yaml
kubectl apply -f deploy/manifests/webhook-config.yaml
kubectl apply -f deploy/manifests/deployment.yaml
```

### Verification

```bash
# Check webhook is running
kubectl get pods -n node-cleanup-system

# Verify finalizers are added to nodes
kubectl get nodes -o custom-columns=NAME:.metadata.name,FINALIZERS:.metadata.finalizers

# View logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook -f
```

## Plugin System

The webhook uses a plugin-based architecture for cleanup operations. Plugins execute in the **exact order** specified in the `ENABLED_PLUGINS` environment variable.

### Available Plugins

- **logger** - Logs node deletion details (enabled by default)
- **portworx** - Portworx node decommissioning (placeholder implementation)

### Configuring Plugins

```yaml
# Helm values.yaml
env:
  ENABLED_PLUGINS: "logger,portworx"  # Order matters!

  # Logger plugin
  LOGGER_FORMAT: "pretty"
  LOGGER_VERBOSITY: "info"

  # Portworx plugin
  PORTWORX_LABEL_SELECTOR: "px/enabled=true"
  PORTWORX_API_ENDPOINT: "http://portworx-api:9001"
```

**Plugin execution order matters:**
```bash
ENABLED_PLUGINS=logger,portworx  # logger runs first, then portworx
ENABLED_PLUGINS=portworx,logger  # portworx runs first, then logger
```

See [docs/ADDING_PLUGINS.md](docs/ADDING_PLUGINS.md) for creating custom plugins.

## Air-Gapped / Disconnected Environments

This webhook is **fully compatible** with air-gapped environments:

- ✅ All dependencies vendored (3,361 files, 46 MB)
- ✅ Builds with `-mod=vendor` (no internet needed)
- ✅ Manual certificates supported (no cert-manager required)
- ✅ Simple container image transfer

**Quick deployment:**

```bash
# Connected environment: Build and export
podman build -t webhook:v1.0.0 .
podman save webhook:v1.0.0 -o webhook.tar

# Transfer webhook.tar (~50-80 MB) to air-gapped environment

# Air-gapped environment: Load and deploy
podman load -i webhook.tar
podman tag webhook:v1.0.0 internal-registry/webhook:v1.0.0
podman push internal-registry/webhook:v1.0.0
```

**Complete guide:** [docs/AIR_GAPPED_DEPLOYMENT.md](docs/AIR_GAPPED_DEPLOYMENT.md)

## Configuration

### Helm Values

Key configuration options in `values.yaml`:

```yaml
# Number of webhook replicas
replicaCount: 2

# Image configuration
image:
  repository: registry.example.com/node-cleanup-webhook
  tag: "latest"

# Webhook behavior
webhook:
  certManager:
    enabled: false  # Use manual certificates for air-gapped
  failurePolicy: Ignore  # Allow node creation if webhook is down
  timeoutSeconds: 10

# Plugin configuration
env:
  ENABLED_PLUGINS: "logger,portworx"
```

Full configuration options: [values.yaml](deploy/helm/node-cleanup-webhook/values.yaml)

## Usage

### Normal Operation

The webhook operates automatically:

1. **On startup**: Adds finalizers to all existing nodes
2. **On node creation**: Automatically adds finalizer to new nodes
3. **On node deletion**: Runs cleanup, then allows deletion

### Emergency Bypass

If you need to delete a node immediately without cleanup:

```bash
kubectl annotate node <node-name> infra.894.io/skip-cleanup=true
kubectl delete node <node-name>
```

### Testing Locally

```bash
# Run locally against your cluster
make run-local

# Create a test node
kubectl create -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  name: test-node-example
spec:
  podCIDR: 10.244.1.0/24
EOF

# Delete the node and watch cleanup
kubectl delete node test-node-example
```

## Development

### Building

```bash
# Build binary locally
make build-local

# Build container image
make build IMAGE_REGISTRY=myregistry.com IMAGE_TAG=v1.0.0

# Build and push
make push
```

### Testing

```bash
# Run tests
make test

# Run linter
make lint

# Format code
make fmt
```

### Project Structure

```
.
├── cmd/
│   └── webhook/
│       └── main.go              # Entry point
├── pkg/
│   ├── webhook/
│   │   └── server.go           # Admission webhook handler
│   └── watcher/
│       └── watcher.go          # Node cleanup watcher
├── deploy/
│   ├── helm/
│   │   └── node-cleanup-webhook/  # Helm chart
│   └── manifests/               # Raw Kubernetes manifests
├── docs/                        # Additional documentation
├── scripts/                     # Utility scripts
├── Dockerfile                   # Multi-stage build
├── Makefile                     # Build automation
└── go.mod                       # Go dependencies
```

## Implementing Cleanup Logic

The cleanup functions are placeholders in [`pkg/watcher/watcher.go`](pkg/watcher/watcher.go). Implement your custom logic:

```go
func (w *Watcher) runCleanup(ctx context.Context, node *corev1.Node) error {
    klog.Infof("Running cleanup for node: %s", node.Name)

    // Example: Decommission Portworx node
    if node.Labels["px/enabled"] == "true" {
        if err := decommissionPortworx(ctx, node.Name); err != nil {
            return fmt.Errorf("portworx cleanup failed: %w", err)
        }
    }

    // Example: Notify external systems
    if err := notifySlack(node.Name); err != nil {
        klog.Warningf("Slack notification failed: %v", err)
        // Don't fail cleanup for notification failures
    }

    // Example: Clean up storage
    if err := cleanupStorage(ctx, node.Name); err != nil {
        return fmt.Errorf("storage cleanup failed: %w", err)
    }

    return nil
}
```

**Important**: All cleanup functions must be idempotent (safe to run multiple times).

## Monitoring

The webhook exposes the following endpoints:

- `/healthz` - Health check
- `/readyz` - Readiness check
- `/mutate-node` - Webhook endpoint

Enable Prometheus monitoring by setting `monitoring.enabled=true` in Helm values.

## Troubleshooting

### Node stuck in Terminating

```bash
# Check if finalizer is present
kubectl get node <node-name> -o jsonpath='{.metadata.finalizers}'

# Check webhook logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook

# Emergency bypass (skip cleanup)
kubectl annotate node <node-name> infra.894.io/skip-cleanup=true
```

### Webhook not adding finalizers

```bash
# Check webhook is running
kubectl get pods -n node-cleanup-system

# Check webhook configuration
kubectl get mutatingwebhookconfiguration node-cleanup-webhook -o yaml

# Check TLS certificates
kubectl get certificate -n node-cleanup-system
kubectl get secret -n node-cleanup-system node-cleanup-webhook-tls
```

### Cleanup failures

```bash
# View detailed logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook -f

# Check node annotations for status
kubectl get node <node-name> -o jsonpath='{.metadata.annotations}'
```

## Security

- Runs as non-root user (UID 65532)
- Read-only root filesystem
- Drops all capabilities
- RBAC with minimal required permissions
- TLS-secured webhook endpoint

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/894/node-cleanup-webhook/issues)
- **Documentation**: [docs/](docs/)
- **Discussions**: [GitHub Discussions](https://github.com/894/node-cleanup-webhook/discussions)
