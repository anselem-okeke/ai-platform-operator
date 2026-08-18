#!/usr/bin/env bash
set -euo pipefail

REALM="${REALM:-ai-platform}"
ARGOCD_CLIENT_ID="${ARGOCD_CLIENT_ID:-ai-platform-argocd}"
GROUP_SCOPE_NAME="${GROUP_SCOPE_NAME:-groups}"

ARGOCD_URL="${ARGOCD_URL:-https://argocd.ai-platform.local}"

KCADM="${KCADM:-/opt/keycloak/bin/kcadm.sh}"
KCADM_CONFIG="${KCADM_CONFIG:-/tmp/ai-platform-kcadm.config}"

: "${KC_BOOTSTRAP_ADMIN_USERNAME:?KC_BOOTSTRAP_ADMIN_USERNAME is required}"
: "${KC_BOOTSTRAP_ADMIN_PASSWORD:?KC_BOOTSTRAP_ADMIN_PASSWORD is required}"

echo "Configuring Keycloak Argo CD OIDC integration..."

"${KCADM}" config credentials \
  --config "${KCADM_CONFIG}" \
  --server http://127.0.0.1:8080 \
  --realm master \
  --user "${KC_BOOTSTRAP_ADMIN_USERNAME}" \
  --password "${KC_BOOTSTRAP_ADMIN_PASSWORD}"

kcadm() {
  "${KCADM}" \
    "$@" \
    --config "${KCADM_CONFIG}"
}

get_group_id() {
  local group_name="$1"

  kcadm get groups \
    -r "${REALM}" \
    --fields id,name |
  jq -r \
    --arg name "${group_name}" \
    '.[] | select(.name == $name) | .id' |
  head -n1
}

ensure_group() {
  local group_name="$1"
  local group_id

  group_id="$(get_group_id "${group_name}")"

  if [[ -n "${group_id}" ]]; then
    echo "Group exists: ${group_name}"
    return
  fi

  echo "Creating group: ${group_name}"

  kcadm create groups \
    -r "${REALM}" \
    -s "name=${group_name}"
}

get_user_id() {
  local username="$1"

  kcadm get users \
    -r "${REALM}" \
    -q "username=${username}" \
    --fields id,username |
  jq -r \
    --arg username "${username}" \
    '.[] | select(.username == $username) | .id' |
  head -n1
}

ensure_user_group_membership() {
  local username="$1"
  local group_name="$2"

  local user_id
  local group_id

  user_id="$(get_user_id "${username}")"
  group_id="$(get_group_id "${group_name}")"

  if [[ -z "${user_id}" ]]; then
    echo "ERROR: user not found: ${username}" >&2
    exit 1
  fi

  if [[ -z "${group_id}" ]]; then
    echo "ERROR: group not found: ${group_name}" >&2
    exit 1
  fi

  if kcadm get "users/${user_id}/groups" \
      -r "${REALM}" \
      --fields id,name |
      jq -e \
        --arg id "${group_id}" \
        '.[] | select(.id == $id)' \
        >/dev/null; then

    echo "Membership exists: ${username} -> ${group_name}"
    return
  fi

  echo "Adding membership: ${username} -> ${group_name}"

  kcadm update \
    "users/${user_id}/groups/${group_id}" \
    -r "${REALM}"
}

get_client_uuid() {
  local client_id="$1"

  kcadm get clients \
    -r "${REALM}" \
    -q "clientId=${client_id}" \
    --fields id,clientId |
  jq -r \
    --arg client_id "${client_id}" \
    '.[] | select(.clientId == $client_id) | .id' |
  head -n1
}

ensure_argocd_client() {
  local client_uuid

  client_uuid="$(get_client_uuid "${ARGOCD_CLIENT_ID}")"

  if [[ -z "${client_uuid}" ]]; then
    echo "Creating client: ${ARGOCD_CLIENT_ID}"

    kcadm create clients \
      -r "${REALM}" \
      -s "clientId=${ARGOCD_CLIENT_ID}" \
      -s enabled=true \
      -s publicClient=true \
      -s standardFlowEnabled=true \
      -s implicitFlowEnabled=false \
      -s directAccessGrantsEnabled=false \
      -s serviceAccountsEnabled=false \
      -s "rootUrl=${ARGOCD_URL}" \
      -s "baseUrl=/applications" \
      -s 'redirectUris=["https://argocd.ai-platform.local/auth/callback","https://argocd.ai-platform.local/pkce/verify","http://localhost:8085/auth/callback"]' \
      -s 'webOrigins=["https://argocd.ai-platform.local"]' \
      -s 'attributes={"pkce.code.challenge.method":"S256","post.logout.redirect.uris":"https://argocd.ai-platform.local/applications"}'

    client_uuid="$(get_client_uuid "${ARGOCD_CLIENT_ID}")"

    if [[ -z "${client_uuid}" ]]; then
      echo "ERROR: failed to create ${ARGOCD_CLIENT_ID}" >&2
      exit 1
    fi
  else
    echo "Client exists: ${ARGOCD_CLIENT_ID}"

    echo "Reconciling client configuration..."

    kcadm update \
      "clients/${client_uuid}" \
      -r "${REALM}" \
      -s enabled=true \
      -s publicClient=true \
      -s standardFlowEnabled=true \
      -s implicitFlowEnabled=false \
      -s directAccessGrantsEnabled=false \
      -s serviceAccountsEnabled=false \
      -s "rootUrl=${ARGOCD_URL}" \
      -s "baseUrl=/applications" \
      -s 'redirectUris=["https://argocd.ai-platform.local/auth/callback","https://argocd.ai-platform.local/pkce/verify","http://localhost:8085/auth/callback"]' \
      -s 'webOrigins=["https://argocd.ai-platform.local"]' \
      -s 'attributes={"pkce.code.challenge.method":"S256","post.logout.redirect.uris":"https://argocd.ai-platform.local/applications"}'
  fi
}

get_client_scope_id() {
  local scope_name="$1"

  kcadm get client-scopes \
    -r "${REALM}" \
    --fields id,name |
  jq -r \
    --arg name "${scope_name}" \
    '.[] | select(.name == $name) | .id' |
  head -n1
}

ensure_groups_scope() {
  local scope_id

  scope_id="$(get_client_scope_id "${GROUP_SCOPE_NAME}")"

  if [[ -z "${scope_id}" ]]; then
    echo "Creating client scope: ${GROUP_SCOPE_NAME}"

    kcadm create client-scopes \
      -r "${REALM}" \
      -s "name=${GROUP_SCOPE_NAME}" \
      -s protocol=openid-connect \
      -s 'attributes={"include.in.token.scope":"true","display.on.consent.screen":"false"}'

    scope_id="$(get_client_scope_id "${GROUP_SCOPE_NAME}")"

    if [[ -z "${scope_id}" ]]; then
      echo "ERROR: failed to create groups client scope" >&2
      exit 1
    fi
  else
    echo "Client scope exists: ${GROUP_SCOPE_NAME}"
  fi
}

ensure_groups_mapper() {
  local scope_id
  local mapper_id

  scope_id="$(get_client_scope_id "${GROUP_SCOPE_NAME}")"

  mapper_id="$(
    kcadm get \
      "client-scopes/${scope_id}/protocol-mappers/models" \
      -r "${REALM}" |
    jq -r \
      '.[] | select(.name == "groups") | .id' |
    head -n1
  )"

  if [[ -n "${mapper_id}" ]]; then
    echo "Groups mapper exists"
    return
  fi

  echo "Creating groups protocol mapper"

  kcadm create \
    "client-scopes/${scope_id}/protocol-mappers/models" \
    -r "${REALM}" \
    -s name=groups \
    -s protocol=openid-connect \
    -s protocolMapper=oidc-group-membership-mapper \
    -s consentRequired=false \
    -s 'config={"full.path":"false","claim.name":"groups","id.token.claim":"true","access.token.claim":"true","userinfo.token.claim":"true"}'
}

ensure_default_scope_on_client() {
  local client_id="$1"

  local client_uuid
  local scope_id

  client_uuid="$(get_client_uuid "${client_id}")"
  scope_id="$(get_client_scope_id "${GROUP_SCOPE_NAME}")"

  if [[ -z "${client_uuid}" ]]; then
    echo "ERROR: client not found: ${client_id}" >&2
    exit 1
  fi

  if kcadm get \
      "clients/${client_uuid}/default-client-scopes" \
      -r "${REALM}" |
      jq -e \
        --arg id "${scope_id}" \
        '.[] | select(.id == $id)' \
        >/dev/null; then

    echo "Default scope already attached: ${client_id} -> ${GROUP_SCOPE_NAME}"
    return
  fi

  echo "Attaching default scope: ${client_id} -> ${GROUP_SCOPE_NAME}"

  kcadm update \
    "clients/${client_uuid}/default-client-scopes/${scope_id}" \
    -r "${REALM}"
}

echo
echo "=== Groups ==="

ensure_group platform-viewer
ensure_group platform-deployer
ensure_group platform-admin

echo
echo "=== User memberships ==="

ensure_user_group_membership viewer-user platform-viewer
ensure_user_group_membership deployer-user platform-deployer
ensure_user_group_membership admin-user platform-admin

echo
echo "=== Argo CD client ==="

ensure_argocd_client

echo
echo "=== Groups client scope ==="

ensure_groups_scope
ensure_groups_mapper

echo
echo "=== Attach groups scope ==="

ensure_default_scope_on_client ai-platform-argocd
ensure_default_scope_on_client ai-platform-cli

echo
echo "PASS: Keycloak Argo CD OIDC configuration completed."
