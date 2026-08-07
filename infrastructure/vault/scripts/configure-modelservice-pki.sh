#!/usr/bin/env bash
set -euo pipefail
: "${VAULT_ADDR:?}"
: "${VAULT_CACERT:?}"
VAULT_PKI_PATH="${VAULT_PKI_PATH:-pki_modelservice}"
VAULT_PKI_ROLE="${VAULT_PKI_ROLE:-modelservice}"
VAULT_POLICY="${VAULT_POLICY:-cert-manager-modelservice-pki}"
POLICY_FILE="${POLICY_FILE:-infrastructure/vault/policies/cert-manager-modelservice-pki.hcl}"
vault secrets list -format=json | jq -e --arg p "${VAULT_PKI_PATH}/" 'has($p)' >/dev/null ||
  vault secrets enable -path="${VAULT_PKI_PATH}" pki
vault secrets tune -max-lease-ttl=87600h "${VAULT_PKI_PATH}"
vault read "${VAULT_PKI_PATH}/cert/ca" >/dev/null 2>&1 ||
  vault write -field=certificate "${VAULT_PKI_PATH}/root/generate/internal"     common_name="AI Platform ModelService Root CA" ttl=87600h key_type=ec key_bits=256     > /tmp/modelservice-root-ca.crt
vault write "${VAULT_PKI_PATH}/roles/${VAULT_PKI_ROLE}"   allowed_domains="fraud-model.local" allow_bare_domains=true   allow_subdomains=false enforce_hostnames=true key_type=ec key_bits=256 max_ttl=720h
vault policy write "${VAULT_POLICY}" "${POLICY_FILE}"
