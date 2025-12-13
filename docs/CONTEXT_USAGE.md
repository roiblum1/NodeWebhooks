# Context Usage in Go - Best Practices

## Why Always Use Context?

Context is a fundamental part of Go programming for production systems. Here's why you should **always** use context:

## 1. Cancellation Propagation

When a request is cancelled (user closes connection, timeout occurs, process shuts down), all downstream operations should stop immediately.

### Without Context (BAD):
```go
func processNode(nodeName string) error {
    // This goroutine leaks if parent operation is cancelled
    go func() {
        for {
            time.Sleep(1 * time.Second)
            checkStatus(nodeName) // Runs forever!
        }
    }()
    return nil
}
```

### With Context (GOOD):
```go
func processNode(ctx context.Context, nodeName string) error {
    go func() {
        ticker := time.NewTicker(1 * time.Second)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                checkStatus(nodeName)
            case <-ctx.Done():
                klog.InfoS("Stopping status checks", "node", nodeName)
                return  // Cleanup when cancelled
            }
        }
    }()
    return nil
}
```

## 2. Timeout Enforcement

Prevents operations from hanging forever, especially critical for external API calls.

### Without Context (BAD):
```go
func decommissionNode(nodeName string) error {
    // This could hang forever if API doesn't respond
    resp, err := http.Get("http://api.example.com/decommission/" + nodeName)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```

### With Context (GOOD):
```go
func decommissionNode(ctx context.Context, nodeName string) error {
    // Automatically cancelled after 30 seconds
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET",
        "http://api.example.com/decommission/"+nodeName, nil)
    if err != nil {
        return err
    }

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    return nil
}
```

## 3. Resource Cleanup

Ensures goroutines don't leak and resources are properly released.

### Example: Kubernetes Informer
```go
func (w *Watcher) Run(ctx context.Context) {
    // Start informer
    go w.informer.Run(ctx.Done())  // Stops when context is cancelled

    // Process work queue
    for {
        select {
        case nodeName := <-w.workqueue:
            w.processNode(ctx, nodeName)
        case <-ctx.Done():
            klog.InfoS("Watcher stopping - context cancelled")
            return  // Clean shutdown
        }
    }
}
```

## 4. Request-Scoped Values

Carry request-specific data (request IDs, auth tokens, trace IDs) across function boundaries.

```go
type contextKey string

const requestIDKey contextKey = "requestID"

func processWithTracing(ctx context.Context, node *corev1.Node) error {
    requestID := uuid.New().String()
    ctx = context.WithValue(ctx, requestIDKey, requestID)

    klog.InfoS("Processing node",
        "node", node.Name,
        "requestID", requestID)

    return performCleanup(ctx, node)
}

func performCleanup(ctx context.Context, node *corev1.Node) error {
    if requestID, ok := ctx.Value(requestIDKey).(string); ok {
        klog.InfoS("Cleanup started",
            "node", node.Name,
            "requestID", requestID)
    }
    // ... cleanup logic
    return nil
}
```

## 5. Graceful Shutdown

Handle application shutdown cleanly.

```go
func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown signals
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigCh
        klog.InfoS("Shutdown signal received")
        cancel()  // Cancels all child contexts
    }()

    // Start services
    watcher.Run(ctx)      // Stops when ctx is cancelled
    webhookServer.Run(ctx) // Stops when ctx is cancelled
}
```

## Real-World Examples from This Project

### Plugin Execution with Timeout

```go
func (r *Registry) RunAll(ctx context.Context, node *corev1.Node) error {
    for _, name := range r.pluginOrder {
        plugin := r.plugins[name]

        // Add timeout per plugin
        pluginCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
        defer cancel()

        if err := plugin.Cleanup(pluginCtx, node); err != nil {
            if pluginCtx.Err() == context.DeadlineExceeded {
                return fmt.Errorf("plugin %s timed out after 5 minutes", name)
            }
            return fmt.Errorf("plugin %s failed: %w", name, err)
        }
    }
    return nil
}
```

### Retry with Context

```go
func (w *Watcher) processNode(ctx context.Context, nodeName string) {
    cleanupErr := w.runCleanup(ctx, node)
    if cleanupErr != nil {
        // Re-enqueue for retry after backoff
        go func() {
            timer := time.NewTimer(10 * time.Second)
            defer timer.Stop()

            select {
            case <-timer.C:
                // Re-fetch and re-enqueue
                if n, err := w.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{}); err == nil {
                    w.enqueueIfDeleting(n)
                }
            case <-ctx.Done():
                // Shutdown before retry - don't re-enqueue
                klog.InfoS("Retry cancelled - shutting down", "node", nodeName)
                return
            }
        }()
    }
}
```

### Database Query with Context

```go
func getUserData(ctx context.Context, userID string) (*User, error) {
    // Context ensures query is cancelled if:
    // - HTTP request is cancelled
    // - Server is shutting down
    // - Timeout is exceeded

    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    row := db.QueryRowContext(ctx, "SELECT * FROM users WHERE id = $1", userID)

    var user User
    if err := row.Scan(&user.ID, &user.Name); err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return nil, fmt.Errorf("query timed out")
        }
        return nil, err
    }

    return &user, nil
}
```

## Context Best Practices

### ✅ DO:
1. **Always pass context as first parameter**: `func process(ctx context.Context, ...)`
2. **Accept context in all long-running functions**
3. **Respect context cancellation**: Check `ctx.Done()` in loops
4. **Create child contexts with timeout**: `context.WithTimeout(ctx, duration)`
5. **Use context for HTTP requests**: `http.NewRequestWithContext(ctx, ...)`
6. **Propagate context through call chains**

### ❌ DON'T:
1. **Don't store context in structs** (except rare cases like servers)
2. **Don't pass nil context** - use `context.Background()` or `context.TODO()`
3. **Don't ignore context.Err()** - always check why operation failed
4. **Don't forget to call cancel()** - use `defer cancel()`
5. **Don't use context for optional parameters** - use function arguments

## Common Patterns

### Pattern 1: HTTP Client with Context
```go
client := &http.Client{
    Timeout: 30 * time.Second,
}

req, err := http.NewRequestWithContext(ctx, "POST", url, body)
if err != nil {
    return err
}

resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()
```

### Pattern 2: Worker Pool with Context
```go
func worker(ctx context.Context, jobs <-chan Job, results chan<- Result) {
    for {
        select {
        case job := <-jobs:
            result := process(ctx, job)
            results <- result
        case <-ctx.Done():
            return
        }
    }
}

// Start workers
for i := 0; i < numWorkers; i++ {
    go worker(ctx, jobs, results)
}
```

### Pattern 3: Kubernetes Client Operations
```go
func deleteNode(ctx context.Context, client kubernetes.Interface, name string) error {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    err := client.CoreV1().Nodes().Delete(ctx, name, metav1.DeleteOptions{})
    if err != nil {
        return fmt.Errorf("failed to delete node: %w", err)
    }
    return nil
}
```

## Context Errors

Always check what error occurred:

```go
err := someOperation(ctx)
if err != nil {
    switch {
    case errors.Is(err, context.Canceled):
        // Parent cancelled the operation
        klog.InfoS("Operation cancelled by user")
    case errors.Is(err, context.DeadlineExceeded):
        // Operation timed out
        klog.ErrorS(err, "Operation timed out")
    default:
        // Other error
        klog.ErrorS(err, "Operation failed")
    }
    return err
}
```

## Summary

**Context is essential for:**
- ⏱️ Timeout enforcement
- 🛑 Cancellation propagation
- 🧹 Resource cleanup
- 📊 Request tracing
- 🔒 Graceful shutdown

**Always use context in:**
- HTTP requests
- Database queries
- External API calls
- Long-running operations
- Goroutines
- Kubernetes client operations

By following these practices, your code will be more robust, maintainable, and production-ready.
