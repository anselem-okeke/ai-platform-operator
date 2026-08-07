#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
ROUTE_NAMESPACE="${ROUTE_NAMESPACE:-ai-platform}"
POLICY_NAME="${POLICY_NAME:-fraud-model-jwt-authentication}"

MODEL_HOSTNAME="${MODEL_HOSTNAME:-fraud-model.local}"
KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-auth.ai-platform.local}"

CA_FILE="${CA_FILE:-.local/keycloak/auth-ai-platform-root-ca.crt}"
TOKEN_FILE="${TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"

for command_name in kubectl curl jq; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command is missing: ${command_name}" >&2
    exit 1
  }
done

if [[ ! -s "${CA_FILE}" ]]; then
  echo "ERROR: CA file is missing: ${CA_FILE}" >&2
  exit 1
fi

if [[ ! -s "${TOKEN_FILE}" ]]; then
  echo "ERROR: Token file is missing: ${TOKEN_FILE}" >&2
  exit 1
fi

kubectl get securitypolicy "${POLICY_NAME}" \
  --namespace "${ROUTE_NAMESPACE}" \
  >/dev/null

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

if [[ -z "${gateway_ip}" ]]; then
  echo "ERROR: Gateway has no address." >&2
  exit 1
fi

access_token="$(
  cat "${TOKEN_FILE}"
)"

echo "Checking request without a JWT..."

no_token_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    "https://${MODEL_HOSTNAME}/"
)"

if [[ "${no_token_status}" != "401" ]]; then
  echo "ERROR: Missing token returned ${no_token_status}; expected 401." >&2
  exit 1
fi

echo "Checking malformed JWT..."

invalid_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    --header 'Authorization: Bearer invalid.jwt.value' \
    "https://${MODEL_HOSTNAME}/"
)"

if [[ "${invalid_status}" != "401" ]]; then
  echo "ERROR: Invalid token returned ${invalid_status}; expected 401." >&2
  exit 1
fi

echo "Checking valid JWT..."

valid_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    --header "Authorization: Bearer ${access_token}" \
    "https://${MODEL_HOSTNAME}/"
)"

if [[ "${valid_status}" == "401" ]]; then
  echo "ERROR: Valid token was rejected with 401." >&2
  exit 1
fi

echo
echo "PASS: Missing JWT returns 401."
echo "PASS: Malformed JWT returns 401."
echo "PASS: Valid JWT passes gateway authentication."
echo "Backend response status: ${valid_status}"
