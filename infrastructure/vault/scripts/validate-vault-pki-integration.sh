#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-gateway-system}"

echo "=== Vault Issuer ==="
kubectl get issuer vault-issuer \
  --namespace "${NAMESPACE}"

kubectl wait \
  --for=condition=Ready \
  issuer/vault-issuer \
  --namespace "${NAMESPACE}" \
  --timeout=180s

echo
echo "=== Certificate ==="
kubectl get certificate fraud-model-local \
  --namespace "${NAMESPACE}"

kubectl wait \
  --for=condition=Ready \
  certificate/fraud-model-local \
  --namespace "${NAMESPACE}" \
  --timeout=180s

echo
echo "=== Latest CertificateRequest ==="
kubectl get certificaterequests \
  --namespace "${NAMESPACE}" \
  --sort-by=.metadata.creationTimestamp

echo
echo "=== Live certificate ==="
kubectl get secret fraud-model-local-tls \
  --namespace "${NAMESPACE}" \
  --output jsonpath='{.data.tls\.crt}' |
  base64 --decode |
  openssl x509 \
    -noout \
    -subject \
    -issuer \
    -dates \
    -serial

echo
echo "=== Gateway ==="
kubectl get gateway shared-gateway \
  --namespace "${NAMESPACE}" \
  --output wide
