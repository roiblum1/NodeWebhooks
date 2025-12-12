# Adding a New Cleanup Plugin

The plugin system makes it trivial to add new cleanup behavior. Follow these 3 steps:

## Step 1: Create Your Plugin File

Create a new file in `pkg/plugins/` (e.g., `my_service.go`):

```go
package plugins

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// MyServicePlugin handles cleanup for my custom service
type MyServicePlugin struct {
	BasePlugin
	apiEndpoint string
}

// NewMyServicePlugin creates a new plugin instance
func NewMyServicePlugin(client kubernetes.Interface, apiEndpoint string) *MyServicePlugin {
	return &MyServicePlugin{
		BasePlugin: BasePlugin{
			name:   "myservice",  // This is the plugin name used in config
			client: client,
		},
		apiEndpoint: apiEndpoint,
	}
}

// ShouldRun determines if this plugin should run for the given node
func (p *MyServicePlugin) ShouldRun(node *corev1.Node) bool {
	// Example: Only run if node has specific label
	if val, ok := node.Labels["myservice/enabled"]; ok && val == "true" {
		return true
	}
	return false
}

// Cleanup performs the actual cleanup
func (p *MyServicePlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	klog.Infof("MyService: Cleaning up node %s", node.Name)

	// Add your cleanup logic here:
	// - Call your service's API
	// - Remove node from database
	// - Update external systems
	// etc.

	klog.Infof("✅ MyService: Cleanup completed for %s", node.Name)
	return nil
}
```

## Step 2: Register the Plugin in main.go

Edit `cmd/webhook/main.go` and add your plugin to the registration section:

```go
// Register available plugins
klog.Info("Registering cleanup plugins...")
pluginRegistry.Register(plugins.NewLoggerPlugin(client))
pluginRegistry.Register(plugins.NewDrainPlugin(client, cfg.GetPluginOptionDuration("drain", "timeout", 5*time.Minute)))
pluginRegistry.Register(plugins.NewPortworxPlugin(client, cfg.GetPluginOption("portworx", "labelSelector", "px/enabled=true")))
pluginRegistry.Register(plugins.NewSlackPlugin(...))

// ADD YOUR PLUGIN HERE:
pluginRegistry.Register(plugins.NewMyServicePlugin(
	client,
	cfg.GetPluginOption("myservice", "apiEndpoint", "http://myservice-api:8080"),
))
```

## Step 3: Configure the Plugin

Add configuration to `pkg/config/config.go` in the `loadPluginConfigs()` function:

```go
// MyService plugin configuration
c.PluginConfigs["myservice"] = PluginConfig{
	Enabled: c.isPluginEnabled("myservice"),
	Options: map[string]string{
		"apiEndpoint": getEnv("MYSERVICE_API_ENDPOINT", "http://myservice-api:8080"),
		"timeout":     getEnv("MYSERVICE_TIMEOUT", "60s"),
		"apiKey":      getEnv("MYSERVICE_API_KEY", ""),
	},
}
```

## Step 4: Use It!

Enable your plugin with environment variables:

```bash
# Enable your plugin
export ENABLED_PLUGINS=logger,drain,myservice

# Configure it
export MYSERVICE_API_ENDPOINT=http://my-api:9000
export MYSERVICE_TIMEOUT=120s

# Run the webhook
make run-local
```

Or in Helm values:

```yaml
plugins:
  enabled:
    - logger
    - drain
    - myservice

  myservice:
    apiEndpoint: "http://my-api:9000"
    timeout: "120s"
```

## That's It!

Your plugin is now:
- ✅ Automatically loaded on startup
- ✅ Configured via environment variables
- ✅ Integrated with the cleanup workflow
- ✅ Logged and monitored

## Plugin Interface Reference

Every plugin must implement:

```go
type Plugin interface {
	// Name returns the plugin name (used in config)
	Name() string

	// ShouldRun decides if plugin runs for this specific node
	// Return false to skip this node
	ShouldRun(node *corev1.Node) bool

	// Cleanup performs the actual cleanup work
	// Return error to fail the entire cleanup (will retry)
	// Return nil to continue to next plugin
	Cleanup(ctx context.Context, node *corev1.Node) error
}
```

## Best Practices

1. **Make cleanup idempotent** - Safe to run multiple times
2. **Check before acting** - Use `ShouldRun()` to filter nodes
3. **Return errors for critical failures** - Cleanup will retry
4. **Log important steps** - Use klog for visibility
5. **Respect context** - Honor context cancellation
6. **Keep it focused** - One plugin = one responsibility

## Example: CMDB Update Plugin

```go
package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type CMDBPlugin struct {
	BasePlugin
	apiURL string
	apiKey string
}

func NewCMDBPlugin(client kubernetes.Interface, apiURL, apiKey string) *CMDBPlugin {
	return &CMDBPlugin{
		BasePlugin: BasePlugin{
			name:   "cmdb",
			client: client,
		},
		apiURL: apiURL,
		apiKey: apiKey,
	}
}

func (p *CMDBPlugin) ShouldRun(node *corev1.Node) bool {
	// Run for all nodes
	return true
}

func (p *CMDBPlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	klog.Infof("CMDB: Updating database for node %s", node.Name)

	payload := map[string]interface{}{
		"node_name": node.Name,
		"status":    "deleted",
		"timestamp": node.DeletionTimestamp,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", p.apiURL+"/nodes/delete", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("CMDB API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CMDB API returned status %d", resp.StatusCode)
	}

	klog.Infof("✅ CMDB: Updated for node %s", node.Name)
	return nil
}
```

Then register it:

```go
pluginRegistry.Register(plugins.NewCMDBPlugin(
	client,
	cfg.GetPluginOption("cmdb", "apiURL", ""),
	cfg.GetPluginOption("cmdb", "apiKey", ""),
))
```

Enable it:

```bash
export ENABLED_PLUGINS=logger,drain,cmdb
export CMDB_API_URL=https://cmdb.company.com/api
export CMDB_API_KEY=secret123
```

Done! 🎉
