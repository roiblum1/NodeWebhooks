# Simple Air-Gapped Deployment Guide

**Simplest approach**: Just transfer the container image. No need to build in air-gapped environment.

## Overview

This is the **recommended approach** for air-gapped deployments:

1. **Connected environment**: Build container image
2. **Export**: `podman save` to tar file
3. **Transfer**: Copy tar to air-gapped environment
4. **Load**: `podman load` from tar
5. **Deploy**: Push to internal registry and deploy

**No compilation needed in air-gapped environment!**

---

## Step-by-Step Guide

### Phase 1: Connected Environment (Build & Export)

#### 1. Clone Repository
```bash
git clone <your-repo-url>
cd node-cleanup-webhook
```

The repository includes:
- ✅ All source code
- ✅ Vendored dependencies (3,361 files, 46 MB)
- ✅ Helm charts
- ✅ Documentation

#### 2. Build Container Image
```bash
# Build image (uses vendored dependencies)
podman build -t node-cleanup-webhook:v1.0.0 .

# Verify build
podman images | grep node-cleanup-webhook
```

**What happens during build:**
- Uses `golang:1.21-alpine` base image
- Copies vendor/ directory (no internet download needed)
- Builds with `-mod=vendor` flag
- Creates minimal distroless final image (~50-80 MB)

#### 3. Export Image to Tar
```bash
# Export image
podman save node-cleanup-webhook:v1.0.0 -o node-cleanup-webhook-v1.0.0.tar

# Check file size
ls -lh node-cleanup-webhook-v1.0.0.tar
# ~50-80 MB
```

#### 4. Prepare Deployment Files
```bash
# Create bundle directory
mkdir airgap-transfer
cp node-cleanup-webhook-v1.0.0.tar airgap-transfer/
cp -r deploy/ airgap-transfer/
cp README.md airgap-transfer/
cp docs/AIR_GAPPED_DEPLOYMENT.md airgap-transfer/
cp docs/AIR_GAPPED_DEPLOYMENT.md airgap-transfer/

# Optional: create compressed archive
tar -czf airgap-transfer.tar.gz airgap-transfer/
```

#### 5. Transfer Files
Transfer to air-gapped environment via:
- USB drive
- Secure file transfer
- Physical media
- Internal network

**Files to transfer:**
- `node-cleanup-webhook-v1.0.0.tar` (50-80 MB) - **Required**
- `airgap-transfer/` directory - **Required**

---

### Phase 2: Air-Gapped Environment (Load & Deploy)

#### 1. Load Container Image
```bash
# Load image from tar
podman load -i node-cleanup-webhook-v1.0.0.tar

# Verify image loaded
podman images | grep node-cleanup-webhook
# Should show: node-cleanup-webhook  v1.0.0  ...
```

#### 2. Tag for Internal Registry
```bash
# Tag for your internal registry
podman tag node-cleanup-webhook:v1.0.0 \
  internal-registry.company.local/infra/node-cleanup-webhook:v1.0.0

# Verify tag
podman images | grep node-cleanup-webhook
```

#### 3. Login and Push to Internal Registry
```bash
# Login to internal registry
podman login internal-registry.company.local

# Push image
podman push internal-registry.company.local/infra/node-cleanup-webhook:v1.0.0

# Verify image is in registry
podman search internal-registry.company.local/infra/node-cleanup-webhook
```

#### 4. Generate Certificates (Manual - No cert-manager)

**Why manual certificates?**
- ✅ No cert-manager images needed
- ✅ No CRDs to install
- ✅ Simpler and faster
- ✅ Works anywhere

```bash
# Create certs directory
mkdir -p certs && cd certs

# Generate CA and webhook certificates
openssl req -x509 -newkey rsa:4096 -nodes -keyout ca.key -out ca.crt -days 3650 \
  -subj "/CN=Webhook CA"

openssl req -x509 -newkey rsa:4096 -nodes -keyout tls.key -out tls.crt -days 365 \
  -subj "/CN=node-cleanup-webhook.node-cleanup-system.svc" \
  -addext "subjectAltName=DNS:node-cleanup-webhook,DNS:node-cleanup-webhook.node-cleanup-system,DNS:node-cleanup-webhook.node-cleanup-system.svc,DNS:node-cleanup-webhook.node-cleanup-system.svc.cluster.local"

# Get CA bundle for webhook config
CA_BUNDLE=$(cat ca.crt | base64 -w 0)
echo "CA_BUNDLE=$CA_BUNDLE"
```

#### 5. Create Kubernetes Secret
```bash
# Create namespace
kubectl create namespace node-cleanup-system

# Create TLS secret
kubectl create secret tls webhook-server-cert \
  --cert=tls.crt \
  --key=tls.key \
  -n node-cleanup-system

# Verify secret
kubectl get secret webhook-server-cert -n node-cleanup-system
```

#### 6. Deploy with Helm
```bash
cd airgap-transfer/

# Create custom values file
cat > custom-values.yaml <<EOF
image:
  repository: internal-registry.company.local/infra/node-cleanup-webhook
  tag: v1.0.0
  pullPolicy: IfNotPresent

webhook:
  certManager:
    enabled: false  # Using manual certificates
  caBundle: "$CA_BUNDLE"

# Configure plugins (order matters!)
env:
  ENABLED_PLUGINS: "logger,portworx"
  PORTWORX_API_ENDPOINT: "http://portworx-api:9001"
  PORTWORX_LABEL_SELECTOR: "px/enabled=true"
EOF

# Install webhook
helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace \
  -f custom-values.yaml

# Wait for deployment
kubectl wait --for=condition=Available --timeout=300s \
  deployment/node-cleanup-webhook -n node-cleanup-system
```

#### 7. Verify Deployment
```bash
# Check pods
kubectl get pods -n node-cleanup-system
# Should show: 2/2 Running

# Check logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook --tail=50

# Verify webhook config
kubectl get mutatingwebhookconfiguration node-cleanup-webhook
```

#### 8. Test Webhook
```bash
# Create test node
kubectl create -f - <<EOF
apiVersion: v1
kind: Node
metadata:
  name: test-airgap-node
  labels:
    px/enabled: "true"
spec:
  podCIDR: 10.244.9.0/24
EOF

# Check finalizer was added
kubectl get node test-airgap-node -o jsonpath='{.metadata.finalizers}'
# Should show: ["infra.894.io/node-cleanup"]

# Delete and watch cleanup
kubectl delete node test-airgap-node

# Check logs to see plugins executed in order
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook --tail=100 | grep -E "(logger|portworx)"
```

---

## What Gets Transferred?

### Minimum Files Required:
```
airgap-transfer/
├── node-cleanup-webhook-v1.0.0.tar  # Container image (50-80 MB)
└── deploy/
    └── helm/node-cleanup-webhook/    # Helm chart
```

**Total size**: ~80-100 MB

### Optional Files (Helpful):
```
├── README.md                         # User guide
├── docs/
│   ├── AIR_GAPPED_DEPLOYMENT.md         # This guide
│   └── AIR_GAPPED_DEPLOYMENT.md   # Certificate options
```

---

## Why This Approach Works

### ✅ Advantages:

1. **Simple**: Just transfer one tar file
2. **Fast**: No compilation in air-gapped environment
3. **Reliable**: Image built in controlled environment
4. **Small**: ~80 MB total transfer
5. **Reproducible**: Same image everywhere

### 🎯 What's Included in the Image:

The container image already contains:
- ✅ Compiled binary (webhook)
- ✅ All dependencies (vendored during build)
- ✅ Minimal distroless base (~2 MB)

**Nothing else needed!**

---

## Alternative: Raw Manifests Instead of Helm

If you don't have Helm in air-gapped environment:

```bash
# Update manifests with your registry
cd airgap-transfer/deploy/manifests/

# Update image reference
sed -i 's|registry.example.com/node-cleanup-webhook:latest|internal-registry.company.local/infra/node-cleanup-webhook:v1.0.0|g' \
  deployment.yaml

# Update webhook caBundle
sed -i "s|caBundle:.*|caBundle: $CA_BUNDLE|" webhook-config.yaml

# Create namespace
kubectl create namespace node-cleanup-system

# Create secret
kubectl create secret tls webhook-server-cert \
  --cert=../../../certs/tls.crt \
  --key=../../../certs/tls.key \
  -n node-cleanup-system

# Deploy
kubectl apply -f .

# Verify
kubectl get pods -n node-cleanup-system
```

---

## Configuration

### Plugin Configuration (via Helm values):

```yaml
env:
  # Plugin execution order (ORDER MATTERS!)
  ENABLED_PLUGINS: "logger,portworx"

  # Logger plugin
  LOGGER_FORMAT: "pretty"
  LOGGER_VERBOSITY: "info"

  # Portworx plugin
  PORTWORX_LABEL_SELECTOR: "px/enabled=true"
  PORTWORX_API_ENDPOINT: "http://portworx-api:9001"
  PORTWORX_TIMEOUT: "300s"
```

### Plugin Configuration (via manifest ConfigMap):

Edit `deploy/manifests/deployment.yaml`:
```yaml
env:
  - name: ENABLED_PLUGINS
    value: "logger,portworx"
  - name: PORTWORX_API_ENDPOINT
    value: "http://portworx-api:9001"
```

---

## Updating to New Version

### Connected Environment:
```bash
# Build new version
podman build -t node-cleanup-webhook:v1.1.0 .

# Export
podman save node-cleanup-webhook:v1.1.0 -o node-cleanup-webhook-v1.1.0.tar

# Transfer to air-gapped
```

### Air-Gapped Environment:
```bash
# Load new version
podman load -i node-cleanup-webhook-v1.1.0.tar

# Tag and push
podman tag node-cleanup-webhook:v1.1.0 \
  internal-registry.company.local/infra/node-cleanup-webhook:v1.1.0
podman push internal-registry.company.local/infra/node-cleanup-webhook:v1.1.0

# Upgrade with Helm
helm upgrade node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --set image.tag=v1.1.0 \
  -f custom-values.yaml

# Or with kubectl
kubectl set image deployment/node-cleanup-webhook \
  webhook=internal-registry.company.local/infra/node-cleanup-webhook:v1.1.0 \
  -n node-cleanup-system
```

---

## Troubleshooting

### Issue: "ImagePullBackOff"

**Cause**: Image not in internal registry or wrong image name

**Solution**:
```bash
# Verify image exists
podman images | grep node-cleanup-webhook

# Check if pushed to registry
curl -k https://internal-registry.company.local/v2/infra/node-cleanup-webhook/tags/list

# Check pod events
kubectl describe pod -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook
```

### Issue: "x509: certificate signed by unknown authority"

**Cause**: CA bundle not configured

**Solution**:
```bash
# Get CA bundle
CA_BUNDLE=$(cat certs/ca.crt | base64 -w 0)

# Update webhook config
kubectl patch mutatingwebhookconfiguration node-cleanup-webhook \
  --type='json' \
  -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'$CA_BUNDLE'}]"
```

### Issue: Webhook not responding

**Solution**:
```bash
# Check logs
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook

# Check if webhook is running
kubectl get pods -n node-cleanup-system

# Test webhook endpoint
kubectl exec -n node-cleanup-system deployment/node-cleanup-webhook -- \
  wget -O- --no-check-certificate https://localhost:8443/healthz
```

---

## Quick Reference Card

```bash
# === CONNECTED ENVIRONMENT ===
podman build -t webhook:v1 .
podman save webhook:v1 -o webhook.tar
# Transfer webhook.tar

# === AIR-GAPPED ENVIRONMENT ===
podman load -i webhook.tar
podman tag webhook:v1 registry/webhook:v1
podman push registry/webhook:v1

# Generate certs
openssl req -x509 -nodes -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 \
  -subj "/CN=node-cleanup-webhook.node-cleanup-system.svc"

# Deploy
kubectl create namespace node-cleanup-system
kubectl create secret tls webhook-server-cert --cert=tls.crt --key=tls.key -n node-cleanup-system
CA_BUNDLE=$(cat tls.crt | base64 -w 0)
helm install webhook ./deploy/helm/... \
  --set image.repository=registry/webhook \
  --set webhook.certManager.enabled=false \
  --set webhook.caBundle=$CA_BUNDLE
```

---

**Summary**: Build image once in connected environment, transfer tar file, load and deploy in air-gapped environment. Simple and reliable!
