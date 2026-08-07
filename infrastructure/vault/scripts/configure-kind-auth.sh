#!/usr/bin/env bash
set -euo pipefail

: "${VAULT_ADDR:?Set VAULT_ADDR}"
: "${VAULT_CACERT:?Set VAULT_CACERT}"
: "${KUBERNETES_HOST:?Set KUBERNETES_HOST}"
: "${KUBERNETES_CA_FILE:?Set KUBERNETES_CA_FILE}"
: "${TOKEN_REVIEWER_JWT_FILE:?Set TOKEN_REVIEWER_JWT_FILE}"

VAULT_K8S_AUTH_PATH="${VAULT_K8S_AUTH_PATH:-kubernetes-kind}"
VAULT_K8S_ROLE="${VAULT_K8S_ROLE:-cert-manager-modelservice}"
VAULT_POLICY="${VAULT_POLICY:-cert-manager-modelservice-pki}"
TOKEN_AUDIENCE="${TOKEN_AUDIENCE:-https://kubernetes.default.svc.cluster.local}"

for file in \
  "${KUBERNETES_CA_FILE}" \
  "${TOKEN_REVIEWER_JWT_FILE}"
do
  if [[ ! -s "${file}" ]]; then
    echo "ERROR: Required file is missing or empty: ${file}" >&2
    exit 1
  fi
done

if ! vault auth list -format=json |
  jq -e --arg path "${VAULT_K8S_AUTH_PATH}/" 'has($path)' >/dev/null
then
  vault auth enable \
    -path="${VAULT_K8S_AUTH_PATH}" \
    kubernetes
fi

vault write "auth/${VAULT_K8S_AUTH_PATH}/config" \
  kubernetes_host="${KUBERNETES_HOST}" \
  kubernetes_ca_cert=@"${KUBERNETES_CA_FILE}" \
  token_reviewer_jwt=@"${TOKEN_REVIEWER_JWT_FILE}" \
  disable_iss_validation=true

vault write "auth/${VAULT_K8S_AUTH_PATH}/role/${VAULT_K8S_ROLE}" \
  bound_service_account_names="cert-manager-vault-issuer" \
  bound_service_account_namespaces="gateway-system" \
  audience="${TOKEN_AUDIENCE}" \
  policies="${VAULT_POLICY}" \
  token_ttl=10m \
  token_max_ttl=30m

echo
echo "Vault Kubernetes authentication configured."
echo "Auth mount: ${VAULT_K8S_AUTH_PATH}/"
echo "Role:       ${VAULT_K8S_ROLE}"
echo "Audience:   ${TOKEN_AUDIENCE}"
