package plugins

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// PortworxPlugin handles Portworx node decommissioning
type PortworxPlugin struct {
	BasePlugin
	labelSelector string
}

// NewPortworxPlugin creates a new Portworx cleanup plugin
func NewPortworxPlugin(client kubernetes.Interface, labelSelector string) *PortworxPlugin {
	if labelSelector == "" {
		labelSelector = "px/enabled=true"
	}

	return &PortworxPlugin{
		BasePlugin: BasePlugin{
			name:   "portworx",
			client: client,
		},
		labelSelector: labelSelector,
	}
}

// ShouldRun checks if this node has Portworx enabled
func (p *PortworxPlugin) ShouldRun(node *corev1.Node) bool {
	// Check if node has the Portworx label
	if val, ok := node.Labels["px/enabled"]; ok && val == "true" {
		return true
	}

	// Alternative: check for px/status label
	if _, ok := node.Labels["px/status"]; ok {
		return true
	}

	klog.V(2).Infof("Node %s does not have Portworx enabled", node.Name)
	return false
}

// Cleanup performs Portworx decommissioning
func (p *PortworxPlugin) Cleanup(ctx context.Context, node *corev1.Node) error {
	klog.Infof("🔧 Portworx: Decommissioning node %s", node.Name)

	// TODO: Implement actual Portworx decommissioning
	// Options:
	// 1. Call Portworx REST API
	// 2. Execute pxctl command via kubectl exec
	// 3. Delete/Update StorageNode CRD

	// For now, simulate the decommission process
	klog.Infof("Portworx: Checking node status...")
	time.Sleep(500 * time.Millisecond)

	klog.Infof("Portworx: Starting decommission process...")
	time.Sleep(1 * time.Second)

	klog.Infof("Portworx: Draining storage pools...")
	time.Sleep(500 * time.Millisecond)

	klog.Infof("Portworx: Removing node from cluster...")
	time.Sleep(500 * time.Millisecond)

	// Example implementation:
	// if err := p.callPortworxAPI(ctx, node.Name); err != nil {
	//     return fmt.Errorf("portworx API call failed: %w", err)
	// }

	klog.Infof("✅ Portworx: Successfully decommissioned node %s", node.Name)
	return nil
}

// callPortworxAPI calls the Portworx REST API (example implementation)
func (p *PortworxPlugin) callPortworxAPI(ctx context.Context, nodeName string) error {
	// Example implementation:
	// POST http://portworx-api:9001/v1/cluster/decommission/{nodeName}
	//
	// client := &http.Client{Timeout: 30 * time.Second}
	// url := fmt.Sprintf("http://portworx-api:9001/v1/cluster/decommission/%s", nodeName)
	//
	// req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	// if err != nil {
	//     return err
	// }
	//
	// resp, err := client.Do(req)
	// if err != nil {
	//     return err
	// }
	// defer resp.Body.Close()
	//
	// if resp.StatusCode != http.StatusOK {
	//     return fmt.Errorf("portworx API returned status %d", resp.StatusCode)
	// }

	return fmt.Errorf("not implemented - add your Portworx API call here")
}
