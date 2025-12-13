# Insecure kube-apiserver Configuration

## Overview

If your Kubernetes cluster has a kube-apiserver with invalid, self-signed, or untrusted TLS certificates, you can configure the webhook to skip TLS verification when making API calls.

**⚠️ WARNING:** This is **NOT RECOMMENDED** for production environments as it makes the webhook vulnerable to man-in-the-middle attacks.

## When to Use This

- **Development environments** with self-signed certificates
- **Testing environments** without proper CA infrastructure
- **Legacy clusters** that cannot be upgraded to use proper certificates
- **Air-gapped environments** with custom certificate authorities

## Configuration

### Option 1: Helm Chart (Recommended)

Edit `values.yaml`:

```yaml
kubeClient:
  insecureSkipTLSVerify: true  # Skip TLS verification for kube-apiserver
```

Deploy:

```bash
helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace \
  --set kubeClient.insecureSkipTLSVerify=true
```

### Option 2: Environment Variable

For local development or raw manifests:

```bash
export INSECURE_SKIP_TLS_VERIFY=true
make run-local
```

For Kubernetes deployment, edit `deploy/manifests/deployment.yaml`:

```yaml
env:
  - name: INSECURE_SKIP_TLS_VERIFY
    value: "true"
```

### Option 3: Command Line Flag (Future)

```bash
./webhook --insecure-skip-tls-verify=true
```

## What Gets Affected

### ✅ Affects (Outgoing Connections)

- Kubernetes API calls from webhook to kube-apiserver
- Node GET/LIST/WATCH operations
- Node PATCH/UPDATE operations (finalizer removal)
- Event creation

### ❌ Does NOT Affect (Incoming Connections)

- Webhook TLS certificates (still required!)
- Incoming mutating webhook requests from kube-apiserver
- Liveness/readiness probe endpoints

## Important Notes

1. **Webhook still needs TLS certificates**: Even with `insecureSkipTLSVerify=true`, the webhook server itself must have valid TLS certificates for incoming connections from kube-apiserver.

2. **Two different TLS connections**:
   - **Incoming**: kube-apiserver → webhook (needs webhook TLS certs)
   - **Outgoing**: webhook → kube-apiserver (can skip verification with this setting)

3. **Security implications**:
   - Disabling TLS verification exposes the webhook to MITM attacks
   - An attacker could intercept and modify API responses
   - Use only in trusted, isolated networks

## Complete Example

### Insecure Development Environment

```yaml
# values.yaml
kubeClient:
  insecureSkipTLSVerify: true

webhook:
  certManager:
    enabled: false  # Use manual certificates
  caBundle: "LS0t..." # Base64 CA cert for webhook

env:
  ENABLED_PLUGINS: "logger"
```

### Secure Production Environment

```yaml
# values.yaml
kubeClient:
  insecureSkipTLSVerify: false  # Default - verify TLS

webhook:
  certManager:
    enabled: true  # Use cert-manager
    issuerRef:
      name: production-issuer
      kind: ClusterIssuer

env:
  ENABLED_PLUGINS: "logger,portworx"
```

## Verification

Check if the setting is applied:

```bash
# View webhook logs at startup
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook

# You should see:
# Configuration:
#   TLS Cert: /etc/webhook/certs/tls.crt
#   TLS Key: /etc/webhook/certs/tls.key
#   Port: 8443
#   Insecure Skip TLS Verify: true  ← Should be true
#   Enabled Plugins: [logger]
```

If you see the warning message, verification is disabled:

```
⚠️  TLS verification disabled for kube-apiserver - NOT RECOMMENDED for production!
```

## Troubleshooting

### Error: "x509: certificate signed by unknown authority"

**Without insecureSkipTLSVerify:**
```
Failed to get node: Get "https://10.96.0.1:443/api/v1/nodes/test-node":
x509: certificate signed by unknown authority
```

**Solution:** Set `insecureSkipTLSVerify: true`

### Error: "connection refused"

This error is **not** related to TLS verification. Check:
- kube-apiserver is running
- Network connectivity
- Service account permissions

### Webhook still requires certificates

**Error:**
```
Webhook server failed: tls: failed to find any PEM data in certificate input
```

**Solution:** This is about **incoming** webhook certificates, not outgoing API calls. You still need to generate webhook certificates:

```bash
# Manual certificates
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout tls.key -out tls.crt \
  -days 365 -subj "/CN=node-cleanup-webhook.node-cleanup-system.svc"

kubectl create secret tls node-cleanup-webhook-tls \
  --cert=tls.crt --key=tls.key \
  -n node-cleanup-system
```

## Alternative: Use Proper CA Certificates

Instead of disabling verification, consider:

1. **Mount CA certificate into the webhook pod**:
   ```yaml
   volumeMounts:
     - name: ca-cert
       mountPath: /etc/ssl/certs/ca.crt
       subPath: ca.crt
   volumes:
     - name: ca-cert
       configMap:
         name: kube-root-ca.crt
   ```

2. **Use in-cluster config** (default): The webhook automatically uses the service account token and CA from `/var/run/secrets/kubernetes.io/serviceaccount/`.

## References

- [Kubernetes API Access Control](https://kubernetes.io/docs/concepts/security/controlling-access/)
- [TLS Bootstrapping](https://kubernetes.io/docs/reference/access-authn-authz/kubelet-tls-bootstrapping/)
- [Air-Gapped Deployment Guide](./AIR_GAPPED_DEPLOYMENT.md)
