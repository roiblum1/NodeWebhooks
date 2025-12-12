# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A **production-ready Kubernetes Mutating Admission Webhook** that automatically manages node cleanup before deletion using finalizers. Ensures critical cleanup tasks (Portworx decommissioning, storage cleanup, notifications) complete before nodes are removed.

**Key Design**: Uses webhook + watcher pattern instead of full operator for simplicity and reliability. Webhook atomically adds finalizers at node creation; watcher orchestrates cleanup on deletion.

## Repository Structure

```
NodesOperator/
├── cmd/webhook/main.go          # Entry point - webhook server + watcher
├── pkg/
│   ├── webhook/server.go        # Admission webhook handler
│   └── watcher/watcher.go       # Node cleanup watcher
├── deploy/
│   ├── helm/node-cleanup-webhook/  # Production Helm chart
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/           # K8s resource templates
│   └── manifests/               # Raw Kubernetes manifests
├── docs/
│   ├── ARCHITECTURE.md          # Design decisions and flow
│   └── DEVELOPMENT.md           # Development guide
├── .github/workflows/           # CI/CD pipelines
├── scripts/                     # Utility scripts
├── Dockerfile                   # Multi-stage container build
├── Makefile                     # Build and deployment automation
├── go.mod                       # Go dependencies
└── README.md                    # User-facing documentation
```

## Quick Commands

### Development
```bash
# Run locally (auto-generates certs)
make run-local

# Build binary
make build-local

# Run tests
make test

# Format and lint
make fmt
make lint
```

### Deployment
```bash
# Deploy with Helm (recommended)
make deploy IMAGE_REGISTRY=myregistry.com IMAGE_TAG=v1.0.0

# Deploy with raw manifests
make deploy-manifests

# Upgrade existing deployment
make upgrade

# Remove from cluster
make undeploy
```

### Debugging
```bash
# View logs
make logs

# Check status
make status

# Describe pods
make describe
```

### Building
```bash
# Build container image
make build IMAGE_REGISTRY=myregistry.com IMAGE_TAG=v1.0.0

# Build and push
make push
```

## Code Organization

### Entry Point: [cmd/webhook/main.go](cmd/webhook/main.go)
- Starts both webhook server (port 8443) and cleanup watcher
- Creates Kubernetes client (in-cluster or kubeconfig)
- Handles graceful shutdown
- Registers HTTP endpoints: `/mutate-node`, `/healthz`, `/readyz`

### Webhook Handler: [pkg/webhook/server.go](pkg/webhook/server.go)
- `HandleMutateNode()` - HTTP handler for admission requests
- `mutateNode()` - Adds finalizer via JSON patch
- Only intercepts Node CREATE operations
- Stateless operation

### Cleanup Watcher: [pkg/watcher/watcher.go](pkg/watcher/watcher.go)
**Key Functions**:
- `New()` - Creates watcher with client-go informer
- `Run()` - Main loop: starts informer, initializes existing nodes, processes queue
- `initializeExistingNodes()` - Adds finalizers to all nodes on startup
- `ensureFinalizer()` - Ensures finalizer present on new/updated nodes
- `enqueueIfDeleting()` - Detects deletion and queues for cleanup
- `processNode()` - Handles single node cleanup with retry
- `runCleanup()` - **Placeholder for actual cleanup logic** (line ~207)
- `removeFinalizer()` - Removes finalizer after successful cleanup

## Implementation Status

### ✅ Complete
- Webhook adds finalizer to nodes atomically on CREATE
- Watcher adds finalizers to existing nodes on startup
- Watcher automatically adds finalizers to new nodes
- Cleanup triggered on node deletion
- Retry logic with 10s backoff
- Emergency bypass via annotation
- Production-ready Helm chart
- CI/CD pipelines
- Comprehensive documentation

### ⚠️ Placeholder (Needs Implementation)
**[pkg/watcher/watcher.go:~207](pkg/watcher/watcher.go)** - `runCleanup()` function

Currently just prints node name. Replace with actual cleanup:
```go
func (w *Watcher) runCleanup(ctx context.Context, node *corev1.Node) error {
    // Add your cleanup logic here:
    // - Decommission Portworx: pxctl cluster decommission
    // - Clean up storage: migrate PVs, remove volumes
    // - Notify external systems: CMDB, Slack, monitoring

    return nil
}
```

**Important**: Cleanup must be idempotent (safe to run multiple times).

## Helm Chart Configuration

### Key Values ([deploy/helm/node-cleanup-webhook/values.yaml](deploy/helm/node-cleanup-webhook/values.yaml))

```yaml
replicaCount: 2                  # HA deployment
image:
  repository: registry.example.com/node-cleanup-webhook
  tag: latest

webhook:
  failurePolicy: Ignore          # Allow node creation if webhook down
  certManager:
    enabled: true                # Auto TLS cert management

cleanup:
  portworx:
    enabled: false               # Enable Portworx cleanup
  timeout: 300s
  retryDelay: 10s

log:
  verbosity: 2                   # 0-4, higher = more verbose
```

## Common Tasks

### Adding Cleanup Logic
1. Edit [pkg/watcher/watcher.go](pkg/watcher/watcher.go)
2. Modify `runCleanup()` function
3. Ensure cleanup is idempotent
4. Test locally with `make run-local`
5. Build and deploy: `make build push deploy`

### Modifying Helm Chart
1. Edit templates in `deploy/helm/node-cleanup-webhook/templates/`
2. Update values in `deploy/helm/node-cleanup-webhook/values.yaml`
3. Lint: `helm lint deploy/helm/node-cleanup-webhook`
4. Test template: `helm template test deploy/helm/node-cleanup-webhook`
5. Upgrade: `make upgrade`

### Changing Finalizer Name
1. Update constant in [pkg/watcher/watcher.go](pkg/watcher/watcher.go): `FinalizerName`
2. Update constant in [pkg/webhook/server.go](pkg/webhook/server.go): `FinalizerName`
3. Update in [deploy/helm/node-cleanup-webhook/values.yaml](deploy/helm/node-cleanup-webhook/values.yaml): `finalizer.name`

### Emergency: Skip Cleanup
```bash
kubectl annotate node <node-name> infra.894.io/skip-cleanup=true
kubectl delete node <node-name>
```

## Testing Flow

1. **Start webhook locally**:
   ```bash
   make run-local
   ```

2. **Create test node**:
   ```bash
   kubectl create -f - <<EOF
   apiVersion: v1
   kind: Node
   metadata:
     name: test-node
   spec:
     podCIDR: 10.244.1.0/24
   EOF
   ```

3. **Verify finalizer added automatically**:
   ```bash
   kubectl get node test-node -o jsonpath='{.metadata.finalizers}'
   # Should show: ["infra.894.io/node-cleanup"]
   ```

4. **Delete and watch cleanup**:
   ```bash
   kubectl delete node test-node
   # Watch logs to see cleanup running
   ```

## Architecture Highlights

### Why Webhook vs Operator?
- **Simpler**: No reconciliation loop, leader election, or state management
- **Atomic**: Finalizer added at creation time (no race conditions)
- **Reliable**: `failurePolicy: Ignore` allows nodes to create if webhook unavailable

### Data Flow
```
1. CREATE node → Webhook adds finalizer → Node created with finalizer
2. DELETE node → Sets deletionTimestamp (blocked by finalizer)
3. Watcher detects → Runs cleanup → Removes finalizer
4. Kubernetes completes deletion
```

### High Availability
- 2 replicas (configurable)
- PodDisruptionBudget (minAvailable: 1)
- Stateless webhook (any replica handles requests)
- Each watcher replica processes different nodes

## Security

- Runs as non-root (UID 65532)
- Read-only root filesystem
- Drops all capabilities
- Minimal RBAC: nodes (get/list/watch/patch/update), events (create/patch)
- TLS-secured webhook endpoint

## Troubleshooting

### Node stuck in Terminating
```bash
# Check logs
make logs

# Check finalizer
kubectl get node <name> -o jsonpath='{.metadata.finalizers}'

# Emergency bypass
kubectl annotate node <name> infra.894.io/skip-cleanup=true
```

### Webhook not adding finalizers
```bash
# Check webhook running
kubectl get pods -n node-cleanup-system

# Check webhook config
kubectl get mutatingwebhookconfiguration

# Check TLS certs
kubectl get certificate -n node-cleanup-system
```

## Key Constants

- `FinalizerName`: `infra.894.io/node-cleanup`
- `SkipCleanupAnnotation`: `infra.894.io/skip-cleanup`
- Default namespace: `node-cleanup-system`
- Default port: `8443`

## Documentation

- [README.md](README.md) - User guide
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - Design and architecture
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) - Development guide
- [deploy/helm/node-cleanup-webhook/values.yaml](deploy/helm/node-cleanup-webhook/values.yaml) - Helm configuration

## CI/CD

- **On PR/Push**: Lint, test, build, validate Helm chart
- **On Tag (v\*)**: Build multi-arch images, push to registry, create release

## Next Steps for Production

1. Implement actual cleanup logic in `runCleanup()`
2. Add unit tests for webhook and watcher
3. Configure monitoring (Prometheus metrics)
4. Set up alerting for cleanup failures
5. Document your specific cleanup procedures
