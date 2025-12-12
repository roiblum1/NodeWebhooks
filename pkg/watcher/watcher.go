package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/894/node-cleanup-webhook/pkg/plugins"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	// FinalizerName is the finalizer we watch for
	FinalizerName = "infra.894.io/node-cleanup"

	// SkipCleanupAnnotation allows bypassing cleanup in emergencies
	SkipCleanupAnnotation = "infra.894.io/skip-cleanup"
)

// Watcher watches for nodes being deleted and runs cleanup
type Watcher struct {
	client         kubernetes.Interface
	informer       cache.SharedIndexInformer
	workqueue      chan string
	pluginRegistry *plugins.Registry
	// Track nodes being processed to avoid duplicate work
	processing sync.Map
}

// New creates a new cleanup watcher
func New(client kubernetes.Interface, pluginRegistry *plugins.Registry) *Watcher {
	// Create informer factory
	factory := informers.NewSharedInformerFactory(client, 30*time.Second)
	nodeInformer := factory.Core().V1().Nodes().Informer()

	watcher := &Watcher{
		client:         client,
		informer:       nodeInformer,
		workqueue:      make(chan string, 100),
		pluginRegistry: pluginRegistry,
	}

	// Add event handlers
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*corev1.Node)
			watcher.ensureFinalizer(node)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			node := newObj.(*corev1.Node)
			watcher.ensureFinalizer(node)
			watcher.enqueueIfDeleting(node)
		},
		DeleteFunc: func(obj interface{}) {
			// Node is already gone, just log
			if node, ok := obj.(*corev1.Node); ok {
				klog.Infof("Node %s has been deleted", node.Name)
			}
		},
	})

	return watcher
}

// ensureFinalizer adds the finalizer to a node if it doesn't have it
func (w *Watcher) ensureFinalizer(node *corev1.Node) {
	// Skip if node is being deleted
	if node.DeletionTimestamp != nil {
		return
	}

	// Skip if finalizer already exists
	if containsFinalizer(node.Finalizers, FinalizerName) {
		return
	}

	// Add finalizer in the background
	go func() {
		ctx := context.Background()
		if err := w.addFinalizer(ctx, node); err != nil {
			klog.Errorf("Failed to add finalizer to node %s: %v", node.Name, err)
		} else {
			klog.Infof("✅ Added finalizer to node: %s", node.Name)
		}
	}()
}

func (w *Watcher) enqueueIfDeleting(node *corev1.Node) {
	// Only process nodes that are being deleted
	if node.DeletionTimestamp == nil {
		return
	}

	// Only process if our finalizer is present
	if !containsFinalizer(node.Finalizers, FinalizerName) {
		return
	}

	// Check if already being processed
	if _, loaded := w.processing.LoadOrStore(node.Name, true); loaded {
		klog.V(2).Infof("Node %s already being processed", node.Name)
		return
	}

	klog.Infof("Enqueueing node %s for cleanup", node.Name)
	w.workqueue <- node.Name
}

// Run starts the watcher
func (w *Watcher) Run(ctx context.Context) {
	klog.Info("Starting cleanup watcher")

	// Start the informer
	go w.informer.Run(ctx.Done())

	// Wait for cache sync
	if !cache.WaitForCacheSync(ctx.Done(), w.informer.HasSynced) {
		klog.Fatal("Failed to sync informer cache")
	}
	klog.Info("Informer cache synced")

	// Initialize finalizers on existing nodes
	if err := w.initializeExistingNodes(ctx); err != nil {
		klog.Warningf("Failed to initialize existing nodes: %v", err)
	}

	// Process work queue
	for {
		select {
		case nodeName := <-w.workqueue:
			w.processNode(ctx, nodeName)
		case <-ctx.Done():
			klog.Info("Cleanup watcher stopping")
			return
		}
	}
}

func (w *Watcher) processNode(ctx context.Context, nodeName string) {
	defer w.processing.Delete(nodeName)

	klog.Infof("Processing cleanup for node %s", nodeName)

	// Get current node state
	node, err := w.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", nodeName, err)
		return
	}

	// Double-check it's still being deleted with our finalizer
	if node.DeletionTimestamp == nil || !containsFinalizer(node.Finalizers, FinalizerName) {
		klog.V(2).Infof("Node %s no longer needs cleanup", nodeName)
		return
	}

	// Check for skip annotation
	if node.Annotations[SkipCleanupAnnotation] == "true" {
		klog.Warningf("Skip annotation present on node %s, bypassing cleanup", nodeName)
		if err := w.removeFinalizer(ctx, node); err != nil {
			klog.Errorf("Failed to remove finalizer from node %s: %v", nodeName, err)
		}
		return
	}

	// Run cleanup
	cleanupErr := w.runCleanup(ctx, node)
	if cleanupErr != nil {
		klog.Errorf("Cleanup failed for node %s: %v", nodeName, cleanupErr)

		// Re-enqueue for retry after backoff
		go func() {
			time.Sleep(10 * time.Second)
			w.processing.Delete(nodeName)
			// Re-fetch and re-enqueue
			if n, err := w.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{}); err == nil {
				w.enqueueIfDeleting(n)
			}
		}()
		return
	}

	// Cleanup succeeded - remove finalizer
	klog.Infof("Cleanup completed for node %s, removing finalizer", nodeName)

	// Re-fetch node to get latest version
	node, err = w.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get node %s for finalizer removal: %v", nodeName, err)
		return
	}

	if err := w.removeFinalizer(ctx, node); err != nil {
		klog.Errorf("Failed to remove finalizer from node %s: %v", nodeName, err)
		return
	}

	klog.Infof("Successfully cleaned up node %s", nodeName)
}

func (w *Watcher) runCleanup(ctx context.Context, node *corev1.Node) error {
	klog.Infof("🗑️  Running cleanup for node: %s", node.Name)

	// Run all enabled plugins
	if err := w.pluginRegistry.RunAll(ctx, node); err != nil {
		return fmt.Errorf("plugin execution failed: %w", err)
	}

	klog.Infof("✅ All cleanup plugins completed for node: %s", node.Name)
	return nil
}

func (w *Watcher) removeFinalizer(ctx context.Context, node *corev1.Node) error {
	// Build new finalizers list without our finalizer
	newFinalizers := []string{}
	for _, f := range node.Finalizers {
		if f != FinalizerName {
			newFinalizers = append(newFinalizers, f)
		}
	}

	// Create patch
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": newFinalizers,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = w.client.CoreV1().Nodes().Patch(
		ctx,
		node.Name,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}

	klog.Infof("Removed finalizer from node %s", node.Name)
	return nil
}

// initializeExistingNodes adds finalizers to all existing nodes that don't have them
func (w *Watcher) initializeExistingNodes(ctx context.Context) error {
	klog.Info("Initializing finalizers on existing nodes...")

	// List all nodes
	nodes, err := w.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	addedCount := 0
	skippedCount := 0

	for _, node := range nodes.Items {
		// Skip if node is already being deleted
		if node.DeletionTimestamp != nil {
			klog.V(2).Infof("Skipping node %s (already being deleted)", node.Name)
			skippedCount++
			continue
		}

		// Check if finalizer already exists
		if containsFinalizer(node.Finalizers, FinalizerName) {
			klog.V(2).Infof("Node %s already has finalizer", node.Name)
			skippedCount++
			continue
		}

		// Add finalizer
		if err := w.addFinalizer(ctx, &node); err != nil {
			klog.Errorf("Failed to add finalizer to node %s: %v", node.Name, err)
			continue
		}

		klog.Infof("✅ Added finalizer to existing node: %s", node.Name)
		addedCount++
	}

	klog.Infof("Initialization complete: added finalizers to %d nodes, skipped %d nodes", addedCount, skippedCount)
	return nil
}

// addFinalizer adds the cleanup finalizer to a node
func (w *Watcher) addFinalizer(ctx context.Context, node *corev1.Node) error {
	// Build new finalizers list with our finalizer
	newFinalizers := append([]string{}, node.Finalizers...)
	newFinalizers = append(newFinalizers, FinalizerName)

	// Create patch
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"finalizers": newFinalizers,
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = w.client.CoreV1().Nodes().Patch(
		ctx,
		node.Name,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}

	return nil
}

func containsFinalizer(finalizers []string, target string) bool {
	for _, f := range finalizers {
		if f == target {
			return true
		}
	}
	return false
}
