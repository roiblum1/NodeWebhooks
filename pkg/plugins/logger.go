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

// Cleanup logs node information using structured logging
func (p *LoggerPlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	// Structured log with all node metadata
	klog.InfoS("Node deletion initiated",
		"node", node.Name,
		"createdAt", node.CreationTimestamp.Time,
		"deletionTimestamp", node.DeletionTimestamp.Time,
		"uid", node.UID,
		"labelCount", len(node.Labels),
		"conditionCount", len(node.Status.Conditions),
	)

	// Log labels as structured data
	if len(node.Labels) > 0 {
		klog.V(1).InfoS("Node labels", "node", node.Name, "labels", node.Labels)

		// Also print for human readability
		fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
		fmt.Printf("  🗑️  DELETING NODE: %s\n", node.Name)
		fmt.Printf("═══════════════════════════════════════════════════════════════\n")
		fmt.Printf("  Labels:\n")
		for k, v := range node.Labels {
			fmt.Printf("    - %s: %s\n", k, v)
		}
	}

	// Log conditions as structured data
	if len(node.Status.Conditions) > 0 {
		conditions := make(map[string]string)
		for _, cond := range node.Status.Conditions {
			conditions[string(cond.Type)] = string(cond.Status)
		}
		klog.V(1).InfoS("Node conditions", "node", node.Name, "conditions", conditions)

		fmt.Printf("  Conditions:\n")
		for _, cond := range node.Status.Conditions {
			fmt.Printf("    - %s: %s (reason: %s)\n", cond.Type, cond.Status, cond.Reason)
		}
		fmt.Printf("═══════════════════════════════════════════════════════════════\n\n")
	}

	return nil
}
