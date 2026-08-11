#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"
MODEL_SERVICE_NAME="${MODEL_SERVICE_NAME:-fraud-model}"

TOKEN_SCRIPT="${TOKEN_SCRIPT:-infrastructure/keycloak/scripts/get-machine-token.sh}"
TOKEN_FILE="${TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"

PASS_COUNT=0
FAIL_COUNT=0

pass() {
  echo "PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
  echo "FAIL: $1"
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

expect_status() {
  local description="$1"
  local expected="$2"
  shift 2

  local body_file
  body_file="$(mktemp)"

  local status
  status="$(
    curl \
      --silent \
      --show-error \
      --output "${body_file}" \
      --write-out '%{http_code}' \
      "$@"
  )"

  if [[ "${status}" == "${expected}" ]]; then
    pass "${description} (${status})"
  else
    fail "${description}"
    echo "  Expected HTTP: ${expected}"
    echo "  Actual HTTP:   ${status}"
    echo "  Response:"
    sed 's/^/    /' "${body_file}"
  fi

  rm -f "${body_file}"
}

echo "AI Platform API JWT Authentication Validation"
echo "============================================="
echo
echo "API: ${API_BASE_URL}"
echo

echo "1. Checking public health endpoint"
expect_status \
  "health endpoint is public" \
  "200" \
  "${API_BASE_URL}/healthz"

echo
echo "2. Checking public readiness endpoint"
expect_status \
  "readiness endpoint is public" \
  "200" \
  "${API_BASE_URL}/readyz"

echo
echo "3. Checking protected endpoint without JWT"
expect_status \
  "protected endpoint rejects missing JWT" \
  "401" \
  "${API_BASE_URL}/api/v1/model-services"

echo
echo "4. Checking protected endpoint with malformed JWT"
expect_status \
  "protected endpoint rejects malformed JWT" \
  "401" \
  --header 'Authorization: Bearer definitely-not-a-jwt' \
  "${API_BASE_URL}/api/v1/model-services"

echo
echo "5. Obtaining fresh machine token"

if [[ ! -x "${TOKEN_SCRIPT}" ]]; then
  echo "ERROR: Token script is not executable:"
  echo "  ${TOKEN_SCRIPT}"
  exit 1
fi

"${TOKEN_SCRIPT}"

if [[ ! -s "${TOKEN_FILE}" ]]; then
  echo "ERROR: Token file was not created or is empty:"
  echo "  ${TOKEN_FILE}"
  exit 1
fi

TOKEN="$(<"${TOKEN_FILE}")"

if [[ -z "${TOKEN}" ]]; then
  echo "ERROR: Machine token is empty"
  exit 1
fi

pass "fresh machine token obtained"

echo
echo "6. Checking authenticated list endpoint"
expect_status \
  "valid JWT can list ModelServices" \
  "200" \
  --header "Authorization: Bearer ${TOKEN}" \
  "${API_BASE_URL}/api/v1/model-services"

echo
echo "7. Checking authenticated get endpoint"
expect_status \
  "valid JWT can get ModelService" \
  "200" \
  --header "Authorization: Bearer ${TOKEN}" \
  "${API_BASE_URL}/api/v1/model-services/${MODEL_SERVICE_NAME}"

echo
echo "8. Checking authenticated status endpoint"
expect_status \
  "valid JWT can read ModelService status" \
  "200" \
  --header "Authorization: Bearer ${TOKEN}" \
  "${API_BASE_URL}/api/v1/model-services/${MODEL_SERVICE_NAME}/status"

echo
echo "9. Checking nonexistent resource with valid JWT"
expect_status \
  "authenticated missing ModelService returns 404" \
  "404" \
  --header "Authorization: Bearer ${TOKEN}" \
  "${API_BASE_URL}/api/v1/model-services/does-not-exist"

echo
echo "10. Checking nonexistent status resource with valid JWT"
expect_status \
  "authenticated missing ModelService status returns 404" \
  "404" \
  --header "Authorization: Bearer ${TOKEN}" \
  "${API_BASE_URL}/api/v1/model-services/does-not-exist/status"

unset TOKEN

echo
echo "============================================="
echo "Validation summary"
echo "============================================="
echo "PASS: ${PASS_COUNT}"
echo "FAIL: ${FAIL_COUNT}"

if [[ "${FAIL_COUNT}" -ne 0 ]]; then
  echo
  echo "FAIL: AI Platform API JWT authentication validation failed."
  exit 1
fi

echo
echo "PASS: AI Platform API JWT authentication validated."
