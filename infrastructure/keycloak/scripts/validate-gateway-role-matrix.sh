#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
MODEL_HOSTNAME="${MODEL_HOSTNAME:-fraud-model.local}"

CA_FILE="${CA_FILE:-.local/keycloak/fraud-model-root-ca.crt}"

VIEWER_TOKEN_FILE="${VIEWER_TOKEN_FILE:-.local/keycloak/tokens/viewer-access-token.jwt}"
DEPLOYER_TOKEN_FILE="${DEPLOYER_TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"
ADMIN_TOKEN_FILE="${ADMIN_TOKEN_FILE:-.local/keycloak/tokens/admin-access-token.jwt}"

for command_name in kubectl curl; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command is missing: ${command_name}" >&2
    exit 1
  }
done

for required_file in \
  "${CA_FILE}" \
  "${VIEWER_TOKEN_FILE}" \
  "${DEPLOYER_TOKEN_FILE}" \
  "${ADMIN_TOKEN_FILE}"
do
  if [[ ! -s "${required_file}" ]]; then
    echo "ERROR: Required file is missing or empty: ${required_file}" >&2
    exit 1
  fi
done

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

if [[ -z "${gateway_ip}" ]]; then
  echo "ERROR: Gateway address is empty." >&2
  exit 1
fi

viewer_token="$(cat "${VIEWER_TOKEN_FILE}")"
deployer_token="$(cat "${DEPLOYER_TOKEN_FILE}")"
admin_token="$(cat "${ADMIN_TOKEN_FILE}")"

request_status() {
  local method="$1"
  local token="${2:-}"
  local body_file

  body_file="$(mktemp)"

  local arguments=(
    --silent
    --show-error
    --output "${body_file}"
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

  if [[ "${method}" == "POST" || "${method}" == "PUT" || "${method}" == "PATCH" ]]; then
    arguments+=(
      --header 'Content-Type: application/json'
      --data '{}'
    )
  fi

  curl \
    "${arguments[@]}" \
    "https://${MODEL_HOSTNAME}/"

  rm -f "${body_file}"
}

require_status() {
  local description="$1"
  local actual="$2"
  local expected="$3"

  if [[ "${actual}" != "${expected}" ]]; then
    echo "ERROR: ${description} returned ${actual}; expected ${expected}." >&2
    exit 1
  fi

  echo "PASS: ${description} returned ${actual}."
}

require_gateway_pass() {
  local description="$1"
  local actual="$2"

  case "${actual}" in
    401|403)
      echo "ERROR: ${description} was rejected by the Gateway with ${actual}." >&2
      exit 1
      ;;
    *)
      echo "PASS: ${description} passed Gateway authorization with backend status ${actual}."
      ;;
  esac
}

echo "Checking authentication failures..."

no_token_get_status="$(
  request_status GET
)"

invalid_token_get_status="$(
  request_status GET 'invalid.jwt.value'
)"

require_status \
  "Missing-token GET" \
  "${no_token_get_status}" \
  "401"

require_status \
  "Invalid-token GET" \
  "${invalid_token_get_status}" \
  "401"

echo
echo "Checking model-viewer permissions..."

viewer_get_status="$(
  request_status GET "${viewer_token}"
)"

viewer_post_status="$(
  request_status POST "${viewer_token}"
)"

viewer_delete_status="$(
  request_status DELETE "${viewer_token}"
)"

require_status \
  "model-viewer GET" \
  "${viewer_get_status}" \
  "200"

require_status \
  "model-viewer POST" \
  "${viewer_post_status}" \
  "403"

require_status \
  "model-viewer DELETE" \
  "${viewer_delete_status}" \
  "403"

echo
echo "Checking model-deployer permissions..."

deployer_get_status="$(
  request_status GET "${deployer_token}"
)"

deployer_post_status="$(
  request_status POST "${deployer_token}"
)"

deployer_delete_status="$(
  request_status DELETE "${deployer_token}"
)"

require_status \
  "model-deployer GET" \
  "${deployer_get_status}" \
  "200"

require_gateway_pass \
  "model-deployer POST" \
  "${deployer_post_status}"

require_status \
  "model-deployer DELETE" \
  "${deployer_delete_status}" \
  "403"

echo
echo "Checking platform-admin permissions..."

admin_get_status="$(
  request_status GET "${admin_token}"
)"

admin_post_status="$(
  request_status POST "${admin_token}"
)"

admin_delete_status="$(
  request_status DELETE "${admin_token}"
)"

require_status \
  "platform-admin GET" \
  "${admin_get_status}" \
  "200"

require_gateway_pass \
  "platform-admin POST" \
  "${admin_post_status}"

require_gateway_pass \
  "platform-admin DELETE" \
  "${admin_delete_status}"

echo
echo "Authorization matrix:"
printf '%-24s %-8s %-8s\n' "Identity" "Method" "Status"
printf '%-24s %-8s %-8s\n' "missing-token" "GET" "${no_token_get_status}"
printf '%-24s %-8s %-8s\n' "invalid-token" "GET" "${invalid_token_get_status}"
printf '%-24s %-8s %-8s\n' "model-viewer" "GET" "${viewer_get_status}"
printf '%-24s %-8s %-8s\n' "model-viewer" "POST" "${viewer_post_status}"
printf '%-24s %-8s %-8s\n' "model-viewer" "DELETE" "${viewer_delete_status}"
printf '%-24s %-8s %-8s\n' "model-deployer" "GET" "${deployer_get_status}"
printf '%-24s %-8s %-8s\n' "model-deployer" "POST" "${deployer_post_status}"
printf '%-24s %-8s %-8s\n' "model-deployer" "DELETE" "${deployer_delete_status}"
printf '%-24s %-8s %-8s\n' "platform-admin" "GET" "${admin_get_status}"
printf '%-24s %-8s %-8s\n' "platform-admin" "POST" "${admin_post_status}"
printf '%-24s %-8s %-8s\n' "platform-admin" "DELETE" "${admin_delete_status}"

echo
echo "PASS: Gateway authentication and role-authorization matrix validated."
