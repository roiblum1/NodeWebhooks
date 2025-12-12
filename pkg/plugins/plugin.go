package plugins

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// Plugin defines the interface for cleanup plugins
type Plugin interface {
	// Name returns the plugin name
	Name() string

	// ShouldRun determines if this plugin should run for the given node
	ShouldRun(node *corev1.Node) bool

	// Cleanup performs the cleanup operation
	Cleanup(ctx context.Context, node *corev1.Node) error
}

// Registry manages all available cleanup plugins
type Registry struct {
	plugins map[string]Plugin
	enabled map[string]bool
}

// NewRegistry creates a new plugin registry
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		enabled: make(map[string]bool),
	}
}

// Register registers a new plugin
func (r *Registry) Register(plugin Plugin) {
	name := plugin.Name()
	r.plugins[name] = plugin
	klog.V(2).Infof("Registered plugin: %s", name)
}

// Enable enables a plugin by name
func (r *Registry) Enable(name string) error {
	if _, exists := r.plugins[name]; !exists {
		return fmt.Errorf("plugin %s not found", name)
	}
	r.enabled[name] = true
	klog.Infof("✅ Enabled cleanup plugin: %s", name)
	return nil
}

// Disable disables a plugin by name
func (r *Registry) Disable(name string) {
	r.enabled[name] = false
	klog.Infof("Disabled cleanup plugin: %s", name)
}

// RunAll runs all enabled plugins that should run for the given node
func (r *Registry) RunAll(ctx context.Context, node *corev1.Node) error {
	klog.Infof("Running cleanup plugins for node: %s", node.Name)

	ranCount := 0
	for name, plugin := range r.plugins {
		// Skip if plugin is not enabled
		if !r.enabled[name] {
			klog.V(2).Infof("Skipping disabled plugin: %s", name)
			continue
		}

		// Skip if plugin should not run for this node
		if !plugin.ShouldRun(node) {
			klog.V(2).Infof("Plugin %s: skipping (conditions not met)", name)
			continue
		}

		klog.Infof("🔧 Running plugin: %s", name)
		if err := plugin.Cleanup(ctx, node); err != nil {
			return fmt.Errorf("plugin %s failed: %w", name, err)
		}
		klog.Infof("✅ Plugin %s completed successfully", name)
		ranCount++
	}

	klog.Infof("Cleanup completed: ran %d plugins for node %s", ranCount, node.Name)
	return nil
}

// GetEnabledPlugins returns a list of enabled plugin names
func (r *Registry) GetEnabledPlugins() []string {
	var enabled []string
	for name, isEnabled := range r.enabled {
		if isEnabled {
			enabled = append(enabled, name)
		}
	}
	return enabled
}

// BasePlugin provides common functionality for plugins
type BasePlugin struct {
	name   string
	client kubernetes.Interface
}

// Name returns the plugin name
func (b *BasePlugin) Name() string {
	return b.name
}
