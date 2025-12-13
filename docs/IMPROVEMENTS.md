# Future Improvements & Roadmap

This document outlines potential improvements, enhancements, and features that can be added to the Node Cleanup Webhook.

## 🎯 High Priority Improvements

### 1. Prometheus Metrics & Monitoring

**What**: Add Prometheus metrics for observability and alerting.

**Why**: Production systems need metrics to monitor health, performance, and failures.

**Implementation**:
```go
// Add to pkg/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Webhook metrics
	WebhookRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "node_cleanup_webhook_requests_total",
			Help: "Total number of webhook requests",
		},
		[]string{"operation", "result"},
	)

	WebhookDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "node_cleanup_webhook_duration_seconds",
			Help: "Webhook request duration in seconds",
		},
		[]string{"operation"},
	)

	// Cleanup metrics
	CleanupOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "node_cleanup_operations_total",
			Help: "Total number of cleanup operations",
		},
		[]string{"plugin", "result"},
	)

	CleanupDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "node_cleanup_duration_seconds",
			Help: "Cleanup operation duration in seconds",
		},
		[]string{"plugin"},
	)

	NodesWithFinalizerGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "node_cleanup_nodes_with_finalizer",
			Help: "Current number of nodes with cleanup finalizer",
		},
	)
)
```

**Changes needed**:
- Add `github.com/prometheus/client_golang` dependency
- Create `pkg/metrics/` package
- Add `/metrics` HTTP endpoint in main.go
- Instrument webhook handler and plugin registry
- Update Helm chart with ServiceMonitor for Prometheus Operator

**Estimated effort**: 4-6 hours

---

### 2. Event Recording

**What**: Create Kubernetes Events for cleanup lifecycle.

**Why**: Provides audit trail and visibility in `kubectl describe node`.

**Implementation**:
```go
// Add to pkg/watcher/events.go
package watcher

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

func (w *Watcher) recordEvent(node *corev1.Node, eventType, reason, message string) {
	w.recorder.Event(node, eventType, reason, message)
}

// Usage in cleanup flow:
w.recordEvent(node, corev1.EventTypeNormal, "CleanupStarted",
	"Node cleanup started")
w.recordEvent(node, corev1.EventTypeNormal, "CleanupCompleted",
	fmt.Sprintf("Cleanup completed, ran %d plugins", count))
w.recordEvent(node, corev1.EventTypeWarning, "CleanupFailed",
	fmt.Sprintf("Cleanup failed: %v", err))
```

**Changes needed**:
- Add EventRecorder to Watcher struct
- Create event recorder in main.go
- Record events at key points: start, plugin execution, completion, failure
- Update RBAC to allow event creation

**Estimated effort**: 2-3 hours

---

### 3. Graceful Plugin Failures

**What**: Make plugin failures configurable (fail-fast vs continue).

**Why**: Some cleanups are critical (Portworx), others are optional (notifications).

**Implementation**:
```go
// Add to plugin interface
type Plugin interface {
	Name() string
	ShouldRun(node *corev1.Node) bool
	Cleanup(ctx context.Context, node *corev1.Node) error

	// NEW: Indicates if failure should stop cleanup
	IsCritical() bool
}

// In plugin registry:
func (r *Registry) RunAll(ctx context.Context, node *corev1.Node) error {
	for name, plugin := range r.plugins {
		if err := plugin.Cleanup(ctx, node); err != nil {
			if plugin.IsCritical() {
				return fmt.Errorf("critical plugin %s failed: %w", name, err)
			}
			// Log but continue for non-critical plugins
			klog.Warningf("Non-critical plugin %s failed: %v", name, err)
		}
	}
	return nil
}
```

**Configuration**:
```yaml
plugins:
  portworx:
    critical: true  # Fail entire cleanup if this fails
  logger:
    critical: false # Log error but continue
```

**Estimated effort**: 2-3 hours

---

### 4. Parallel Plugin Execution

**What**: Run independent plugins in parallel for faster cleanup.

**Why**: Reduces total cleanup time when plugins don't depend on each other.

**Implementation**:
```go
func (r *Registry) RunAll(ctx context.Context, node *corev1.Node) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(r.plugins))

	for name, plugin := range r.plugins {
		if !r.enabled[name] || !plugin.ShouldRun(node) {
			continue
		}

		wg.Add(1)
		go func(name string, p Plugin) {
			defer wg.Done()
			if err := p.Cleanup(ctx, node); err != nil {
				errChan <- fmt.Errorf("plugin %s: %w", name, err)
			}
		}(name, plugin)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}
	return nil
}
```

**Estimated effort**: 3-4 hours

---

### 5. Plugin Dependencies & Ordering

**What**: Define plugin execution order and dependencies.

**Why**: Some cleanups must happen before others (e.g., drain before Portworx).

**Implementation**:
```go
type Plugin interface {
	Name() string
	ShouldRun(node *corev1.Node) bool
	Cleanup(ctx context.Context, node *corev1.Node) error

	// NEW: Plugins that must run before this one
	Dependencies() []string

	// NEW: Execution priority (lower runs first)
	Priority() int
}

// Example:
type PortworxPlugin struct {
	BasePlugin
}

func (p *PortworxPlugin) Dependencies() []string {
	return []string{"drain"} // Wait for drain to complete
}

func (p *PortworxPlugin) Priority() int {
	return 50 // Medium priority
}
```

**Estimated effort**: 4-6 hours

---

## 🔧 Medium Priority Improvements

### 6. Webhook Validating Mode

**What**: Add validating webhook to prevent unsafe node deletions.

**Why**: Block deletions of nodes running critical workloads.

**Implementation**:
- Add ValidatingWebhookConfiguration
- Check node for critical pods/labels before allowing deletion
- Return error to block deletion if unsafe

**Estimated effort**: 3-4 hours

---

### 7. Dry-Run Mode

**What**: Test cleanup without actually running it.

**Why**: Safe testing and validation of plugin configuration.

**Implementation**:
```bash
export DRY_RUN=true
make run-local

# Plugins log what they would do but don't execute
```

**Estimated effort**: 2-3 hours

---

### 8. Plugin Health Checks

**What**: Pre-flight checks before node deletion.

**Why**: Fail fast if external services are unreachable.

**Implementation**:
```go
type Plugin interface {
	Name() string
	ShouldRun(node *corev1.Node) bool
	Cleanup(ctx context.Context, node *corev1.Node) error

	// NEW: Check if plugin dependencies are available
	HealthCheck(ctx context.Context) error
}

// Example:
func (p *PortworxPlugin) HealthCheck(ctx context.Context) error {
	resp, err := http.Get(p.apiEndpoint + "/health")
	if err != nil {
		return fmt.Errorf("portworx API unreachable: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("portworx API unhealthy: %d", resp.StatusCode)
	}
	return nil
}
```

**Estimated effort**: 3-4 hours

---

### 9. Enhanced Retry Logic

**What**: Exponential backoff with configurable limits.

**Why**: Better handling of transient failures.

**Implementation**:
```go
type RetryConfig struct {
	MaxRetries     int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	BackoffFactor  float64
}

func (w *Watcher) processNodeWithRetry(ctx context.Context, nodeName string) {
	retries := 0
	delay := 10 * time.Second

	for retries < w.maxRetries {
		err := w.runCleanup(ctx, node)
		if err == nil {
			break
		}

		retries++
		if retries < w.maxRetries {
			klog.Warningf("Retry %d/%d for node %s after %v",
				retries, w.maxRetries, nodeName, delay)
			time.Sleep(delay)
			delay = time.Duration(float64(delay) * 2) // Exponential
		}
	}
}
```

**Estimated effort**: 2-3 hours

---

### 10. Configuration Hot-Reload

**What**: Reload configuration without restart.

**Why**: Enable/disable plugins dynamically.

**Implementation**:
- Watch ConfigMap for changes
- Reload plugin configuration
- Enable/disable plugins on the fly

**Estimated effort**: 4-6 hours

---

## 📊 Testing & Quality Improvements

### 11. Unit Tests

**What**: Comprehensive unit test coverage.

**Test areas**:
- Plugin interface implementations
- Webhook mutation logic
- Finalizer add/remove
- Configuration loading
- Plugin registry

**Example**:
```go
// pkg/plugins/plugin_test.go
func TestPluginRegistry(t *testing.T) {
	registry := NewRegistry()

	// Test registration
	plugin := &mockPlugin{name: "test"}
	registry.Register(plugin)

	// Test enable/disable
	registry.Enable("test")
	enabled := registry.GetEnabledPlugins()
	assert.Contains(t, enabled, "test")
}
```

**Estimated effort**: 8-12 hours

---

### 12. Integration Tests

**What**: End-to-end testing with real Kubernetes cluster.

**Test scenarios**:
- Node creation with finalizer
- Node deletion with cleanup
- Plugin execution order
- Failure scenarios
- Emergency bypass

**Estimated effort**: 12-16 hours

---

### 13. E2E Testing Framework

**What**: Automated testing in CI/CD.

**Implementation**:
- Kind cluster in GitHub Actions
- Automated node creation/deletion
- Verify finalizers and cleanup
- Test all plugins

**Estimated effort**: 8-12 hours

---

## 🛡️ Security Improvements

### 14. Webhook Authentication

**What**: Verify webhook requests are from API server.

**Why**: Prevent unauthorized webhook calls.

**Implementation**:
- Verify TLS client certificate
- Check request origin
- Validate admission review structure

**Estimated effort**: 3-4 hours

---

### 15. Secret Management for Plugin Configs

**What**: Load sensitive config from Kubernetes Secrets.

**Why**: Keep API keys, tokens out of environment variables.

**Implementation**:
```go
// Load from secret instead of env var
func (c *Config) LoadPluginSecretsFromK8s(client kubernetes.Interface) {
	secret, err := client.CoreV1().Secrets("node-cleanup-system").
		Get(context.TODO(), "plugin-secrets", metav1.GetOptions{})

	if err == nil {
		c.PluginConfigs["portworx"].Options["apiKey"] =
			string(secret.Data["portworx-api-key"])
	}
}
```

**Estimated effort**: 3-4 hours

---

## 📦 Deployment Improvements

### 16. Helm Chart Enhancements

**What**: More configurable Helm chart.

**Additions**:
- Per-plugin configuration in values.yaml
- NetworkPolicy templates
- PodMonitor for Prometheus
- Configurable resource limits per replica
- Horizontal Pod Autoscaling

**Estimated effort**: 4-6 hours

---

### 17. Kustomize Support

**What**: Alternative to Helm for GitOps workflows.

**Why**: Some teams prefer Kustomize over Helm.

**Structure**:
```
deploy/kustomize/
├── base/
│   ├── kustomization.yaml
│   ├── deployment.yaml
│   └── ...
└── overlays/
    ├── development/
    ├── staging/
    └── production/
```

**Estimated effort**: 4-6 hours

---

## 🔌 Plugin Ecosystem

### 18. Pre-built Plugin Library

**What**: Create common plugins that users can import.

**Plugins to add**:
- **Drain Plugin**: Gracefully drain pods (we removed this)
- **Slack/Teams Notifications**: Send alerts
- **CMDB Update**: Update configuration management database
- **Grafana Annotation**: Mark node deletion in Grafana
- **ServiceNow Ticket**: Create ticket for audit
- **Cloud Provider API**: Deregister from cloud (AWS/Azure/GCP)

**Estimated effort**: 2-4 hours per plugin

---

### 19. Plugin Marketplace/Registry

**What**: Documentation site with community plugins.

**Features**:
- Plugin catalog
- Installation instructions
- Configuration examples
- Community contributions

**Estimated effort**: 16-24 hours

---

## 📈 Performance Improvements

### 20. Leader Election for Watchers

**What**: Only one watcher processes each node.

**Why**: Avoid duplicate cleanup with multiple replicas.

**Implementation**:
```go
import "k8s.io/client-go/tools/leaderelection"

// Each watcher replica runs leader election
// Only leader processes cleanup work queue
```

**Estimated effort**: 4-6 hours

---

### 21. Informer Optimization

**What**: Optimize watch filters and cache.

**Why**: Reduce memory and API server load.

**Implementation**:
- Add label selectors to watch
- Implement cache pruning
- Use field selectors

**Estimated effort**: 3-4 hours

---

## 🎨 User Experience

### 22. CLI Tool

**What**: Command-line tool for operations.

**Features**:
```bash
# List nodes with finalizer
node-cleanup-webhook nodes list

# Check plugin status
node-cleanup-webhook plugins status

# Manually trigger cleanup
node-cleanup-webhook cleanup node-name

# Test plugin configuration
node-cleanup-webhook plugins test portworx
```

**Estimated effort**: 8-12 hours

---

### 23. Web Dashboard

**What**: Simple web UI for monitoring.

**Features**:
- List nodes being cleaned up
- View cleanup progress
- Plugin execution status
- Metrics visualization

**Estimated effort**: 20-30 hours

---

## 📝 Documentation Improvements

### 24. Video Tutorials

**What**: Create walkthrough videos.

**Topics**:
- Quick start
- Adding custom plugins
- Troubleshooting
- Production deployment

**Estimated effort**: 8-12 hours

---

### 25. Migration Guides

**What**: Help users migrate from operators.

**Content**:
- Comparison with operators
- Migration steps
- Rollback procedures

**Estimated effort**: 4-6 hours

---

## Priority Matrix

| Priority | Effort | Impact | Recommendation |
|----------|--------|--------|----------------|
| Metrics & Monitoring | Medium | High | **Do First** |
| Event Recording | Low | High | **Do First** |
| Graceful Failures | Low | High | **Do First** |
| Unit Tests | High | High | **Do Second** |
| Parallel Execution | Medium | Medium | **Do Third** |
| Plugin Health Checks | Medium | Medium | **Do Third** |
| Webhook Authentication | Medium | Medium | **Do Third** |
| Integration Tests | High | Medium | **Do Later** |
| CLI Tool | High | Low | **Nice to Have** |
| Web Dashboard | Very High | Low | **Nice to Have** |

---

## Contributing

Want to implement any of these improvements?

1. Open an issue describing which improvement you want to work on
2. We'll discuss the approach
3. Submit a PR with your implementation
4. Include tests and documentation

See [DEVELOPMENT.md](DEVELOPMENT.md) for development guidelines.
