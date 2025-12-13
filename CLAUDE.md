# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A **production-ready Kubernetes Mutating Admission Webhook** with a **plugin-based cleanup system** that automatically manages node cleanup before deletion using finalizers.

**Key Features**:
- ✅ **Plugin-based architecture** - Easy to add custom cleanup logic
- ✅ **Environment-driven configuration** - Configure via env vars
- ✅ **Automatic finalizer management** - Adds finalizers to all nodes
- ✅ **Production-ready** - Helm charts, CI/CD, documentation

## Repository Structure

```
NodesOperator/
├── cmd/webhook/main.go              # Entry point - initializes plugins
├── pkg/
│   ├── plugins/                     # Plugin system
│   │   ├── plugin.go               # Plugin interface & registry
│   │   ├── logger.go               # Logger plugin (default)
│   │   ├── portworx.go             # Portworx plugin
│   │   └── ADDING_PLUGINS.md       # Plugin development guide
│   ├── watcher/watcher.go          # Node cleanup watcher
│   ├── webhook/server.go           # Admission webhook handler
│   └── config/config.go            # Configuration system
├── deploy/
│   ├── helm/node-cleanup-webhook/  # Production Helm chart
│   └── manifests/                  # Raw Kubernetes manifests
├── docs/
│   ├── ARCHITECTURE.md             # Design decisions
│   ├── DEVELOPMENT.md              # Development guide
│   ├── IMPROVEMENTS.md             # Future enhancements
│   ├── CODE_QUALITY.md             # Code quality guidelines
│   └── CONTEXT_USAGE.md            # Why and how to use context
│   ├── AIR_GAPPED_DEPLOYMENT.md    # Disconnected environment deployment
├── vendor/                         # Vendored dependencies (air-gapped support)
├── .env.example                     # Configuration examples
└── README.md                        # User documentation
```

## Plugin System

### Available Plugins

- **logger** - Logs node information with structured logging (enabled by default)
- **portworx** - Portworx decommission (placeholder implementation)

### Enabling Plugins

```bash
# Via environment variable (ORDER MATTERS!)
export ENABLED_PLUGINS=logger,portworx

# Plugins execute in the order specified:
# 1. logger runs first
# 2. portworx runs second

# Configure plugin options
export PORTWORX_API_ENDPOINT=http://portworx-api:9001
export PORTWORX_LABEL_SELECTOR=px/enabled=true
```

### ⚡ Plugin Execution Order

**IMPORTANT**: Plugins run in the **exact order** you specify in `ENABLED_PLUGINS`.

Example:
```bash
# This order: logger → drain → portworx
ENABLED_PLUGINS=logger,drain,portworx

# Different order: portworx → drain → logger
ENABLED_PLUGINS=portworx,drain,logger
```

The order is tracked in the plugin registry and logged on startup:

```log
✅ Enabled cleanup plugin: logger (position 1)
✅ Enabled cleanup plugin: portworx (position 2)
```

During cleanup:

```log
Running plugin plugin="logger" position=1 total=2
Running plugin plugin="portworx" position=2 total=2
```

### 📊 Structured Logging

All logging uses **klog's structured logging** (`InfoS`, `ErrorS`) for better observability:

**Benefits:**
- Machine-parseable logs (JSON format compatible)
- Easy to query in log aggregation systems (Elasticsearch, Loki)
- Consistent key-value pairs across all logs
- Better context for debugging

**Example logs:**
```log
I1213 02:19:38.936635 16796 plugin.go:54] "Enabled cleanup plugin" plugin="logger" position=1
I1213 02:19:38.936639 16796 plugin.go:54] "Enabled cleanup plugin" plugin="portworx" position=2
I1213 02:19:38.936967 16796 watcher.go:121] "Starting cleanup watcher" finalizerName="infra.894.io/node-cleanup"
```

**In code:**
```go
// Structured logging (GOOD)
klog.InfoS("Plugin completed successfully", "plugin", name, "node", node.Name, "duration", elapsed)

// Old style (AVOID)
klog.Infof("Plugin %s completed for node %s in %v", name, node.Name, elapsed)
```

### Adding a New Plugin

See [pkg/plugins/ADDING_PLUGINS.md](pkg/plugins/ADDING_PLUGINS.md) for complete guide.

Quick summary:
1. Create `pkg/plugins/myplugin.go`
2. Implement the `Plugin` interface
3. Register in `cmd/webhook/main.go`
4. Add config in `pkg/config/config.go`
5. Enable with `ENABLED_PLUGINS=logger,myplugin`

## Quick Commands

### Development
```bash
# Run locally with logger plugin
export ENABLED_PLUGINS=logger
make run-local

# Run with multiple plugins
export ENABLED_PLUGINS=logger,portworx
export PORTWORX_API_ENDPOINT=http://my-api:9001
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
# Deploy with Helm
make deploy IMAGE_REGISTRY=myregistry.com IMAGE_TAG=v1.0.0

# Upgrade
make upgrade

# View logs
make logs

# Check status
make status
```

## Code Organization

### Entry Point: [cmd/webhook/main.go](cmd/webhook/main.go)
- Loads configuration from environment
- Initializes plugin registry
- Registers available plugins (logger, portworx)
- Enables plugins based on ENABLED_PLUGINS
- Starts webhook server and cleanup watcher

### Plugin System: [pkg/plugins/](pkg/plugins/)
- `plugin.go` - Plugin interface and registry
- `logger.go` - Logs node deletion info
- `portworx.go` - Portworx decommission (implement your logic here)
- `ADDING_PLUGINS.md` - Step-by-step guide to add plugins

### Configuration: [pkg/config/config.go](pkg/config/config.go)
- Loads from environment variables
- Plugin-specific configuration
- See `.env.example` for all options

### Watcher: [pkg/watcher/watcher.go](pkg/watcher/watcher.go)
- Watches for node deletions
- Runs enabled plugins via registry
- Manages finalizers
- Handles retries

### Webhook: [pkg/webhook/server.go](pkg/webhook/server.go)
- Adds finalizers on node CREATE
- Stateless HTTP handler

## Configuration

### Environment Variables

```bash
# Webhook settings
PORT=8443
TLS_CERT_FILE=/etc/webhook/certs/tls.crt
TLS_KEY_FILE=/etc/webhook/certs/tls.key

# Plugin selection
ENABLED_PLUGINS=logger,portworx

# Logger plugin
LOGGER_FORMAT=pretty
LOGGER_VERBOSITY=info

# Portworx plugin
PORTWORX_LABEL_SELECTOR=px/enabled=true
PORTWORX_API_ENDPOINT=http://portworx-api:9001
PORTWORX_TIMEOUT=300s
```

Full examples in [.env.example](.env.example)

## Common Tasks

### Implement Portworx Cleanup Logic

Edit [pkg/plugins/portworx.go](pkg/plugins/portworx.go), find the `Cleanup()` function (line ~42):

```go
func (p *PortworxPlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
    klog.Infof("🔧 Portworx: Decommissioning node %s", node.Name)

    // TODO: Replace this with actual implementation
    // Option 1: Call Portworx REST API
    // Option 2: Execute pxctl via kubectl exec
    // Option 3: Delete/Update StorageNode CRD

    // Your implementation here
    return p.callPortworxAPI(ctx, node.Name)
}
```

### Add a Custom Plugin

1. **Create plugin file** `pkg/plugins/myservice.go`:
```go
package plugins

import (
    "context"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/client-go/kubernetes"
)

type MyServicePlugin struct {
    BasePlugin
}

func NewMyServicePlugin(client kubernetes.Interface) *MyServicePlugin {
    return &MyServicePlugin{
        BasePlugin: BasePlugin{name: "myservice", client: client},
    }
}

func (p *MyServicePlugin) ShouldRun(node *corev1.Node) bool {
    return true // Run for all nodes
}

func (p *MyServicePlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
    // Your cleanup logic
    return nil
}
```

2. **Register in main.go** (line ~73):
```go
pluginRegistry.Register(plugins.NewMyServicePlugin(client))
```

3. **Add config in config.go** (line ~59):
```go
c.PluginConfigs["myservice"] = PluginConfig{
    Enabled: c.isPluginEnabled("myservice"),
    Options: map[string]string{
        "endpoint": getEnv("MYSERVICE_ENDPOINT", "http://api:8080"),
    },
}
```

4. **Enable it**:
```bash
export ENABLED_PLUGINS=logger,myservice
make run-local
```

### Emergency: Skip Cleanup

```bash
kubectl annotate node <node-name> infra.894.io/skip-cleanup=true
kubectl delete node <node-name>
```

## Testing

### Test Locally

```bash
# Start webhook
export ENABLED_PLUGINS=logger
make run-local

# In another terminal, create test node
kubectl create -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  name: test-node
spec:
  podCIDR: 10.244.1.0/24
EOF

# Verify finalizer added
kubectl get node test-node -o jsonpath='{.metadata.finalizers}'

# Delete and watch cleanup
kubectl delete node test-node
```

You should see:
```
📦 Enabled plugins: [logger]
🔧 Running plugin: logger
═══════════════════════════════════════════════════════════════
  🗑️  DELETING NODE: test-node
═══════════════════════════════════════════════════════════════
✅ Plugin logger completed successfully
```

## Architecture

### Plugin Flow

```
1. Node Created
   └─> Webhook adds finalizer automatically

2. Node Deleted
   └─> Deletion blocked by finalizer
   └─> Watcher detects deletionTimestamp
   └─> Plugin Registry runs enabled plugins:
       ├─> logger: Logs node info ✅
       ├─> portworx: Decommissions (if enabled) ✅
       └─> custom: Your plugin ✅
   └─> All plugins succeed
   └─> Remove finalizer
   └─> Kubernetes completes deletion
```

### Why Plugins?

- ✅ **Modular** - Each cleanup task is isolated
- ✅ **Configurable** - Enable/disable via env vars
- ✅ **Extensible** - Add new plugins in minutes
- ✅ **Testable** - Test each plugin independently
- ✅ **Maintainable** - Changes don't affect other plugins

## Key Constants

- `FinalizerName`: `infra.894.io/node-cleanup`
- `SkipCleanupAnnotation`: `infra.894.io/skip-cleanup`
- Default namespace: `node-cleanup-system`
- Default port: `8443`

## Documentation

- [README.md](README.md) - User guide with quick start
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - Design decisions
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) - Development guide
- [docs/IMPROVEMENTS.md](docs/IMPROVEMENTS.md) - Future enhancements (25+ ideas)
- [docs/CODE_QUALITY.md](docs/CODE_QUALITY.md) - Code quality guidelines
- [pkg/plugins/ADDING_PLUGINS.md](pkg/plugins/ADDING_PLUGINS.md) - Plugin guide

## Troubleshooting

### Plugin not running

```bash
# Check which plugins are enabled
make logs | grep "Enabled plugins"
# Should show: 📦 Enabled plugins: [logger portworx]

# Verify plugin configuration
make logs | grep "Plugin \["
# Shows config for each plugin
```

### Node stuck in Terminating

```bash
# Check finalizer
kubectl get node <name> -o jsonpath='{.metadata.finalizers}'

# Check logs for cleanup errors
make logs

# Emergency bypass
kubectl annotate node <name> infra.894.io/skip-cleanup=true
```

### Plugin failing

```bash
# View detailed logs
make logs

# Look for plugin-specific errors
# Example: "Plugin portworx failed: connection refused"

# Fix the plugin configuration
export PORTWORX_API_ENDPOINT=http://correct-endpoint:9001
```

## Next Steps

1. **Implement Portworx cleanup** in [pkg/plugins/portworx.go](pkg/plugins/portworx.go)
2. **Add your custom plugins** - see [pkg/plugins/ADDING_PLUGINS.md](pkg/plugins/ADDING_PLUGINS.md)
3. **Add tests** - see [docs/CODE_QUALITY.md](docs/CODE_QUALITY.md)
4. **Deploy to production** - see [README.md](README.md) deployment section
5. **Consider improvements** - see [docs/IMPROVEMENTS.md](docs/IMPROVEMENTS.md) for 25+ ideas
