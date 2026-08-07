#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-keycloak}"
REALM="${KEYCLOAK_REALM:-ai-platform}"

VIEWER_ROLE="${VIEWER_ROLE:-model-viewer}"
DEPLOYER_ROLE="${DEPLOYER_ROLE:-model-deployer}"
ADMIN_ROLE="${ADMIN_ROLE:-platform-admin}"

VIEWER_USERNAME="${VIEWER_USERNAME:-viewer-user}"
VIEWER_EMAIL="${VIEWER_EMAIL:-viewer-user@ai-platform.local}"
VIEWER_FIRST_NAME="${VIEWER_FIRST_NAME:-Viewer}"
VIEWER_LAST_NAME="${VIEWER_LAST_NAME:-User}"

DEPLOYER_USERNAME="${DEPLOYER_USERNAME:-deployer-user}"
DEPLOYER_EMAIL="${DEPLOYER_EMAIL:-deployer-user@ai-platform.local}"
DEPLOYER_FIRST_NAME="${DEPLOYER_FIRST_NAME:-Deployer}"
DEPLOYER_LAST_NAME="${DEPLOYER_LAST_NAME:-User}"

ADMIN_USERNAME="${ADMIN_USERNAME:-admin-user}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin-user@ai-platform.local}"
ADMIN_FIRST_NAME="${ADMIN_FIRST_NAME:-Platform}"
ADMIN_LAST_NAME="${ADMIN_LAST_NAME:-Administrator}"

SERVICE_CLIENT_ID="${SERVICE_CLIENT_ID:-ai-platform-service}"

KCADM_CONFIG="${KCADM_CONFIG:-/tmp/ai-platform-roles-users-kcadm.config}"

: "${VIEWER_PASSWORD:?VIEWER_PASSWORD is required}"
: "${DEPLOYER_PASSWORD:?DEPLOYER_PASSWORD is required}"
: "${ADMIN_PASSWORD:?ADMIN_PASSWORD is required}"

for command_name in kubectl jq; do
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

cleanup() {
  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    rm -f "${KCADM_CONFIG}" \
    >/dev/null 2>&1 || true
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

realm_exists() {
  kcadm get "realms/${REALM}" \
    >/dev/null 2>&1
}

role_exists() {
  local role_name="$1"

  kcadm get \
    "roles/${role_name}" \
    -r "${REALM}" \
    >/dev/null 2>&1
}

ensure_role() {
  local role_name="$1"
  local description="$2"

  if role_exists "${role_name}"; then
    echo "Updating role ${role_name}..."

    kcadm update \
      "roles/${role_name}" \
      -r "${REALM}" \
      -s "name=${role_name}" \
      -s "description=${description}" \
      >/dev/null
  else
    echo "Creating role ${role_name}..."

    kcadm create roles \
      -r "${REALM}" \
      -s "name=${role_name}" \
      -s "description=${description}" \
      >/dev/null
  fi
}

get_role_json() {
  local role_name="$1"

  kcadm get \
    "roles/${role_name}" \
    -r "${REALM}"
}

role_is_composite_member() {
  local parent_role="$1"
  local child_role="$2"

  kcadm get \
    "roles/${parent_role}/composites/realm" \
    -r "${REALM}" |
  jq -e \
    --arg child_role "${child_role}" \
    'any(.[]; .name == $child_role)' \
    >/dev/null
}

ensure_composite_member() {
  local parent_role="$1"
  local child_role="$2"
  local role_payload

  if role_is_composite_member "${parent_role}" "${child_role}"; then
    echo "${parent_role} already includes ${child_role}."
    return
  fi

  echo "Adding ${child_role} to composite role ${parent_role}..."

  role_payload="$(
    get_role_json "${child_role}" |
    jq -c '[.]'
  )"

  printf '%s\n' "${role_payload}" |
  kubectl exec \
    --stdin \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    /opt/keycloak/bin/kcadm.sh \
      create \
      "roles/${parent_role}/composites" \
      -r "${REALM}" \
      -f - \
      --config "${KCADM_CONFIG}" \
      >/dev/null
}

get_user_id() {
  local username="$1"

  kcadm get users \
    -r "${REALM}" \
    -q "username=${username}" |
  jq -r \
    --arg username "${username}" \
    '.[] | select(.username == $username) | .id' |
  head -n 1
}

ensure_user() {
  local username="$1"
  local email="$2"
  local first_name="$3"
  local last_name="$4"
  local password="$5"
  local user_id

  user_id="$(get_user_id "${username}")"

  if [[ -z "${user_id}" ]]; then
    echo "Creating user ${username}..."

    kcadm create users \
      -r "${REALM}" \
      -s "username=${username}" \
      -s "email=${email}" \
      -s "firstName=${first_name}" \
      -s "lastName=${last_name}" \
      -s enabled=true \
      -s emailVerified=true \
      -s 'requiredActions=[]' \
      >/dev/null

    user_id="$(get_user_id "${username}")"
  else
    echo "Updating user ${username}..."

    kcadm update \
      "users/${user_id}" \
      -r "${REALM}" \
      -s "email=${email}" \
      -s "firstName=${first_name}" \
      -s "lastName=${last_name}" \
      -s enabled=true \
      -s emailVerified=true \
      -s 'requiredActions=[]' \
      >/dev/null
  fi

  if [[ -z "${user_id}" ]]; then
    echo "ERROR: Could not resolve user ID for ${username}." >&2
    exit 1
  fi

  echo "Setting password for ${username}..."

  kcadm set-password \
    -r "${REALM}" \
    --userid "${user_id}" \
    --new-password "${password}" \
    --temporary=false \
    >/dev/null

  printf '%s\n' "${user_id}"
}

user_has_role() {
  local user_id="$1"
  local role_name="$2"

  kcadm get \
    "users/${user_id}/role-mappings/realm" \
    -r "${REALM}" |
  jq -e \
    --arg role_name "${role_name}" \
    'any(.[]; .name == $role_name)' \
    >/dev/null
}

ensure_user_role() {
  local user_id="$1"
  local username="$2"
  local role_name="$3"

  if user_has_role "${user_id}" "${role_name}"; then
    echo "${username} already has role ${role_name}."
    return
  fi

  echo "Assigning ${role_name} to ${username}..."

  kcadm add-roles \
    -r "${REALM}" \
    --uid "${user_id}" \
    --rolename "${role_name}" \
    >/dev/null
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

get_service_account_user_id() {
  local client_uuid="$1"

  kcadm get \
    "clients/${client_uuid}/service-account-user" \
    -r "${REALM}" |
  jq -r '.id'
}

login_admin

if ! realm_exists; then
  echo "ERROR: Realm ${REALM} does not exist." >&2
  echo "Run configure-keycloak-realm-clients.sh first." >&2
  exit 1
fi

echo
echo "Configuring AI Platform realm roles..."

ensure_role \
  "${VIEWER_ROLE}" \
  "Read AI Platform model-service resources"

ensure_role \
  "${DEPLOYER_ROLE}" \
  "Create and update AI Platform model-service resources"

ensure_role \
  "${ADMIN_ROLE}" \
  "Administer AI Platform model-service resources"

echo
echo "Configuring composite role hierarchy..."

ensure_composite_member \
  "${DEPLOYER_ROLE}" \
  "${VIEWER_ROLE}"

ensure_composite_member \
  "${ADMIN_ROLE}" \
  "${DEPLOYER_ROLE}"

echo
echo "Configuring test users..."

viewer_user_id="$(
  ensure_user \
    "${VIEWER_USERNAME}" \
    "${VIEWER_EMAIL}" \
    "${VIEWER_FIRST_NAME}" \
    "${VIEWER_LAST_NAME}" \
    "${VIEWER_PASSWORD}" |
  tail -n 1
)"

deployer_user_id="$(
  ensure_user \
    "${DEPLOYER_USERNAME}" \
    "${DEPLOYER_EMAIL}" \
    "${DEPLOYER_FIRST_NAME}" \
    "${DEPLOYER_LAST_NAME}" \
    "${DEPLOYER_PASSWORD}" |
  tail -n 1
)"

admin_user_id="$(
  ensure_user \
    "${ADMIN_USERNAME}" \
    "${ADMIN_EMAIL}" \
    "${ADMIN_FIRST_NAME}" \
    "${ADMIN_LAST_NAME}" \
    "${ADMIN_PASSWORD}" |
  tail -n 1
)"

ensure_user_role \
  "${viewer_user_id}" \
  "${VIEWER_USERNAME}" \
  "${VIEWER_ROLE}"

ensure_user_role \
  "${deployer_user_id}" \
  "${DEPLOYER_USERNAME}" \
  "${DEPLOYER_ROLE}"

ensure_user_role \
  "${admin_user_id}" \
  "${ADMIN_USERNAME}" \
  "${ADMIN_ROLE}"

echo
echo "Configuring machine-to-machine role..."

service_client_uuid="$(get_client_uuid "${SERVICE_CLIENT_ID}")"

if [[ -z "${service_client_uuid}" ]]; then
  echo "ERROR: Client ${SERVICE_CLIENT_ID} does not exist." >&2
  exit 1
fi

service_account_user_id="$(
  get_service_account_user_id "${service_client_uuid}"
)"

if [[ -z "${service_account_user_id}" || "${service_account_user_id}" == "null" ]]; then
  echo "ERROR: Service-account user for ${SERVICE_CLIENT_ID} was not found." >&2
  exit 1
fi

ensure_user_role \
  "${service_account_user_id}" \
  "service-account-${SERVICE_CLIENT_ID}" \
  "${DEPLOYER_ROLE}"

echo
echo "Roles and test users configured successfully."
echo "Realm:            ${REALM}"
echo "Viewer role:      ${VIEWER_ROLE}"
echo "Deployer role:    ${DEPLOYER_ROLE}"
echo "Administrator:    ${ADMIN_ROLE}"
echo "Viewer user:      ${VIEWER_USERNAME}"
echo "Deployer user:    ${DEPLOYER_USERNAME}"
echo "Admin user:       ${ADMIN_USERNAME}"
echo "Service account:  service-account-${SERVICE_CLIENT_ID}"
