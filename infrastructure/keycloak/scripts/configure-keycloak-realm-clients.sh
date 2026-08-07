#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-keycloak}"

REALM="${KEYCLOAK_REALM:-ai-platform}"
DISPLAY_NAME="${KEYCLOAK_DISPLAY_NAME:-AI Platform}"
EXTERNAL_URL="${KEYCLOAK_EXTERNAL_URL:-https://auth.ai-platform.local}"

CLI_CLIENT_ID="${CLI_CLIENT_ID:-ai-platform-cli}"
SERVICE_CLIENT_ID="${SERVICE_CLIENT_ID:-ai-platform-service}"
AUDIENCE_CLIENT_ID="${AUDIENCE_CLIENT_ID:-ai-platform-gateway}"

KCADM_CONFIG="${KCADM_CONFIG:-/tmp/ai-platform-kcadm.config}"

SERVICE_CLIENT_SECRET_NAME="${SERVICE_CLIENT_SECRET_NAME:-ai-platform-service-client-credentials}"

for command_name in kubectl jq base64; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "ERROR: Required command not found: ${command_name}" >&2
    exit 1
  fi
done

keycloak_pod="$(
  kubectl get pods \
    --namespace "${NAMESPACE}" \
    --selector app.kubernetes.io/name=keycloak \
    --field-selector status.phase=Running \
    --output jsonpath='{.items[0].metadata.name}' \
    2>/dev/null || true
)"

if [[ -z "${keycloak_pod}" ]]; then
  echo "ERROR: No running Keycloak Pod found in namespace ${NAMESPACE}." >&2
  exit 1
fi

echo "Using Keycloak Pod: ${keycloak_pod}"

if ! kubectl get secret \
  "${SERVICE_CLIENT_SECRET_NAME}" \
  --namespace "${NAMESPACE}" \
  >/dev/null 2>&1; then
  echo "ERROR: Secret ${SERVICE_CLIENT_SECRET_NAME} was not found in namespace ${NAMESPACE}." >&2
  exit 1
fi

service_client_secret="$(
  kubectl get secret \
    "${SERVICE_CLIENT_SECRET_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.data.CLIENT_SECRET}' |
  base64 --decode
)"

if [[ -z "${service_client_secret}" ]]; then
  echo "ERROR: Service-client secret is empty." >&2
  exit 1
fi

cleanup() {
  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    rm -f "${KCADM_CONFIG}" \
    >/dev/null 2>&1 || true

  unset service_client_secret
}

trap cleanup EXIT

login_admin() {
  echo "Authenticating Keycloak Admin CLI..."

  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    sh -ec "
      rm -f '${KCADM_CONFIG}'

      /opt/keycloak/bin/kcadm.sh \
        config credentials \
        --server http://127.0.0.1:8080 \
        --realm master \
        --user \"\${KC_BOOTSTRAP_ADMIN_USERNAME}\" \
        --password \"\${KC_BOOTSTRAP_ADMIN_PASSWORD}\" \
        --config '${KCADM_CONFIG}'
    "
}

kcadm() {
  if [[ $# -lt 1 ]]; then
    echo "ERROR: kcadm requires a command." >&2
    return 1
  fi

  local command_name="$1"
  shift

  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    /opt/keycloak/bin/kcadm.sh \
    "${command_name}" \
    "$@" \
    --config "${KCADM_CONFIG}"
}

get_client_uuid() {
  local client_id="$1"

  kcadm get clients \
    -r "${REALM}" \
    -q "clientId=${client_id}" |
  jq -r \
    --arg client_id "${client_id}" \
    '.[] | select(.clientId == $client_id) | .id' |
  head -n 1
}

ensure_audience_mapper() {
  local client_uuid="$1"
  local client_name="$2"
  local mapper_name="audience-${AUDIENCE_CLIENT_ID}"
  local mapper_uuid

  mapper_uuid="$(
    kcadm get \
      "clients/${client_uuid}/protocol-mappers/models" \
      -r "${REALM}" |
    jq -r \
      --arg mapper_name "${mapper_name}" \
      '.[] | select(.name == $mapper_name) | .id' |
    head -n 1
  )"

  if [[ -n "${mapper_uuid}" ]]; then
    echo "Updating audience mapper for ${client_name}..."

    kcadm update \
      "clients/${client_uuid}/protocol-mappers/models/${mapper_uuid}" \
      -r "${REALM}" \
      -s "name=${mapper_name}" \
      -s protocol=openid-connect \
      -s protocolMapper=oidc-audience-mapper \
      -s consentRequired=false \
      -s "config.\"included.client.audience\"=${AUDIENCE_CLIENT_ID}" \
      -s 'config."id.token.claim"=false' \
      -s 'config."access.token.claim"=true' \
      >/dev/null

    return
  fi

  echo "Adding audience mapper to ${client_name}..."

  kcadm create \
    "clients/${client_uuid}/protocol-mappers/models" \
    -r "${REALM}" \
    -s "name=${mapper_name}" \
    -s protocol=openid-connect \
    -s protocolMapper=oidc-audience-mapper \
    -s consentRequired=false \
    -s "config.\"included.client.audience\"=${AUDIENCE_CLIENT_ID}" \
    -s 'config."id.token.claim"=false' \
    -s 'config."access.token.claim"=true' \
    >/dev/null
}

login_admin

if kcadm get "realms/${REALM}" >/dev/null 2>&1; then
  echo "Updating realm ${REALM}..."

  kcadm update \
    "realms/${REALM}" \
    -s "displayName=${DISPLAY_NAME}" \
    -s enabled=true \
    -s registrationAllowed=false \
    -s registrationEmailAsUsername=false \
    -s resetPasswordAllowed=true \
    -s rememberMe=true \
    -s loginWithEmailAllowed=true \
    -s duplicateEmailsAllowed=false \
    -s verifyEmail=false \
    -s sslRequired=external \
    >/dev/null
else
  echo "Creating realm ${REALM}..."

  kcadm create realms \
    -s "realm=${REALM}" \
    -s "displayName=${DISPLAY_NAME}" \
    -s enabled=true \
    -s registrationAllowed=false \
    -s registrationEmailAsUsername=false \
    -s resetPasswordAllowed=true \
    -s rememberMe=true \
    -s loginWithEmailAllowed=true \
    -s duplicateEmailsAllowed=false \
    -s verifyEmail=false \
    -s sslRequired=external \
    >/dev/null
fi

echo "Configuring resource-server audience client..."

audience_uuid="$(get_client_uuid "${AUDIENCE_CLIENT_ID}")"

if [[ -z "${audience_uuid}" ]]; then
  kcadm create clients \
    -r "${REALM}" \
    -s "clientId=${AUDIENCE_CLIENT_ID}" \
    -s "name=AI Platform Gateway" \
    -s enabled=true \
    -s bearerOnly=true \
    -s publicClient=false \
    -s standardFlowEnabled=false \
    -s implicitFlowEnabled=false \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=false \
    >/dev/null

  audience_uuid="$(get_client_uuid "${AUDIENCE_CLIENT_ID}")"
else
  kcadm update \
    "clients/${audience_uuid}" \
    -r "${REALM}" \
    -s "name=AI Platform Gateway" \
    -s enabled=true \
    -s bearerOnly=true \
    -s publicClient=false \
    -s standardFlowEnabled=false \
    -s implicitFlowEnabled=false \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=false \
    >/dev/null
fi

if [[ -z "${audience_uuid}" ]]; then
  echo "ERROR: Unable to resolve audience client UUID." >&2
  exit 1
fi

echo "Configuring public CLI client..."

cli_uuid="$(get_client_uuid "${CLI_CLIENT_ID}")"

if [[ -z "${cli_uuid}" ]]; then
  kcadm create clients \
    -r "${REALM}" \
    -s "clientId=${CLI_CLIENT_ID}" \
    -s "name=AI Platform CLI" \
    -s enabled=true \
    -s publicClient=true \
    -s bearerOnly=false \
    -s standardFlowEnabled=true \
    -s implicitFlowEnabled=false \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=false \
    -s 'redirectUris=["http://127.0.0.1:18080/*","http://localhost:18080/*"]' \
    -s 'webOrigins=["http://127.0.0.1:18080","http://localhost:18080"]' \
    -s 'attributes."pkce.code.challenge.method"=S256' \
    >/dev/null

  cli_uuid="$(get_client_uuid "${CLI_CLIENT_ID}")"
else
  kcadm update \
    "clients/${cli_uuid}" \
    -r "${REALM}" \
    -s "name=AI Platform CLI" \
    -s enabled=true \
    -s publicClient=true \
    -s bearerOnly=false \
    -s standardFlowEnabled=true \
    -s implicitFlowEnabled=false \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=false \
    -s 'redirectUris=["http://127.0.0.1:18080/*","http://localhost:18080/*"]' \
    -s 'webOrigins=["http://127.0.0.1:18080","http://localhost:18080"]' \
    -s 'attributes."pkce.code.challenge.method"=S256' \
    >/dev/null
fi

if [[ -z "${cli_uuid}" ]]; then
  echo "ERROR: Unable to resolve CLI client UUID." >&2
  exit 1
fi

echo "Configuring machine-to-machine client..."

service_uuid="$(get_client_uuid "${SERVICE_CLIENT_ID}")"

if [[ -z "${service_uuid}" ]]; then
  kcadm create clients \
    -r "${REALM}" \
    -s "clientId=${SERVICE_CLIENT_ID}" \
    -s "name=AI Platform Service" \
    -s enabled=true \
    -s publicClient=false \
    -s bearerOnly=false \
    -s clientAuthenticatorType=client-secret \
    -s "secret=${service_client_secret}" \
    -s standardFlowEnabled=false \
    -s implicitFlowEnabled=false \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=true \
    -s authorizationServicesEnabled=false \
    >/dev/null

  service_uuid="$(get_client_uuid "${SERVICE_CLIENT_ID}")"
else
  kcadm update \
    "clients/${service_uuid}" \
    -r "${REALM}" \
    -s "name=AI Platform Service" \
    -s enabled=true \
    -s publicClient=false \
    -s bearerOnly=false \
    -s clientAuthenticatorType=client-secret \
    -s "secret=${service_client_secret}" \
    -s standardFlowEnabled=false \
    -s implicitFlowEnabled=false \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=true \
    -s authorizationServicesEnabled=false \
    >/dev/null
fi

if [[ -z "${service_uuid}" ]]; then
  echo "ERROR: Unable to resolve service client UUID." >&2
  exit 1
fi

ensure_audience_mapper \
  "${cli_uuid}" \
  "${CLI_CLIENT_ID}"

ensure_audience_mapper \
  "${service_uuid}" \
  "${SERVICE_CLIENT_ID}"

echo
echo "Realm and clients configured successfully."
echo "Realm:            ${REALM}"
echo "Audience client:  ${AUDIENCE_CLIENT_ID}"
echo "User client:      ${CLI_CLIENT_ID}"
echo "Service client:   ${SERVICE_CLIENT_ID}"
echo "External issuer:  ${EXTERNAL_URL}/realms/${REALM}"
