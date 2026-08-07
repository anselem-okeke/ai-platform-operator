#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
MODEL_HOSTNAME="${MODEL_HOSTNAME:-fraud-model.local}"

CA_FILE="${CA_FILE:-.local/keycloak/fraud-model-root-ca.crt}"
TOKEN_FILE="${TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"

for command_name in kubectl curl; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command missing: ${command_name}" >&2
    exit 1
  }
done

[[ -s "${CA_FILE}" ]] || {
  echo "ERROR: CA file missing: ${CA_FILE}" >&2
  exit 1
}

[[ -s "${TOKEN_FILE}" ]] || {
  echo "ERROR: Token file missing: ${TOKEN_FILE}" >&2
  exit 1
}

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

[[ -n "${gateway_ip}" ]] || {
  echo "ERROR: Gateway address is empty." >&2
  exit 1
}

access_token="$(cat "${TOKEN_FILE}")"

request_status() {
  local method="$1"
  local token="${2:-}"

  local arguments=(
    --silent
    --show-error
    --output /dev/null
    --write-out '%{http_code}'
    --request "${method}"
    --cacert "${CA_FILE}"
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}"
  )

  if [[ -n "${token}" ]]; then
    arguments+=(
      --header "Authorization: Bearer ${token}"
    )
  fi

  curl \
    "${arguments[@]}" \
    "https://${MODEL_HOSTNAME}/"
}

echo "Checking authentication failures..."

no_token_status="$(request_status GET)"
invalid_token_status="$(request_status GET 'invalid.jwt.value')"

[[ "${no_token_status}" == "401" ]] || {
  echo "ERROR: Missing token returned ${no_token_status}; expected 401." >&2
  exit 1
}

[[ "${invalid_token_status}" == "401" ]] || {
  echo "ERROR: Invalid token returned ${invalid_token_status}; expected 401." >&2
  exit 1
}

echo "Checking model-deployer permissions..."

deployer_get_status="$(request_status GET "${access_token}")"
deployer_post_status="$(request_status POST "${access_token}")"
deployer_delete_status="$(request_status DELETE "${access_token}")"

[[ "${deployer_get_status}" == "200" ]] || {
  echo "ERROR: Deployer GET returned ${deployer_get_status}; expected 200." >&2
  exit 1
}

case "${deployer_post_status}" in
  401|403)
    echo "ERROR: Deployer POST was rejected with ${deployer_post_status}." >&2
    exit 1
    ;;
esac

[[ "${deployer_delete_status}" == "403" ]] || {
  echo "ERROR: Deployer DELETE returned ${deployer_delete_status}; expected 403." >&2
  exit 1
}

echo
echo "PASS: Missing JWT returns 401."
echo "PASS: Invalid JWT returns 401."
echo "PASS: model-deployer can use GET."
echo "PASS: model-deployer can pass POST authorization."
echo "PASS: model-deployer cannot use DELETE."
echo "INFO: GET status:    ${deployer_get_status}"
echo "INFO: POST status:   ${deployer_post_status}"
echo "INFO: DELETE status: ${deployer_delete_status}"
