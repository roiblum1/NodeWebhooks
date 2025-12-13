# Code Quality & Best Practices

This document outlines code quality improvements and best practices that should be implemented.

## 🧪 Testing Strategy

### Current State
- ❌ No unit tests
- ❌ No integration tests
- ❌ No E2E tests

### Recommended Test Structure

```
.
├── pkg/
│   ├── plugins/
│   │   ├── plugin.go
│   │   ├── plugin_test.go          # NEW
│   │   ├── portworx.go
│   │   ├── portworx_test.go        # NEW
│   │   └── logger_test.go          # NEW
│   ├── watcher/
│   │   ├── watcher.go
│   │   └── watcher_test.go         # NEW
│   └── webhook/
│       ├── server.go
│       └── server_test.go          # NEW
└── test/
    ├── e2e/
    │   ├── suite_test.go
    │   ├── finalizer_test.go
    │   └── cleanup_test.go
    └── integration/
        ├── plugin_integration_test.go
        └── webhook_integration_test.go
```

### Example Unit Test

```go
// pkg/plugins/plugin_test.go
package plugins

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPluginRegistry_Enable(t *testing.T) {
	registry := NewRegistry()
	mockPlugin := &mockPlugin{name: "test"}

	registry.Register(mockPlugin)

	err := registry.Enable("test")
	if err != nil {
		t.Errorf("Enable failed: %v", err)
	}

	enabled := registry.GetEnabledPlugins()
	if len(enabled) != 1 || enabled[0] != "test" {
		t.Errorf("Expected [test], got %v", enabled)
	}
}

func TestPluginRegistry_RunAll(t *testing.T) {
	registry := NewRegistry()
	mockPlugin := &mockPlugin{
		name:      "test",
		shouldRun: true,
		runCount:  0,
	}

	registry.Register(mockPlugin)
	registry.Enable("test")

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	err := registry.RunAll(context.Background(), node)
	if err != nil {
		t.Errorf("RunAll failed: %v", err)
	}

	if mockPlugin.runCount != 1 {
		t.Errorf("Expected plugin to run once, ran %d times", mockPlugin.runCount)
	}
}

// Mock plugin for testing
type mockPlugin struct {
	name      string
	shouldRun bool
	runCount  int
	runError  error
}

func (m *mockPlugin) Name() string                                      { return m.name }
func (m *mockPlugin) ShouldRun(node *corev1.Node) bool                  { return m.shouldRun }
func (m *mockPlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	m.runCount++
	return m.runError
}
```

### Example Integration Test

```go
// test/integration/webhook_integration_test.go
// +build integration

package integration

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func TestWebhookAddsFinalizer(t *testing.T) {
	// Requires running webhook and k8s cluster
	client := getKubernetesClient(t)

	// Create test node
	node := createTestNode(t, client, "integration-test-node")
	defer deleteNode(t, client, node.Name)

	// Wait for finalizer to be added
	time.Sleep(2 * time.Second)

	// Verify finalizer exists
	updatedNode, err := client.CoreV1().Nodes().Get(
		context.Background(),
		node.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Fatalf("Failed to get node: %v", err)
	}

	found := false
	for _, f := range updatedNode.Finalizers {
		if f == "infra.894.io/node-cleanup" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Finalizer was not added to node")
	}
}
```

---

## 🔍 Code Quality Tools

### 1. Linting with golangci-lint

**Install**:
```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | \
  sh -s -- -b $(go env GOPATH)/bin v1.55.2
```

**Configuration** (`.golangci.yml`):
```yaml
linters:
  enable:
    - gofmt
    - goimports
    - govet
    - errcheck
    - staticcheck
    - unused
    - gosimple
    - structcheck
    - varcheck
    - ineffassign
    - deadcode
    - typecheck
    - gosec
    - gocyclo
    - dupl

linters-settings:
  gocyclo:
    min-complexity: 15
  dupl:
    threshold: 100
  govet:
    check-shadowing: true

run:
  timeout: 5m
  skip-dirs:
    - vendor
```

**Run**:
```bash
golangci-lint run ./...
```

### 2. Code Coverage

**Generate coverage**:
```bash
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

**Target**: Minimum 70% coverage for critical packages

### 3. Vulnerability Scanning

**Using govulncheck**:
```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

### 4. Dependency Management

**Check for updates**:
```bash
go list -u -m all
```

**Audit dependencies**:
```bash
go mod verify
go mod tidy
```

---

## 📐 Code Structure Improvements

### 1. Error Handling

**Current**: Basic error wrapping
```go
return fmt.Errorf("failed: %w", err)
```

**Improved**: Custom error types
```go
// pkg/errors/errors.go
package errors

type CleanupError struct {
	Plugin string
	Node   string
	Err    error
}

func (e *CleanupError) Error() string {
	return fmt.Sprintf("plugin %s failed for node %s: %v", e.Plugin, e.Node, e.Err)
}

// Usage:
return &errors.CleanupError{
	Plugin: "portworx",
	Node:   node.Name,
	Err:    err,
}
```

### 2. Logging Standards

**Current**: Mixed logging styles
```go
klog.Infof("Running cleanup for: %s", node.Name)
fmt.Printf("Node: %s\n", node.Name)
```

**Improved**: Structured logging
```go
// Use consistent klog with levels
klog.V(0).Infof("Critical: %s", msg)  // Always shown
klog.V(1).Infof("Important: %s", msg) // -v=1
klog.V(2).Infof("Detailed: %s", msg)  // -v=2
klog.V(4).Infof("Debug: %s", msg)     // -v=4

// Add context fields
klog.Infof("[node=%s] [plugin=%s] %s", node.Name, pluginName, msg)
```

### 3. Context Propagation

**Current**: Context not always used
```go
func (p *Plugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	// ctx not used in HTTP calls
	resp, err := http.Get(url)
}
```

**Improved**: Always use context
```go
func (p *Plugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
}
```

### 4. Interface Segregation

**Current**: Large plugin interface
```go
type Plugin interface {
	Name() string
	ShouldRun(node *corev1.Node) bool
	Cleanup(ctx context.Context, node *corev1.Node) error
	// Future additions make ALL plugins change
}
```

**Improved**: Optional interfaces
```go
// Base interface
type Plugin interface {
	Name() string
	Cleanup(ctx context.Context, node *corev1.Node) error
}

// Optional interfaces
type ConditionalPlugin interface {
	Plugin
	ShouldRun(node *corev1.Node) bool
}

type HealthCheckablePlugin interface {
	Plugin
	HealthCheck(ctx context.Context) error
}

// Plugins implement only what they need
```

---

## 🛡️ Security Best Practices

### 1. Input Validation

**Add validation for webhook inputs**:
```go
func (s *Server) mutateNode(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// Validate operation type
	if req.Operation != admissionv1.Create {
		return allowResponse()
	}

	// Validate object type
	if req.Kind.Kind != "Node" {
		return denyResponse("unexpected kind")
	}

	// Validate node object
	var node corev1.Node
	if err := json.Unmarshal(req.Object.Raw, &node); err != nil {
		return denyResponse(fmt.Sprintf("invalid node: %v", err))
	}

	// Validate node name
	if node.Name == "" {
		return denyResponse("node name is required")
	}

	// Continue with mutation...
}
```

### 2. Timeout Enforcement

**Add timeouts to all operations**:
```go
// In plugin cleanup
ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
defer cancel()

// In HTTP calls
client := &http.Client{
	Timeout: 30 * time.Second,
}
```

### 3. Secrets Handling

**Never log secrets**:
```go
// Bad
klog.Infof("API Key: %s", apiKey)

// Good
klog.Infof("API Key: ***REDACTED***")

// Use secret redaction helper
func redact(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return s[:2] + "***" + s[len(s)-2:]
}
```

---

## 📊 Performance Best Practices

### 1. Avoid Memory Leaks

**Use contexts for goroutines**:
```go
// Bad - goroutine may leak
go func() {
	for {
		time.Sleep(1 * time.Second)
		// This runs forever
	}
}()

// Good - respects context
go func() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Do work
		case <-ctx.Done():
			return
		}
	}
}()
```

### 2. Efficient Resource Usage

**Close resources properly**:
```go
// Always use defer for cleanup
resp, err := http.Get(url)
if err != nil {
	return err
}
defer resp.Body.Close()

// Read body
body, err := io.ReadAll(resp.Body)
```

### 3. Informer Caching

**Use informer cache instead of direct API calls**:
```go
// Bad - hits API server every time
node, err := client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})

// Good - uses informer cache
obj, exists, err := w.informer.GetIndexer().GetByKey(name)
if !exists {
	return fmt.Errorf("node not found in cache")
}
node := obj.(*corev1.Node)
```

---

## 📝 Documentation Best Practices

### 1. Code Comments

**Function documentation**:
```go
// NewPortworxPlugin creates a new Portworx cleanup plugin.
//
// The plugin decommissions Portworx nodes before deletion by calling
// the Portworx API to drain storage pools and remove the node from
// the cluster.
//
// Parameters:
//   - client: Kubernetes client for API operations
//   - labelSelector: Label to identify Portworx nodes (e.g., "px/enabled=true")
//
// The plugin only runs on nodes matching the label selector.
func NewPortworxPlugin(client kubernetes.Interface, labelSelector string) *PortworxPlugin {
	// Implementation...
}
```

### 2. Package Documentation

**Add package-level docs**:
```go
// Package plugins provides a plugin system for node cleanup operations.
//
// Plugins implement the Plugin interface and are registered with the
// Registry. Each plugin can decide whether to run for a specific node
// and performs cleanup operations before the node is deleted.
//
// Example usage:
//
//	registry := plugins.NewRegistry()
//	registry.Register(plugins.NewPortworxPlugin(client, "px/enabled=true"))
//	registry.Enable("portworx")
//	err := registry.RunAll(ctx, node)
package plugins
```

### 3. README in Each Package

**Add README.md to important packages**:
```markdown
# plugins/

This package implements the plugin system for node cleanup.

## Available Plugins

- **logger**: Logs node information (always enabled by default)
- **portworx**: Decommissions Portworx storage nodes

## Adding a New Plugin

See [ADDING_PLUGINS.md](ADDING_PLUGINS.md) for detailed instructions.
```

---

## 🔄 CI/CD Improvements

### 1. Pre-commit Hooks

**Install pre-commit**:
```bash
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/golangci/golangci-lint
    rev: v1.55.2
    hooks:
      - id: golangci-lint

  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.5.0
    hooks:
      - id: trailing-whitespace
      - id: end-of-file-fixer
      - id: check-yaml
      - id: check-added-large-files
```

### 2. GitHub Actions Improvements

**Add more CI checks**:
```yaml
# .github/workflows/ci.yaml additions

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run Trivy scanner
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: 'fs'
          scan-ref: '.'
          format: 'sarif'
          output: 'trivy-results.sarif'

  dependency-review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Dependency Review
        uses: actions/dependency-review-action@v3
```

---

## Summary

### Quick Wins (Do First)
1. ✅ Add golangci-lint configuration
2. ✅ Add basic unit tests for plugin registry
3. ✅ Add structured logging
4. ✅ Add timeout enforcement
5. ✅ Add input validation

### Medium Term
1. Achieve 70% test coverage
2. Add integration tests
3. Implement custom error types
4. Add pre-commit hooks
5. Improve documentation

### Long Term
1. E2E testing framework
2. Performance benchmarks
3. Security audit
4. Complete API documentation
5. Plugin certification process
