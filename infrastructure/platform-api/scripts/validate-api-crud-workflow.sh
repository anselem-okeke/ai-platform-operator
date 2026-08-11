#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:8080}"

NAMESPACE="${MODEL_SERVICE_NAMESPACE:-ai-platform}"
MODEL_SERVICE_NAME="${MODEL_SERVICE_NAME:-api-e2e-model}"

TOKEN_SCRIPT="${TOKEN_SCRIPT:-infrastructure/keycloak/scripts/get-machine-token.sh}"
DEPLOYER_TOKEN_FILE="${DEPLOYER_TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"
ADMIN_TOKEN_FILE="${ADMIN_TOKEN_FILE:-.local/keycloak/tokens/admin-access-token.jwt}"
VIEWER_TOKEN_FILE="${VIEWER_TOKEN_FILE:-.local/keycloak/tokens/viewer-access-token.jwt}"

E2E_IMAGE="${E2E_IMAGE:-nginxinc/nginx-unprivileged:1.27-alpine}"
E2E_PORT="${E2E_PORT:-8080}"

WAIT_TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-180}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-2}"

PASS_COUNT=0
FAIL_COUNT=0

DEPLOYER_TOKEN=""
ADMIN_TOKEN=""
VIEWER_TOKEN=""

pass() {
  echo "PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
  echo "FAIL: $1"
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

die() {
  echo "ERROR: $1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    die "$1 is required"
  }
}

token_is_expired() {
  local token="$1"

  local payload
  payload="$(
    cut -d '.' -f2 <<<"${token}" |
      tr '_-' '/+' |
      awk '{
        l=length($0)%4;
        if (l==2) printf "%s==", $0;
        else if (l==3) printf "%s=", $0;
        else printf "%s", $0;
      }' |
      base64 -d 2>/dev/null || true
  )"

  local exp
  exp="$(
    jq -r '.exp // 0' <<<"${payload}" 2>/dev/null || echo 0
  )"

  if [[ ! "${exp}" =~ ^[0-9]+$ ]]; then
    return 0
  fi

  local now
  now="$(date +%s)"

  (( exp <= now + 15 ))
}

load_token_file() {
  local description="$1"
  local file="$2"

  if [[ ! -s "${file}" ]]; then
    echo "ERROR: ${description} token file does not exist or is empty:" >&2
    echo "       ${file}" >&2
    return 1
  fi

  local token
  token="$(tr -d '\r\n' < "${file}")"

  if [[ -z "${token}" ]]; then
    echo "ERROR: ${description} token is empty" >&2
    return 1
  fi

  if token_is_expired "${token}"; then
    echo "ERROR: ${description} token is expired or invalid:" >&2
    echo "       ${file}" >&2
    return 1
  fi

  printf '%s' "${token}"
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

request_json() {
  local method="$1"
  local url="$2"
  local token="$3"
  local body="${4:-}"

  if [[ -n "${body}" ]]; then
    curl \
      --silent \
      --show-error \
      --request "${method}" \
      --header "Authorization: Bearer ${token}" \
      --header "Content-Type: application/json" \
      --data "${body}" \
      "${url}"
  else
    curl \
      --silent \
      --show-error \
      --request "${method}" \
      --header "Authorization: Bearer ${token}" \
      "${url}"
  fi
}

wait_for_reconciliation() {
  local description="$1"

  local deadline
  deadline=$((SECONDS + WAIT_TIMEOUT_SECONDS))

  while (( SECONDS < deadline )); do
    local generation
    local observed_generation
    local ready

    generation="$(
      kubectl get \
        modelservice.platform.anselem.dev \
        "${MODEL_SERVICE_NAME}" \
        -n "${NAMESPACE}" \
        -o jsonpath='{.metadata.generation}' \
        2>/dev/null || true
    )"

    observed_generation="$(
      kubectl get \
        modelservice.platform.anselem.dev \
        "${MODEL_SERVICE_NAME}" \
        -n "${NAMESPACE}" \
        -o jsonpath='{.status.observedGeneration}' \
        2>/dev/null || true
    )"

    phase="$(
      kubectl get \
        modelservice.platform.anselem.dev \
        "${MODEL_SERVICE_NAME}" \
        -n "${NAMESPACE}" \
        -o jsonpath='{.status.phase}' \
        2>/dev/null || true
    )"

    available="$(
      kubectl get \
        modelservice.platform.anselem.dev \
        "${MODEL_SERVICE_NAME}" \
        -n "${NAMESPACE}" \
        -o json |
        jq -r '
          [
            .status.conditions[]?
            | select(.type == "Available")
            | .status
          ][0] // ""
        ' 2>/dev/null || true
    )"

    if [[ -n "${generation}" ]] &&
       [[ "${observed_generation}" == "${generation}" ]] &&
       [[ "${phase}" == "Ready" ]] &&
       [[ "${available}" == "True" ]]; then
      pass "${description}"
      return 0
    fi

    sleep "${POLL_INTERVAL_SECONDS}"
  done

  fail "${description}"
  echo "  Timed out after ${WAIT_TIMEOUT_SECONDS}s"
  echo "  Current ModelService:"
  kubectl get \
    modelservice.platform.anselem.dev \
    "${MODEL_SERVICE_NAME}" \
    -n "${NAMESPACE}" \
    -o yaml 2>/dev/null |
    sed 's/^/    /' || true

  return 1
}

wait_for_deletion() {
  local deadline
  deadline=$((SECONDS + WAIT_TIMEOUT_SECONDS))

  while (( SECONDS < deadline )); do
    if ! kubectl get \
      modelservice.platform.anselem.dev \
      "${MODEL_SERVICE_NAME}" \
      -n "${NAMESPACE}" \
      >/dev/null 2>&1; then
      pass "ModelService removed from Kubernetes"
      return 0
    fi

    sleep "${POLL_INTERVAL_SECONDS}"
  done

  fail "ModelService removed from Kubernetes"
  return 1
}

cleanup() {
  kubectl delete \
    modelservice.platform.anselem.dev \
    "${MODEL_SERVICE_NAME}" \
    -n "${NAMESPACE}" \
    --ignore-not-found \
    --wait=false \
    >/dev/null 2>&1 || true
}

trap cleanup EXIT

for command_name in curl jq kubectl base64; do
  require_command "${command_name}"
done

echo "AI Platform API CRUD Workflow Validation"
echo "========================================"
echo
echo "API:       ${API_BASE_URL}"
echo "Namespace: ${NAMESPACE}"
echo "Resource:  ${MODEL_SERVICE_NAME}"
echo "Image:     ${E2E_IMAGE}"
echo

echo "1. Checking API health"

expect_status \
  "health endpoint is reachable" \
  "200" \
  "${API_BASE_URL}/healthz"

expect_status \
  "readiness endpoint is reachable" \
  "200" \
  "${API_BASE_URL}/readyz"

echo
echo "2. Checking Kubernetes connectivity"

if kubectl get namespace "${NAMESPACE}" >/dev/null 2>&1; then
  pass "Kubernetes namespace ${NAMESPACE} is reachable"
else
  die "Kubernetes namespace ${NAMESPACE} is not reachable"
fi

echo
echo "3. Obtaining fresh deployer machine token"

if [[ ! -x "${TOKEN_SCRIPT}" ]]; then
  die "token script is not executable: ${TOKEN_SCRIPT}"
fi

"${TOKEN_SCRIPT}"

DEPLOYER_TOKEN="$(
  load_token_file \
    "deployer" \
    "${DEPLOYER_TOKEN_FILE}"
)"

pass "fresh deployer token loaded"

echo
echo "4. Loading admin token"

if ADMIN_TOKEN="$(
  load_token_file \
    "admin" \
    "${ADMIN_TOKEN_FILE}"
)"; then
  pass "admin token loaded"
else
  echo
  echo "Obtain a fresh platform-admin token before running this workflow."
  echo "Expected file:"
  echo "  ${ADMIN_TOKEN_FILE}"
  exit 1
fi

echo
echo "5. Loading viewer token for negative authorization test"

if VIEWER_TOKEN="$(
  load_token_file \
    "viewer" \
    "${VIEWER_TOKEN_FILE}"
)"; then
  pass "viewer token loaded"
else
  echo
  echo "WARNING: viewer token is unavailable or expired."
  echo "Viewer mutation authorization check will be skipped."
  VIEWER_TOKEN=""
fi

echo
echo "6. Removing stale E2E object"

kubectl delete \
  modelservice.platform.anselem.dev \
  "${MODEL_SERVICE_NAME}" \
  -n "${NAMESPACE}" \
  --ignore-not-found \
  --wait=true \
  >/dev/null

pass "stale E2E ModelService absent"

RESOURCE_URL="${API_BASE_URL}/api/v1/model-services/${MODEL_SERVICE_NAME}"
COLLECTION_URL="${API_BASE_URL}/api/v1/model-services"
STATUS_URL="${RESOURCE_URL}/status"

CREATE_BODY="$(
  jq -nc \
    --arg name "${MODEL_SERVICE_NAME}" \
    --arg image "${E2E_IMAGE}" \
    --argjson port "${E2E_PORT}" \
    '{
      name: $name,
      image: $image,
      replicas: 1,
      port: $port,
      exposure: {
        enabled: false
      },
      storage: {
        enabled: false
      }
    }'
)"

echo
echo "7. Creating ModelService through REST API"

CREATE_RESPONSE="$(
  request_json \
    POST \
    "${COLLECTION_URL}" \
    "${DEPLOYER_TOKEN}" \
    "${CREATE_BODY}"
)"

if jq -e \
  --arg name "${MODEL_SERVICE_NAME}" \
  '
    .name == $name and
    .replicas == 1
  ' \
  <<<"${CREATE_RESPONSE}" \
  >/dev/null; then
  pass "POST created ${MODEL_SERVICE_NAME}"
else
  fail "POST created ${MODEL_SERVICE_NAME}"
  echo "${CREATE_RESPONSE}" | jq . 2>/dev/null || echo "${CREATE_RESPONSE}"
fi

echo
echo "8. Verifying namespace restriction"

ACTUAL_NAMESPACE="$(
  kubectl get \
    modelservice.platform.anselem.dev \
    "${MODEL_SERVICE_NAME}" \
    -n "${NAMESPACE}" \
    -o jsonpath='{.metadata.namespace}'
)"

if [[ "${ACTUAL_NAMESPACE}" == "${NAMESPACE}" ]]; then
  pass "ModelService exists only in configured namespace"
else
  fail "ModelService namespace restriction"
  echo "  Expected: ${NAMESPACE}"
  echo "  Actual:   ${ACTUAL_NAMESPACE}"
fi

echo
echo "9. Waiting for operator reconciliation"

wait_for_reconciliation \
  "created ModelService reconciled and Ready"

echo
echo "10. Reading ModelService through API"

GET_RESPONSE="$(
  request_json \
    GET \
    "${RESOURCE_URL}" \
    "${DEPLOYER_TOKEN}"
)"

if jq -e \
  --arg name "${MODEL_SERVICE_NAME}" \
  --arg image "${E2E_IMAGE}" \
  --argjson port "${E2E_PORT}" \
  '
    .name == $name and
    .image == $image and
    .replicas == 1 and
    .port == $port
  ' \
  <<<"${GET_RESPONSE}" \
  >/dev/null; then
  pass "GET returned expected created state"
else
  fail "GET returned expected created state"
  echo "${GET_RESPONSE}" | jq . 2>/dev/null || echo "${GET_RESPONSE}"
fi

echo
echo "11. Updating ModelService with PUT"

UPDATE_BODY="$(
  jq -nc \
    --arg image "${E2E_IMAGE}" \
    --argjson port "${E2E_PORT}" \
    '{
      image: $image,
      replicas: 2,
      port: $port,
      exposure: {
        enabled: false
      },
      storage: {
        enabled: false
      }
    }'
)"

UPDATE_RESPONSE="$(
  request_json \
    PUT \
    "${RESOURCE_URL}" \
    "${DEPLOYER_TOKEN}" \
    "${UPDATE_BODY}"
)"

if jq -e \
  '
    .replicas == 2
  ' \
  <<<"${UPDATE_RESPONSE}" \
  >/dev/null; then
  pass "PUT changed replicas to 2"
else
  fail "PUT changed replicas to 2"
  echo "${UPDATE_RESPONSE}" | jq . 2>/dev/null || echo "${UPDATE_RESPONSE}"
fi

wait_for_reconciliation \
  "PUT generation reconciled and Ready"

echo
echo "12. Partially updating ModelService with PATCH"

PATCH_RESPONSE="$(
  request_json \
    PATCH \
    "${RESOURCE_URL}" \
    "${DEPLOYER_TOKEN}" \
    '{"replicas":3}'
)"

if jq -e \
  --arg image "${E2E_IMAGE}" \
  --argjson port "${E2E_PORT}" \
  '
    .replicas == 3 and
    .image == $image and
    .port == $port
  ' \
  <<<"${PATCH_RESPONSE}" \
  >/dev/null; then
  pass "PATCH changed only replicas and preserved other fields"
else
  fail "PATCH changed only replicas and preserved other fields"
  echo "${PATCH_RESPONSE}" | jq . 2>/dev/null || echo "${PATCH_RESPONSE}"
fi

wait_for_reconciliation \
  "PATCH generation reconciled and Ready"

echo
echo "13. Checking status endpoint"

STATUS_RESPONSE="$(
  request_json \
    GET \
    "${STATUS_URL}" \
    "${DEPLOYER_TOKEN}"
)"

if jq -e \
  '
    .desiredReplicas == 3
  ' \
  <<<"${STATUS_RESPONSE}" \
  >/dev/null; then
  pass "status endpoint reports desired replicas 3"
else
  fail "status endpoint reports desired replicas 3"
  echo "${STATUS_RESPONSE}" | jq . 2>/dev/null || echo "${STATUS_RESPONSE}"
fi

echo
echo "14. Checking missing JWT protection"

expect_status \
  "mutation without JWT is rejected" \
  "401" \
  --request PATCH \
  --header "Content-Type: application/json" \
  --data '{"replicas":2}' \
  "${RESOURCE_URL}"

if [[ -n "${VIEWER_TOKEN}" ]]; then
  echo
  echo "15. Checking viewer mutation denial"

  expect_status \
    "viewer cannot PATCH ModelService" \
    "403" \
    --request PATCH \
    --header "Authorization: Bearer ${VIEWER_TOKEN}" \
    --header "Content-Type: application/json" \
    --data '{"replicas":2}' \
    "${RESOURCE_URL}"
else
  echo
  echo "15. Viewer mutation denial skipped"
fi

echo
echo "16. Checking deployer cannot DELETE"

expect_status \
  "deployer cannot DELETE ModelService" \
  "403" \
  --request DELETE \
  --header "Authorization: Bearer ${DEPLOYER_TOKEN}" \
  "${RESOURCE_URL}"

echo
echo "17. Deleting ModelService with platform-admin token"

expect_status \
  "admin can DELETE ModelService" \
  "204" \
  --request DELETE \
  --header "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${RESOURCE_URL}"

echo
echo "18. Waiting for Kubernetes deletion"

wait_for_deletion

echo
echo "19. Verifying resource is absent through API"

expect_status \
  "deleted ModelService returns 404" \
  "404" \
  --header "Authorization: Bearer ${DEPLOYER_TOKEN}" \
  "${RESOURCE_URL}"

trap - EXIT

unset DEPLOYER_TOKEN
unset ADMIN_TOKEN
unset VIEWER_TOKEN

echo
echo "========================================"
echo "Validation summary"
echo "========================================"
echo "PASS: ${PASS_COUNT}"
echo "FAIL: ${FAIL_COUNT}"

if [[ "${FAIL_COUNT}" -ne 0 ]]; then
  echo
  echo "FAIL: AI Platform API CRUD workflow validation failed."
  exit 1
fi

echo
echo "PASS: AI Platform API CRUD workflow validated."
