#!/usr/bin/env bash
set -euo pipefail
OUTPUT_DIR="${1:-.local/vault-auth}"
mkdir -p "${OUTPUT_DIR}"
kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' |
  base64 --decode > "${OUTPUT_DIR}/kind-ai-platform-policy-ca.crt"
chmod 0644 "${OUTPUT_DIR}/kind-ai-platform-policy-ca.crt"
kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}{"\n"}'
