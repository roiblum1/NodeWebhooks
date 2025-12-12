# Architecture

This document describes the architecture and design decisions for the Node Cleanup Webhook.

## Design Philosophy

### Why a Webhook Instead of an Operator?

| Aspect | Operator Approach | Webhook Approach (Our Choice) |
|--------|-------------------|-------------------------------|
| Add finalizer | Watch nodes, add finalizer (race condition possible) | Mutate CREATE request (atomic, guaranteed) |
| Complexity | Full reconciliation loop, leader election, state management | Stateless HTTP handler |
| Failure mode | If operator is down, new nodes don't get finalizer | `failurePolicy: Ignore` allows nodes to be created |
| Resource usage | Higher (continuous reconciliation) | Lower (event-driven) |

**Conclusion**: A webhook is simpler, more reliable, and sufficient for our use case.

## Components

### 1. Mutating Admission Webhook

**Purpose**: Atomically add finalizer to nodes at creation time.

**How it works**:
1. Kubernetes API server calls webhook for all Node CREATE operations
2. Webhook adds `infra.894.io/node-cleanup` finalizer via JSON patch
3. Node is created with finalizer already attached

**Code**: [`pkg/webhook/server.go`](../pkg/webhook/server.go)

**Key features**:
- Stateless operation
- Only intercepts CREATE operations
- `failurePolicy: Ignore` - allows node creation if webhook unavailable
- TLS-secured endpoint

### 2. Cleanup Watcher

**Purpose**: Watch for node deletions and orchestrate cleanup.

**How it works**:
1. Uses client-go informer to watch node events
2. On startup, adds finalizers to all existing nodes
3. On new node creation/update, ensures finalizer is present
4. When node deletion is requested:
   - Node gets `deletionTimestamp` but isn't deleted (blocked by finalizer)
   - Watcher detects this state
   - Runs cleanup tasks
   - Removes finalizer
   - Kubernetes completes deletion

**Code**: [`pkg/watcher/watcher.go`](../pkg/watcher/watcher.go)

**Key features**:
- Event-driven with work queue
- Retry logic with backoff
- Idempotent cleanup
- Emergency bypass via annotation

### 3. Single Binary

Both webhook and watcher run in the same binary:
- Simplifies deployment (one container, one deployment)
- Shares Kubernetes client
- Single configuration

**Code**: [`cmd/webhook/main.go`](../cmd/webhook/main.go)

## Data Flow

```
┌─────────────────────────────────────────────────────────────────┐
│ kubectl create node                                             │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│ Kubernetes API Server                                           │
│                                                                 │
│ 1. Receives CREATE request                                     │
│ 2. Calls mutating webhook /mutate-node                         │
│ 3. Webhook returns JSON patch with finalizer                   │
│ 4. API server applies patch                                    │
│ 5. Node created with finalizer                                 │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│ Watcher Informer                                                │
│                                                                 │
│ - Receives ADD event for new node                              │
│ - Verifies finalizer is present                                │
│ - (No action needed, webhook already added it)                 │
└─────────────────────────────────────────────────────────────────┘

... time passes ...

┌─────────────────────────────────────────────────────────────────┐
│ kubectl delete node                                             │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│ Kubernetes API Server                                           │
│                                                                 │
│ 1. Sets node.deletionTimestamp                                 │
│ 2. Does NOT delete node (finalizer blocks it)                  │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│ Watcher Informer                                                │
│                                                                 │
│ 1. Receives UPDATE event                                       │
│ 2. Sees deletionTimestamp != nil && finalizer present          │
│ 3. Enqueues node for cleanup                                   │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│ Cleanup Worker                                                  │
│                                                                 │
│ 1. Runs cleanup tasks (Portworx, storage, notifications)       │
│ 2. On success: removes finalizer                               │
│ 3. On failure: retries with backoff                            │
└─────────────────────┬───────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────┐
│ Kubernetes API Server                                           │
│                                                                 │
│ 1. Finalizer removed                                           │
│ 2. No finalizers remaining                                     │
│ 3. Deletes node                                                │
└─────────────────────────────────────────────────────────────────┘
```

## Failure Modes

### Webhook Unavailable During Node Creation

**Scenario**: Webhook pods are down when new node joins cluster.

**Behavior**:
- `failurePolicy: Ignore` allows node creation without finalizer
- Watcher detects new node via informer
- Watcher adds finalizer asynchronously

**Impact**: Minimal - finalizer added within seconds

### Watcher Down During Node Deletion

**Scenario**: Watcher pods are down when node deletion is requested.

**Behavior**:
- Node gets `deletionTimestamp` and stays in Terminating state
- When watcher restarts, informer redelivers events
- Cleanup runs and completes

**Impact**: Delayed deletion, but cleanup still runs

### Cleanup Failures

**Scenario**: Cleanup task fails (e.g., Portworx unreachable).

**Behavior**:
- Error logged
- Cleanup retried after 10s backoff
- Continues retrying until success

**Recovery**:
- Fix underlying issue (e.g., restore Portworx)
- OR add skip annotation: `kubectl annotate node <name> infra.894.io/skip-cleanup=true`

### Split Brain (Network Partition)

**Scenario**: Watcher can't reach Kubernetes API.

**Behavior**:
- Informer cache becomes stale
- Cleanup operations fail (can't patch nodes)
- Health check fails

**Recovery**: Kubernetes restarts pod

## Scaling Considerations

### Horizontal Scaling

- Multiple webhook replicas (default: 2)
- Kubernetes load balances webhook requests
- All replicas can handle requests (stateless)

### Watcher Concurrency

- Each watcher replica runs its own informer
- Work queue processes one node at a time per replica
- Multiple replicas can process different nodes simultaneously
- Processing map prevents duplicate work

### Resource Usage

**Per replica**:
- CPU: 50m request, 200m limit
- Memory: 64Mi request, 128Mi limit

**Bottlenecks**:
- Cleanup duration (external API calls)
- Not CPU or memory bound

## Security

### Pod Security

- Runs as non-root (UID 65532)
- Read-only root filesystem
- No privilege escalation
- All capabilities dropped

### RBAC

Minimal required permissions:
- `nodes`: get, list, watch, patch, update
- `events`: create, patch (optional)

### TLS Certificates

Two options:
1. **cert-manager** (recommended): Auto-generates and rotates certificates
2. **Manual**: Generate with scripts, manage rotation manually

### Network Policies

Not included by default. Example:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: node-cleanup-webhook
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: node-cleanup-webhook
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8443
  egress:
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 6443  # Kubernetes API
```

## Monitoring

### Metrics

Currently not implemented. Future additions:
- `webhook_requests_total` - Counter of webhook requests
- `webhook_request_duration_seconds` - Histogram of webhook latency
- `cleanup_operations_total` - Counter of cleanup operations
- `cleanup_duration_seconds` - Histogram of cleanup duration
- `cleanup_failures_total` - Counter of failed cleanups

### Health Checks

- `/healthz` - Always returns 200 if process is alive
- `/readyz` - Returns 200 if informer cache is synced

### Logging

Structured logging with klog:
- Verbosity levels 0-4
- Key events logged at appropriate levels
- Errors include node name for troubleshooting

## Future Enhancements

1. **Metrics and Monitoring**: Prometheus metrics for observability
2. **Webhook for Updates**: Block certain node updates
3. **Configurable Cleanup**: Load cleanup tasks from ConfigMap
4. **Parallel Cleanup**: Run independent cleanup tasks concurrently
5. **Status Reporting**: Update node annotations with cleanup progress
6. **Event Recording**: Create Kubernetes events for cleanup lifecycle
