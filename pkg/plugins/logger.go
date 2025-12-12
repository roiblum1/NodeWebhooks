package plugins

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// LoggerPlugin logs node deletion information
type LoggerPlugin struct {
	BasePlugin
}

// NewLoggerPlugin creates a new logger plugin
func NewLoggerPlugin(client kubernetes.Interface) *LoggerPlugin {
	return &LoggerPlugin{
		BasePlugin: BasePlugin{
			name:   "logger",
			client: client,
		},
	}
}

// ShouldRun always returns true - log all node deletions
func (p *LoggerPlugin) ShouldRun(node *corev1.Node) bool {
	return true
}

// Cleanup logs node information
func (p *LoggerPlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	fmt.Printf("\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  🗑️  DELETING NODE: %s\n", node.Name)
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  Name:           %s\n", node.Name)
	fmt.Printf("  Created:        %s\n", node.CreationTimestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Deletion Time:  %s\n", node.DeletionTimestamp.Format("2006-01-02 15:04:05"))

	// Log labels
	if len(node.Labels) > 0 {
		fmt.Printf("  Labels:\n")
		for k, v := range node.Labels {
			fmt.Printf("    - %s: %s\n", k, v)
		}
	}

	// Log conditions
	if len(node.Status.Conditions) > 0 {
		fmt.Printf("  Conditions:\n")
		for _, cond := range node.Status.Conditions {
			fmt.Printf("    - %s: %s\n", cond.Type, cond.Status)
		}
	}

	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")

	klog.Infof("Logged deletion info for node: %s", node.Name)
	return nil
}
