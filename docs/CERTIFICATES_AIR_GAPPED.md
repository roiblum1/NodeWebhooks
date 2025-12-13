# Certificate Management in Air-Gapped Environments

## The Problem

cert-manager requires:
- ✅ cert-manager container images
- ✅ cert-manager CRDs (CustomResourceDefinitions)
- ✅ cert-manager webhook
- ❌ May try to download ACME/Let's Encrypt (won't work in air-gapped)

## Solution: Two Options

### Option 1: Manual Certificates (Recommended for Air-Gapped)

Generate certificates manually and create Kubernetes secrets. **No cert-manager needed!**

#### Step 1: Generate Certificates

```bash
# Create a directory for certificates
mkdir -p certs && cd certs

# Generate CA private key
openssl genrsa -out ca.key 4096

# Generate CA certificate
openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 \
  -out ca.crt \
  -subj "/CN=Node Cleanup Webhook CA"

# Generate webhook private key
openssl genrsa -out tls.key 4096

# Create CSR config
cat > csr.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = node-cleanup-webhook
DNS.2 = node-cleanup-webhook.node-cleanup-system
DNS.3 = node-cleanup-webhook.node-cleanup-system.svc
DNS.4 = node-cleanup-webhook.node-cleanup-system.svc.cluster.local
EOF

# Generate CSR
openssl req -new -key tls.key -out tls.csr \
  -subj "/CN=node-cleanup-webhook.node-cleanup-system.svc" \
  -config csr.conf

# Sign certificate with CA
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out tls.crt -days 365 -sha256 \
  -extensions v3_req -extfile csr.conf

# Verify certificate
openssl x509 -in tls.crt -text -noout | grep -A1 "Subject Alternative Name"
```

#### Step 2: Create Kubernetes Secret

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

#### Step 3: Get CA Bundle (Base64)

```bash
# Get CA bundle for webhook configuration
CA_BUNDLE=$(cat ca.crt | base64 -w 0)
echo $CA_BUNDLE
```

#### Step 4: Deploy with Manual Certificates

**Using Helm:**

```bash
# Create custom values
cat > values-manual-certs.yaml <<EOF
webhook:
  certManager:
    enabled: false  # Disable cert-manager
  caBundle: "$CA_BUNDLE"  # Use manual CA bundle

  # Use manual secret
  secretName: webhook-server-cert
EOF

# Deploy with manual certs
helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace \
  -f values-manual-certs.yaml
```

**Using Raw Manifests:**

```bash
# Update webhook-config.yaml with CA bundle
sed -i "s/caBundle: .*/caBundle: $CA_BUNDLE/" \
  deploy/manifests/webhook-config.yaml

# Update deployment to use manual secret
sed -i 's/secretName: webhook-server-cert/secretName: webhook-server-cert/' \
  deploy/manifests/deployment.yaml

# Deploy
kubectl apply -f deploy/manifests/
```

---

### Option 2: cert-manager in Air-Gapped (Advanced)

If you **must** use cert-manager, you need to transfer all its components.

#### Step 1: Download cert-manager (Connected Environment)

```bash
# Download cert-manager manifest
curl -L https://github.com/cert-manager/cert-manager/releases/download/v1.13.3/cert-manager.yaml \
  -o cert-manager.yaml

# List all images in manifest
grep 'image:' cert-manager.yaml | sort -u

# You'll see:
# quay.io/jetstack/cert-manager-controller:v1.13.3
# quay.io/jetstack/cert-manager-webhook:v1.13.3
# quay.io/jetstack/cert-manager-cainjector:v1.13.3
```

#### Step 2: Pull and Export Images

```bash
# Pull images
podman pull quay.io/jetstack/cert-manager-controller:v1.13.3
podman pull quay.io/jetstack/cert-manager-webhook:v1.13.3
podman pull quay.io/jetstack/cert-manager-cainjector:v1.13.3

# Save images
podman save -o cert-manager-images.tar \
  quay.io/jetstack/cert-manager-controller:v1.13.3 \
  quay.io/jetstack/cert-manager-webhook:v1.13.3 \
  quay.io/jetstack/cert-manager-cainjector:v1.13.3
```

#### Step 3: Transfer to Air-Gapped

Transfer these files:
- `cert-manager.yaml`
- `cert-manager-images.tar`

#### Step 4: Load and Push to Internal Registry

```bash
# Load images
podman load -i cert-manager-images.tar

# Tag for internal registry
podman tag quay.io/jetstack/cert-manager-controller:v1.13.3 \
  internal-registry/cert-manager-controller:v1.13.3
podman tag quay.io/jetstack/cert-manager-webhook:v1.13.3 \
  internal-registry/cert-manager-webhook:v1.13.3
podman tag quay.io/jetstack/cert-manager-cainjector:v1.13.3 \
  internal-registry/cert-manager-cainjector:v1.13.3

# Push to internal registry
podman push internal-registry/cert-manager-controller:v1.13.3
podman push internal-registry/cert-manager-webhook:v1.13.3
podman push internal-registry/cert-manager-cainjector:v1.13.3
```

#### Step 5: Update Manifest and Deploy

```bash
# Update image references in cert-manager.yaml
sed -i 's|quay.io/jetstack|internal-registry|g' cert-manager.yaml

# Deploy cert-manager
kubectl apply -f cert-manager.yaml

# Wait for cert-manager to be ready
kubectl wait --for=condition=Available --timeout=300s \
  deployment/cert-manager -n cert-manager
kubectl wait --for=condition=Available --timeout=300s \
  deployment/cert-manager-webhook -n cert-manager
kubectl wait --for=condition=Available --timeout=300s \
  deployment/cert-manager-cainjector -n cert-manager
```

#### Step 6: Create ClusterIssuer

```bash
# Create self-signed ClusterIssuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
EOF
```

#### Step 7: Deploy Webhook with cert-manager

```bash
# Now deploy webhook (it will use cert-manager)
helm install node-cleanup-webhook ./deploy/helm/node-cleanup-webhook \
  --namespace node-cleanup-system \
  --create-namespace \
  --set webhook.certManager.enabled=true
```

---

## Comparison: Manual vs cert-manager

| Feature | Manual Certificates | cert-manager |
|---------|-------------------|--------------|
| **Complexity** | Simple | Complex |
| **Air-gapped compatible** | ✅ Easy | ⚠️ Requires extra work |
| **Additional images needed** | 0 | 3 |
| **Certificate rotation** | Manual | Automatic |
| **Initial setup time** | 5 minutes | 30+ minutes |
| **Dependencies** | None | cert-manager CRDs + images |
| **Recommended for air-gapped** | ✅ **YES** | ❌ No (unless required) |

---

## Helm Values Reference

### For Manual Certificates:

```yaml
webhook:
  certManager:
    enabled: false
  caBundle: "LS0tLS1CRUdJTi..."  # Base64 encoded CA cert
  secretName: webhook-server-cert  # Secret with tls.crt and tls.key
```

### For cert-manager:

```yaml
webhook:
  certManager:
    enabled: true
    issuerRef:
      name: selfsigned-issuer
      kind: ClusterIssuer
```

---

## Testing Certificate Setup

### Verify Certificate Secret

```bash
# Check secret exists
kubectl get secret -n node-cleanup-system webhook-server-cert

# Verify certificate is valid
kubectl get secret -n node-cleanup-system webhook-server-cert -o jsonpath='{.data.tls\.crt}' | \
  base64 -d | \
  openssl x509 -text -noout
```

### Verify Webhook Configuration

```bash
# Check webhook has CA bundle
kubectl get mutatingwebhookconfiguration node-cleanup-webhook -o yaml | \
  grep caBundle

# Test webhook is responding
kubectl create -f - --dry-run=server <<EOF
apiVersion: v1
kind: Node
metadata:
  name: test-cert-node
EOF
```

### Verify Webhook Logs

```bash
# Check webhook is serving TLS
kubectl logs -n node-cleanup-system -l app.kubernetes.io/name=node-cleanup-webhook | \
  grep "Starting webhook server"
```

---

## Troubleshooting

### Issue: "x509: certificate signed by unknown authority"

**Cause**: CA bundle not configured in webhook

**Solution**:
```bash
# Get CA bundle
CA_BUNDLE=$(kubectl get secret -n node-cleanup-system webhook-server-cert \
  -o jsonpath='{.data.ca\.crt}' 2>/dev/null || \
  cat certs/ca.crt | base64 -w 0)

# Patch webhook configuration
kubectl patch mutatingwebhookconfiguration node-cleanup-webhook \
  --type='json' \
  -p="[{'op': 'replace', 'path': '/webhooks/0/clientConfig/caBundle', 'value':'$CA_BUNDLE'}]"
```

### Issue: "certificate is valid for wrong domains"

**Cause**: Certificate SAN doesn't match service DNS

**Solution**: Regenerate certificate with correct SANs (see Step 1 above)

### Issue: "secret 'webhook-server-cert' not found"

**Cause**: Secret not created

**Solution**:
```bash
kubectl create secret tls webhook-server-cert \
  --cert=certs/tls.crt \
  --key=certs/tls.key \
  -n node-cleanup-system
```

---

## Recommended Approach for Air-Gapped

**✅ Use Manual Certificates** for air-gapped environments:

1. **Simpler** - No additional components
2. **Fewer images** - Zero extra images to transfer
3. **More reliable** - No dependency on cert-manager availability
4. **Faster setup** - 5 minutes vs 30+ minutes
5. **Easier to troubleshoot** - Standard TLS, no CRDs

**Certificate renewal**: Generate new certs yearly and update secret:
```bash
# Renew certificate
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out tls-new.crt -days 365 -sha256 \
  -extensions v3_req -extfile csr.conf

# Update secret
kubectl create secret tls webhook-server-cert \
  --cert=tls-new.crt \
  --key=tls.key \
  --dry-run=client -o yaml | \
  kubectl apply -n node-cleanup-system -f -

# Restart webhook pods to pick up new cert
kubectl rollout restart deployment/node-cleanup-webhook -n node-cleanup-system
```

---

## Quick Reference

### Manual Certificates (5 steps)

```bash
# 1. Generate certificates
openssl genrsa -out ca.key 4096 && \
openssl req -x509 -new -nodes -key ca.key -days 3650 -out ca.crt \
  -subj "/CN=Webhook CA" && \
openssl genrsa -out tls.key 4096 && \
openssl req -new -key tls.key -out tls.csr \
  -subj "/CN=node-cleanup-webhook.node-cleanup-system.svc" && \
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out tls.crt -days 365

# 2. Create secret
kubectl create secret tls webhook-server-cert \
  --cert=tls.crt --key=tls.key -n node-cleanup-system

# 3. Get CA bundle
CA_BUNDLE=$(cat ca.crt | base64 -w 0)

# 4. Deploy with Helm
helm install webhook ./deploy/helm/... \
  --set webhook.certManager.enabled=false \
  --set webhook.caBundle=$CA_BUNDLE

# 5. Done!
```

---

**Recommendation for Air-Gapped**: Use manual certificates. They're simpler, faster, and don't require cert-manager.
