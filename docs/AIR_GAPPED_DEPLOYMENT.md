# Air-Gapped / Disconnected Environment Deployment

This guide explains how to deploy the Node Cleanup Webhook in air-gapped or disconnected environments where there's no internet access.

## Overview

The project is configured to work in air-gapped environments by:
- ✅ **Vendoring all Go dependencies** - No need for internet during build
- ✅ **Using `-mod=vendor` flag** - Build uses local vendor directory
- ✅ **Committing vendor/ to git** - Dependencies are part of the repository
- ✅ **No external linter dependencies** - Uses built-in `go vet` and `go fmt`

## Prerequisites

### Connected Environment (Preparation)
- Git
- Go 1.21+
- Podman or Docker
- Access to the internet

### Air-Gapped Environment (Deployment)
- Podman or Docker
- Kubernetes cluster
- kubectl
- Helm (optional)

## Step-by-Step Guide

### Phase 1: Preparation (Connected Environment)

#### 1. Clone the Repository
```bash
git clone https://github.com/your-org/node-cleanup-webhook.git
cd node-cleanup-webhook
```

#### 2. Verify Vendor Directory
The vendor directory is already committed, but you can regenerate it if needed:

```bash
# Verify vendor directory exists
ls -lh vendor/

# Regenerate if needed
make vendor

# Verify all dependencies are vendored
go list -mod=vendor all
```

#### 3. Build Container Image
```bash
# Build image with vendor dependencies
podman build -t node-cleanup-webhook:v1.0.0 .

# Verify image was built
podman images | grep node-cleanup-webhook
```

#### 4. Export Image for Air-Gapped Transfer
```bash
# Export image to tar file
podman save node-cleanup-webhook:v1.0.0 -o node-cleanup-webhook-v1.0.0.tar

# Verify tar file
ls -lh node-cleanup-webhook-v1.0.0.tar
```

#### 5. Prepare Deployment Files
```bash
# Create deployment bundle
mkdir -p airgap-bundle
cp -r deploy/ airgap-bundle/
cp -r docs/ airgap-bundle/
cp README.md airgap-bundle/
cp .env.example airgap-bundle/

# Create tarball
tar -czf airgap-bundle.tar.gz airgap-bundle/
```

#### 6. Transfer to Air-Gapped Environment
Transfer these files to the air-gapped environment:
- `node-cleanup-webhook-v1.0.0.tar` (container image)
- `airgap-bundle.tar.gz` (deployment files)

**Transfer methods:**
- USB drive
- Secure file transfer
- Physical media
- Internal network share

---

### Phase 2: Deployment (Air-Gapped Environment)

#### 1. Load Container Image
```bash
# Load image from tar
podman load -i node-cleanup-webhook-v1.0.0.tar

# Verify image loaded
podman images | grep node-cleanup-webhook
```

#### 2. Tag and Push to Internal Registry
```bash
# Tag for internal registry
podman tag node-cleanup-webhook:v1.0.0 \
  internal-registry.company.local/infra/node-cleanup-webhook:v1.0.0

# Login to internal registry
podman login internal-registry.company.local

# Push to internal registry
podman push internal-registry.company.local/infra/node-cleanup-webhook:v1.0.0
```

#### 3. Extract Deployment Files
```bash
# Extract bundle
tar -xzf airgap-bundle.tar.gz
cd airgap-bundle/
```

#### 4. Configure for Internal Registry
```bash
# Update Helm values
cat > custom-values.yaml <<EOF
image:
  repository: internal-registry.company.local/infra/node-cleanup-webhook
  tag: v1.0.0
  pullPolicy: IfNotPresent

# If using private registry authentication
imagePullSecrets:
  - name: internal-registry-secret
EOF
```

#### 5. Deploy with Helm
```bash
# Install with custom values
helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace \
  -f custom-values.yaml
```

**OR** Deploy with raw manifests:
```bash
# Update image in deployment.yaml
sed -i 's|registry.example.com/node-cleanup-webhook:latest|internal-registry.company.local/infra/node-cleanup-webhook:v1.0.0|g' \
  deploy/manifests/deployment.yaml

# Apply manifests
kubectl create namespace node-cleanup-system
kubectl apply -f deploy/manifests/
```

#### 6. Verify Deployment
```bash
# Check pods
kubectl get pods -n node-cleanup-system

# Check logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook

# Verify webhook configuration
kubectl get mutatingwebhookconfiguration node-cleanup-webhook
```

---

## Building from Source (Air-Gapped)

If you need to build the binary locally in the air-gapped environment:

### Prerequisites
- Go 1.21+ installed
- Source code with vendor/ directory

### Build Binary
```bash
# Build using vendored dependencies
go build -mod=vendor -o bin/webhook ./cmd/webhook

# Verify binary
./bin/webhook --version
```

### Build Container Image
```bash
# The Dockerfile uses vendor directory automatically
podman build -t node-cleanup-webhook:custom .
```

---

## Updating Dependencies (Future Updates)

When you need to update dependencies in the future:

### In Connected Environment:
```bash
# Update dependencies
go get -u ./...
go mod tidy

# Re-vendor dependencies
go mod vendor

# Commit vendor changes
git add vendor/
git commit -m "Update vendored dependencies"

# Rebuild image
podman build -t node-cleanup-webhook:v1.1.0 .
podman save node-cleanup-webhook:v1.1.0 -o node-cleanup-webhook-v1.1.0.tar
```

### Transfer and Deploy:
Follow Phase 2 steps again with the new image.

---

## Verification Checklist

### Before Transfer ✅
- [ ] Vendor directory exists and is complete
- [ ] Container image builds successfully
- [ ] Image tar file created
- [ ] Deployment bundle created
- [ ] Files transferred to air-gapped environment

### After Deployment ✅
- [ ] Image loaded successfully
- [ ] Image pushed to internal registry
- [ ] Pods are running
- [ ] Webhook is responding
- [ ] Finalizers being added to nodes
- [ ] Cleanup working on node deletion

---

## Testing in Air-Gapped Environment

### Test 1: Verify Build Works
```bash
# Build using vendor
go build -mod=vendor -o webhook-test ./cmd/webhook
./webhook-test --help
rm webhook-test
```

### Test 2: Verify Image Works
```bash
# Run container locally
podman run --rm node-cleanup-webhook:v1.0.0 --help
```

### Test 3: Verify Dependencies
```bash
# List all vendored dependencies
go list -mod=vendor -m all

# Verify no external downloads needed
go build -mod=vendor -n ./cmd/webhook 2>&1 | grep -i download
# Should show no downloads
```

### Test 4: Verify Webhook
```bash
# Create test node
kubectl create -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  name: test-airgap-node
spec:
  podCIDR: 10.244.9.0/24
EOF

# Check finalizer added
kubectl get node test-airgap-node -o jsonpath='{.metadata.finalizers}'
# Should show: ["infra.894.io/node-cleanup"]

# Delete and verify cleanup
kubectl delete node test-airgap-node

# Check logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook --tail=50
```

---

## Troubleshooting

### Issue: "go: missing module..."
**Cause**: Vendor directory is incomplete or not used

**Solution**:
```bash
# Ensure using vendor mode
go build -mod=vendor ./cmd/webhook

# Or regenerate vendor
go mod vendor
```

### Issue: "Image pull failed"
**Cause**: Image not in internal registry or wrong image name

**Solution**:
```bash
# Verify image in registry
podman search internal-registry.company.local/infra/node-cleanup-webhook

# Check image pull secrets
kubectl get secret internal-registry-secret -n node-cleanup-system

# Verify deployment uses correct image
kubectl get deployment -n node-cleanup-system node-cleanup-webhook -o jsonpath='{.spec.template.spec.containers[0].image}'
```

### Issue: "Certificate validation failed"
**Cause**: cert-manager not available in air-gapped environment

**Solution**: Use manual certificates (see [ARCHITECTURE.md](ARCHITECTURE.md))
```bash
# Generate certificates manually
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout tls.key -out tls.crt \
  -days 365 -subj "/CN=node-cleanup-webhook.node-cleanup-system.svc"

# Create secret
kubectl create secret tls webhook-certs \
  --cert=tls.crt --key=tls.key \
  -n node-cleanup-system

# Update deployment to use secret
kubectl patch deployment node-cleanup-webhook -n node-cleanup-system \
  -p '{"spec":{"template":{"spec":{"volumes":[{"name":"certs","secret":{"secretName":"webhook-certs"}}]}}}}'
```

---

## Security Considerations

### Image Scanning
Before transferring to air-gapped environment:
```bash
# Scan image for vulnerabilities (in connected environment)
trivy image node-cleanup-webhook:v1.0.0

# Or use your organization's scanner
podman scan node-cleanup-webhook:v1.0.0
```

### Image Signing
Sign images before transfer:
```bash
# Sign with cosign
cosign sign node-cleanup-webhook:v1.0.0

# Verify signature
cosign verify node-cleanup-webhook:v1.0.0
```

### Supply Chain Security
- All dependencies are vendored and reviewed
- No runtime dependencies on external services
- Minimal distroless base image
- No CGO dependencies

---

## File Size Reference

Typical sizes for transfer:
- Container image tar: ~50-80 MB
- Deployment bundle: ~100 KB
- Vendor directory: ~46 MB
- Total: ~130 MB

---

## Quick Reference Commands

### Connected Environment
```bash
# Prepare for air-gapped
make vendor                    # Vendor dependencies
podman build -t webhook:v1 .   # Build image
podman save webhook:v1 -o webhook.tar  # Export image
```

### Air-Gapped Environment
```bash
# Deploy in air-gapped
podman load -i webhook.tar              # Load image
podman tag webhook:v1 registry/webhook  # Tag for internal registry
podman push registry/webhook            # Push to registry
helm install webhook ./deploy/helm/...  # Deploy
```

---

## Additional Resources

- [README.md](../README.md) - Main documentation
- [ARCHITECTURE.md](ARCHITECTURE.md) - Architecture details
- [DEVELOPMENT.md](DEVELOPMENT.md) - Development guide
- [Kubernetes docs on air-gapped](https://kubernetes.io/docs/tasks/administer-cluster/disconnected/)

---

**Summary**: This webhook is fully compatible with air-gapped environments. All dependencies are vendored, and no internet access is required during build or runtime.
