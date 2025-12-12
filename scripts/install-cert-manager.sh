#!/bin/bash
set -e

# Install cert-manager for TLS certificate management

CERT_MANAGER_VERSION=${CERT_MANAGER_VERSION:-"v1.13.0"}

echo "Installing cert-manager ${CERT_MANAGER_VERSION}..."

kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml

echo "Waiting for cert-manager to be ready..."
kubectl wait --for=condition=Available --timeout=300s \
  -n cert-manager \
  deployment/cert-manager \
  deployment/cert-manager-cainjector \
  deployment/cert-manager-webhook

echo "Creating self-signed ClusterIssuer..."
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
EOF

echo "✅ cert-manager installed successfully!"
