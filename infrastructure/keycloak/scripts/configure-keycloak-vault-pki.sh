#!/usr/bin/env bash
set -euo pipefail

: "${VAULT_ADDR:?VAULT_ADDR is required}"
: "${VAULT_CACERT:?VAULT_CACERT is required}"

VAULT_PKI_PATH="${VAULT_PKI_PATH:-pki_modelservice}"
VAULT_PKI_ROLE="${VAULT_PKI_ROLE:-keycloak}"
VAULT_POLICY="${VAULT_POLICY:-cert-manager-keycloak-pki}"
VAULT_K8S_AUTH_PATH="${VAULT_K8S_AUTH_PATH:-kubernetes-kind}"
VAULT_K8S_ROLE="${VAULT_K8S_ROLE:-cert-manager-keycloak}"
TOKEN_AUDIENCE="${TOKEN_AUDIENCE:-https://kubernetes.default.svc.cluster.local}"

SCRIPT_DIR="$(
  cd "$(dirname "${BASH_SOURCE[0]}")" &&
  pwd
)"

POLICY_FILE="$(
  cd "${SCRIPT_DIR}/../policies" &&
  pwd
)/cert-manager-keycloak-pki.hcl"

if [[ ! -s "${POLICY_FILE}" ]]; then
  echo "ERROR: Vault policy file is missing: ${POLICY_FILE}" >&2
  exit 1
fi

echo "Configuring Vault PKI role..."

vault write \
  "${VAULT_PKI_PATH}/roles/${VAULT_PKI_ROLE}" \
  allowed_domains="auth.ai-platform.local" \
  allow_bare_domains=true \
  allow_subdomains=false \
  allow_glob_domains=false \
  allow_wildcard_certificates=false \
  allow_localhost=false \
  allow_ip_sans=false \
  enforce_hostnames=true \
  require_cn=true \
  server_flag=true \
  client_flag=false \
  key_type=ec \
  key_bits=256 \
  key_usage="DigitalSignature,KeyAgreement,KeyEncipherment" \
  max_ttl=720h

echo "Writing Vault policy..."

vault policy write \
  "${VAULT_POLICY}" \
  "${POLICY_FILE}"

echo "Configuring Vault Kubernetes authentication role..."

vault write \
  "auth/${VAULT_K8S_AUTH_PATH}/role/${VAULT_K8S_ROLE}" \
  bound_service_account_names="cert-manager-keycloak-issuer" \
  bound_service_account_namespaces="gateway-system" \
  audience="${TOKEN_AUDIENCE}" \
  policies="${VAULT_POLICY}" \
  token_ttl=10m \
  token_max_ttl=30m

echo
echo "Vault Keycloak certificate configuration complete."
echo "PKI role:  ${VAULT_PKI_PATH}/roles/${VAULT_PKI_ROLE}"
echo "Policy:    ${VAULT_POLICY}"
echo "Auth role: ${VAULT_K8S_AUTH_PATH}/${VAULT_K8S_ROLE}"
echo "Audience:  ${TOKEN_AUDIENCE}"
