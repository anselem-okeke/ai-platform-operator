#!/usr/bin/env bash
set -euo pipefail

KEYCLOAK_URL="${KEYCLOAK_URL:-https://auth.ai-platform.local}"
REALM="${REALM:-ai-platform}"
CLIENT_ID="${CLIENT_ID:-ai-platform-service}"

CLIENT_SECRET_FILE="${CLIENT_SECRET_FILE:-.local/keycloak/ai-platform-service-client-secret}"
TOKEN_DIR="${TOKEN_DIR:-.local/keycloak/tokens}"
TOKEN_FILE="${TOKEN_FILE:-${TOKEN_DIR}/service-access-token.jwt}"
RESPONSE_FILE="${RESPONSE_FILE:-${TOKEN_DIR}/service-token-response.json}"
CA_CERT_FILE="${CA_CERT_FILE:-.local/keycloak/auth-ai-platform-root-ca.crt}"

for command_name in curl jq; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: ${command_name} is required." >&2
    exit 1
  }
done

if [[ ! -s "${CLIENT_SECRET_FILE}" ]]; then
  echo "ERROR: Client secret file does not exist or is empty:" >&2
  echo "       ${CLIENT_SECRET_FILE}" >&2
  exit 1
fi

if [[ ! -s "${CA_CERT_FILE}" ]]; then
  echo "ERROR: Keycloak CA certificate does not exist or is empty:" >&2
  echo "       ${CA_CERT_FILE}" >&2
  exit 1
fi

client_secret="$(tr -d '\r\n' < "${CLIENT_SECRET_FILE}")"

mkdir -p "${TOKEN_DIR}"
chmod 700 "${TOKEN_DIR}"

response="$(
  curl \
    --silent \
    --show-error \
    --fail-with-body \
    --cacert "${CA_CERT_FILE}" \
    --request POST \
    "${KEYCLOAK_URL}/realms/${REALM}/protocol/openid-connect/token" \
    --header "Content-Type: application/x-www-form-urlencoded" \
    --data-urlencode "grant_type=client_credentials" \
    --data-urlencode "client_id=${CLIENT_ID}" \
    --data-urlencode "client_secret=${client_secret}"
)"

access_token="$(jq -r '.access_token // empty' <<<"${response}")"
expires_in="$(jq -r '.expires_in // empty' <<<"${response}")"
token_type="$(jq -r '.token_type // empty' <<<"${response}")"

if [[ -z "${access_token}" ]]; then
  echo "ERROR: Keycloak response did not contain an access_token." >&2
  jq . <<<"${response}" >&2
  exit 1
fi

printf '%s\n' "${response}" > "${RESPONSE_FILE}"
chmod 600 "${RESPONSE_FILE}"

printf '%s\n' "${access_token}" > "${TOKEN_FILE}"
chmod 600 "${TOKEN_FILE}"

echo "PASS: Machine access token obtained."
echo "INFO: Client: ${CLIENT_ID}"
echo "INFO: Token type: ${token_type:-unknown}"
echo "INFO: Expires in: ${expires_in:-unknown} seconds"
echo "INFO: Token written to: ${TOKEN_FILE}"
