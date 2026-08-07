# Roles, Users, and Service Accounts

## Purpose

This document describes how the AI Platform realm roles, human test users, and machine service account were created and validated in Keycloak.

It covers:

- the application role hierarchy;
- creation of human users;
- creation and use of the Keycloak service-account identity;
- assignment of direct and composite roles;
- separation between AI Platform roles and Keycloak administration roles;
- idempotent automation;
- validation and troubleshooting.

The configuration targets:

```text
Realm: ai-platform
Keycloak: https://auth.ai-platform.local
Machine client: ai-platform-service
```

---

## Final Authorization Model

The platform uses three realm roles:

```text
platform-admin
  └── model-deployer
        └── model-viewer
```

This creates the following effective permissions:

```text
model-viewer
  └── read-only access

model-deployer
  ├── model-viewer
  └── deployment-changing access

platform-admin
  ├── model-deployer
  ├── model-viewer
  └── full AI Platform application access
```

These are application roles for the AI Platform.

They are not Keycloak administration roles.

---

## Identities

### Human users

```text
viewer-user
  → model-viewer

deployer-user
  → model-deployer

admin-user
  → platform-admin
```

### Machine identity

```text
service-account-ai-platform-service
  → model-deployer
```

The machine identity is created automatically by Keycloak because the client:

```text
ai-platform-service
```

has:

```text
serviceAccountsEnabled=true
```

---

## Files Created

```text
infrastructure/keycloak/config/authorization.env
infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
config/platform/keycloak/.secrets/test-users.env
.local/keycloak/test-users/viewer-user-password
.local/keycloak/test-users/deployer-user-password
.local/keycloak/test-users/admin-user-password
```

The files under:

```text
config/platform/keycloak/.secrets/
.local/keycloak/
```

must remain excluded from Git.

---

## Non-Secret Authorization Configuration

Create:

```text
infrastructure/keycloak/config/authorization.env
```

```bash
cat > infrastructure/keycloak/config/authorization.env <<'EOF'
KEYCLOAK_REALM=ai-platform

VIEWER_ROLE=model-viewer
DEPLOYER_ROLE=model-deployer
ADMIN_ROLE=platform-admin

VIEWER_USERNAME=viewer-user
VIEWER_EMAIL=viewer-user@ai-platform.local
VIEWER_FIRST_NAME=Viewer
VIEWER_LAST_NAME=User

DEPLOYER_USERNAME=deployer-user
DEPLOYER_EMAIL=deployer-user@ai-platform.local
DEPLOYER_FIRST_NAME=Deployer
DEPLOYER_LAST_NAME=User

ADMIN_USERNAME=admin-user
ADMIN_EMAIL=admin-user@ai-platform.local
ADMIN_FIRST_NAME=Platform
ADMIN_LAST_NAME=Administrator

SERVICE_CLIENT_ID=ai-platform-service
EOF
```

This file contains no passwords or client secrets and is safe to commit.

---

## Generate Local Test-User Passwords

Generate strong local passwords:

```bash
VIEWER_PASSWORD="$(
  openssl rand -base64 36 |
  tr -d '\n'
)"

DEPLOYER_PASSWORD="$(
  openssl rand -base64 36 |
  tr -d '\n'
)"

ADMIN_PASSWORD="$(
  openssl rand -base64 36 |
  tr -d '\n'
)"
```

Create the local secret file:

```bash
cat > config/platform/keycloak/.secrets/test-users.env <<EOF
VIEWER_PASSWORD=${VIEWER_PASSWORD}
DEPLOYER_PASSWORD=${DEPLOYER_PASSWORD}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
EOF
```

Restrict access:

```bash
chmod 600 \
  config/platform/keycloak/.secrets/test-users.env
```

Store local copies for later browser-based PKCE tests:

```bash
mkdir -p .local/keycloak/test-users
chmod 700 .local/keycloak/test-users
```

```bash
printf '%s\n' "${VIEWER_PASSWORD}" \
  > .local/keycloak/test-users/viewer-user-password

printf '%s\n' "${DEPLOYER_PASSWORD}" \
  > .local/keycloak/test-users/deployer-user-password

printf '%s\n' "${ADMIN_PASSWORD}" \
  > .local/keycloak/test-users/admin-user-password
```

```bash
chmod 600 \
  .local/keycloak/test-users/*
```

Clear the temporary shell variables:

```bash
unset VIEWER_PASSWORD
unset DEPLOYER_PASSWORD
unset ADMIN_PASSWORD
```

---

## Confirm Git Exclusions

Verify that the files are ignored:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/test-users.env \
  .local/keycloak/test-users/viewer-user-password \
  .local/keycloak/test-users/deployer-user-password \
  .local/keycloak/test-users/admin-user-password
```

Expected: each file is matched by `.gitignore`.

Confirm that none are tracked:

```bash
git ls-files \
  config/platform/keycloak/.secrets/test-users.env \
  .local/keycloak/test-users
```

Expected: no output.

---

## Update the Example Environment File

Create or update:

```text
infrastructure/keycloak/variables.env.example
```

```bash
cat > infrastructure/keycloak/variables.env.example <<'EOF'
# PostgreSQL configuration
POSTGRES_DB=keycloak
POSTGRES_USER=keycloak
POSTGRES_PASSWORD=replace-with-generated-password

# Temporary Keycloak bootstrap administrator
KC_BOOTSTRAP_ADMIN_USERNAME=platform-admin
KC_BOOTSTRAP_ADMIN_PASSWORD=replace-with-generated-password

# Machine-to-machine OIDC client
CLIENT_ID=ai-platform-service
CLIENT_SECRET=replace-with-generated-client-secret

# Test-user credentials
VIEWER_PASSWORD=replace-with-generated-password
DEPLOYER_PASSWORD=replace-with-generated-password
ADMIN_PASSWORD=replace-with-generated-password
EOF
```

This file is safe to commit because it contains placeholders only.

---

## Idempotent Configuration Script

Create:

```text
infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
```

```bash
cat > infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh <<'EOF'
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

KCADM_CONFIG="/tmp/ai-platform-roles-users-kcadm.config"

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
    --output jsonpath='{.items[0].metadata.name}'
)"

if [[ -z "${keycloak_pod}" ]]; then
  echo "ERROR: No running Keycloak Pod found." >&2
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
    sh -ec '
      rm -f /tmp/ai-platform-roles-users-kcadm.config

      /opt/keycloak/bin/kcadm.sh config credentials \
        --server http://127.0.0.1:8080 \
        --realm master \
        --user "${KC_BOOTSTRAP_ADMIN_USERNAME}" \
        --password "${KC_BOOTSTRAP_ADMIN_PASSWORD}" \
        --config /tmp/ai-platform-roles-users-kcadm.config
    '
}

kcadm_raw() {
  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    /opt/keycloak/bin/kcadm.sh \
    "$@" \
    --config "${KCADM_CONFIG}"
}

kcadm() {
  local output
  local status

  set +e
  output="$(kcadm_raw "$@" 2>&1)"
  status=$?
  set -e

  if [[ ${status} -eq 0 ]]; then
    printf '%s\n' "${output}"
    return 0
  fi

  if grep -qE \
    '401 Unauthorized|Invalid token|Not authenticated' \
    <<<"${output}"
  then
    echo "Admin session rejected; authenticating again..." >&2
    login_admin >&2
    kcadm_raw "$@"
    return
  fi

  printf '%s\n' "${output}" >&2
  return "${status}"
}

realm_exists() {
  kcadm get "realms/${REALM}" \
    >/dev/null 2>&1
}

role_exists() {
  local role_name="$1"

  kcadm get \
    "roles/${role_name}" \
    --realm "${REALM}" \
    >/dev/null 2>&1
}

ensure_role() {
  local role_name="$1"
  local description="$2"

  if role_exists "${role_name}"; then
    echo "Updating role ${role_name}..."

    kcadm update \
      "roles/${role_name}" \
      --realm "${REALM}" \
      -s "name=${role_name}" \
      -s "description=${description}" \
      >/dev/null
  else
    echo "Creating role ${role_name}..."

    kcadm create roles \
      --realm "${REALM}" \
      -s "name=${role_name}" \
      -s "description=${description}" \
      >/dev/null
  fi
}

get_role_json() {
  local role_name="$1"

  kcadm get \
    "roles/${role_name}" \
    --realm "${REALM}"
}

role_is_composite_member() {
  local parent_role="$1"
  local child_role="$2"

  kcadm get \
    "roles/${parent_role}/composites/realm" \
    --realm "${REALM}" |
  jq -e \
    --arg child_role "${child_role}" \
    'any(.[]; .name == $child_role)' \
    >/dev/null
}

ensure_composite_member() {
  local parent_role="$1"
  local child_role="$2"
  local role_file

  if role_is_composite_member \
    "${parent_role}" \
    "${child_role}"
  then
    echo "${parent_role} already includes ${child_role}."
    return
  fi

  echo "Adding ${child_role} to composite role ${parent_role}..."

  role_file="$(mktemp)"

  get_role_json "${child_role}" |
  jq '[.]' \
    > "${role_file}"

  kubectl cp \
    "${role_file}" \
    "${NAMESPACE}/${keycloak_pod}:/tmp/composite-role.json" \
    >/dev/null

  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    /opt/keycloak/bin/kcadm.sh \
    create \
    "roles/${parent_role}/composites" \
    --realm "${REALM}" \
    -f /tmp/composite-role.json \
    --config "${KCADM_CONFIG}" \
    >/dev/null

  kubectl exec \
    --namespace "${NAMESPACE}" \
    "${keycloak_pod}" \
    -- \
    rm -f /tmp/composite-role.json

  rm -f "${role_file}"
}

get_user_id() {
  local username="$1"

  kcadm get users \
    --realm "${REALM}" \
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
      --realm "${REALM}" \
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
      --realm "${REALM}" \
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
    --realm "${REALM}" \
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
    --realm "${REALM}" |
  jq -e \
    --arg role_name "${role_name}" \
    'any(.[]; .name == $role_name)' \
    >/dev/null
}

ensure_user_role() {
  local user_id="$1"
  local username="$2"
  local role_name="$3"

  if user_has_role \
    "${user_id}" \
    "${role_name}"
  then
    echo "${username} already has role ${role_name}."
    return
  fi

  echo "Assigning ${role_name} to ${username}..."

  kcadm add-roles \
    --realm "${REALM}" \
    --uid "${user_id}" \
    --rolename "${role_name}" \
    >/dev/null
}

get_client_uuid() {
  local client_id="$1"

  kcadm get clients \
    --realm "${REALM}" \
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
    --realm "${REALM}" |
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

if [[ -z "${service_account_user_id}" ]]; then
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
EOF
```

Make the script executable:

```bash
chmod +x \
  infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
```

Validate its shell syntax:

```bash
bash -n \
  infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
```

Expected: no output.

---

## Load Configuration and Secrets

```bash
set -a
source infrastructure/keycloak/config/authorization.env
source config/platform/keycloak/.secrets/test-users.env
set +a
```

Confirm that the required variables are loaded without printing their values:

```bash
for variable_name in \
  VIEWER_PASSWORD \
  DEPLOYER_PASSWORD \
  ADMIN_PASSWORD
do
  if [[ -n "${!variable_name:-}" ]]; then
    echo "PASS: ${variable_name} is loaded"
  else
    echo "ERROR: ${variable_name} is empty"
  fi
done
```

---

## Run the Configuration

```bash
infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
```

Expected ending:

```text
Roles and test users configured successfully.
Realm:            ai-platform
Viewer role:      model-viewer
Deployer role:    model-deployer
Administrator:    platform-admin
Viewer user:      viewer-user
Deployer user:    deployer-user
Admin user:       admin-user
Service account:  service-account-ai-platform-service
```

Clear passwords from the shell:

```bash
unset VIEWER_PASSWORD
unset DEPLOYER_PASSWORD
unset ADMIN_PASSWORD
```

---

## Prove Idempotency

Run the script again:

```bash
set -a
source infrastructure/keycloak/config/authorization.env
source config/platform/keycloak/.secrets/test-users.env
set +a

infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh

unset VIEWER_PASSWORD
unset DEPLOYER_PASSWORD
unset ADMIN_PASSWORD
```

Expected behavior:

```text
Updating role model-viewer...
Updating role model-deployer...
Updating role platform-admin...
model-deployer already includes model-viewer.
platform-admin already includes model-deployer.
viewer-user already has role model-viewer.
deployer-user already has role model-deployer.
admin-user already has role platform-admin.
service-account-ai-platform-service already has role model-deployer.
```

No duplicate roles, users, or assignments should be created.

---

## Authenticate the Keycloak Admin CLI for Validation

Resolve the running Keycloak Pod:

```bash
KEYCLOAK_POD="$(
  kubectl get pod \
    -n keycloak \
    -l app.kubernetes.io/name=keycloak \
    -o jsonpath='{.items[0].metadata.name}'
)"
```

Authenticate:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  sh -ec '
    /opt/keycloak/bin/kcadm.sh config credentials \
      --server http://127.0.0.1:8080 \
      --realm master \
      --user "${KC_BOOTSTRAP_ADMIN_USERNAME}" \
      --password "${KC_BOOTSTRAP_ADMIN_PASSWORD}" \
      --config /tmp/validate-roles-users.config
  '
```

---

## Validate Realm Roles

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get roles \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq '
  map(
    select(
      .name == "model-viewer" or
      .name == "model-deployer" or
      .name == "platform-admin"
    )
  )
  |
  map({
    name,
    description,
    composite
  })
'
```

Expected structure:

```json
[
  {
    "name": "platform-admin",
    "composite": true
  },
  {
    "name": "model-deployer",
    "composite": true
  },
  {
    "name": "model-viewer",
    "composite": false
  }
]
```

The output order may differ.

---

## Validate Composite Role Inheritance

Check `model-deployer`:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    roles/model-deployer/composites/realm \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected:

```json
[
  "model-viewer"
]
```

Check `platform-admin`:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    roles/platform-admin/composites/realm \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected:

```json
[
  "model-deployer"
]
```

Because `model-deployer` is itself composite, `platform-admin` effectively inherits both:

```text
model-deployer
model-viewer
```

---

## Validate Users

List the human test users:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get users \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq '
  map(
    select(
      .username == "viewer-user" or
      .username == "deployer-user" or
      .username == "admin-user"
    )
  )
  |
  map({
    id,
    username,
    email,
    enabled,
    emailVerified
  })
'
```

Expected:

```text
viewer-user:
  enabled=true
  emailVerified=true

deployer-user:
  enabled=true
  emailVerified=true

admin-user:
  enabled=true
  emailVerified=true
```

The service-account user may not always appear in a general user-list query. Resolve it from the client when required.

---

## Helper for Resolving User IDs

```bash
get_keycloak_user_id() {
  local username="$1"

  kubectl exec \
    -n keycloak \
    "${KEYCLOAK_POD}" \
    -- \
    /opt/keycloak/bin/kcadm.sh get users \
      -r ai-platform \
      -q "username=${username}" \
      --config /tmp/validate-roles-users.config |
  jq -r \
    --arg username "${username}" \
    '.[] | select(.username == $username) | .id'
}
```

---

## Validate Direct User Role Assignments

### Viewer

```bash
VIEWER_USER_ID="$(
  get_keycloak_user_id viewer-user
)"
```

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    "users/${VIEWER_USER_ID}/role-mappings/realm" \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected to include:

```text
model-viewer
```

### Deployer

```bash
DEPLOYER_USER_ID="$(
  get_keycloak_user_id deployer-user
)"
```

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    "users/${DEPLOYER_USER_ID}/role-mappings/realm" \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected direct role:

```text
model-deployer
```

### Administrator

```bash
ADMIN_USER_ID="$(
  get_keycloak_user_id admin-user
)"
```

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    "users/${ADMIN_USER_ID}/role-mappings/realm" \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected direct role:

```text
platform-admin
```

### Service account

```bash
SERVICE_ACCOUNT_USER_ID="$(
  get_keycloak_user_id service-account-ai-platform-service
)"
```

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    "users/${SERVICE_ACCOUNT_USER_ID}/role-mappings/realm" \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected direct role:

```text
model-deployer
```

---

## Validate the Keycloak Administration Boundary

The role:

```text
platform-admin
```

is an application role only.

It must not grant Keycloak realm-management roles such as:

```text
realm-admin
manage-users
manage-clients
manage-realm
```

Resolve the `realm-management` client UUID:

```bash
REALM_MANAGEMENT_CLIENT_UUID="$(
  kubectl exec \
    -n keycloak \
    "${KEYCLOAK_POD}" \
    -- \
    /opt/keycloak/bin/kcadm.sh get clients \
      -r ai-platform \
      -q clientId=realm-management \
      --config /tmp/validate-roles-users.config |
  jq -r '.[] | select(.clientId == "realm-management") | .id'
)"
```

Check direct mappings:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    "users/${ADMIN_USER_ID}/role-mappings/clients/${REALM_MANAGEMENT_CLIENT_UUID}" \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected:

```json
[]
```

Check effective mappings:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    "users/${ADMIN_USER_ID}/role-mappings/clients/${REALM_MANAGEMENT_CLIENT_UUID}/composite" \
    -r ai-platform \
    --config /tmp/validate-roles-users.config |
jq 'map(.name)'
```

Expected:

```json
[]
```

---

## Important Endpoint Correction

This endpoint is invalid:

```text
users/{USER_ID}/role-mappings/clients
```

It returns a resource-not-found error.

The correct client-role endpoint requires the internal client UUID:

```text
users/{USER_ID}/role-mappings/clients/{CLIENT_UUID}
```

For effective client-role mappings, append:

```text
/composite
```

Correct form:

```text
users/{USER_ID}/role-mappings/clients/{CLIENT_UUID}/composite
```

---

## Clean Up Temporary Admin Sessions

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  rm -f /tmp/validate-roles-users.config
```

Clear local helper values:

```bash
unset VIEWER_USER_ID
unset DEPLOYER_USER_ID
unset ADMIN_USER_ID
unset SERVICE_ACCOUNT_USER_ID
unset REALM_MANAGEMENT_CLIENT_UUID
unset KEYCLOAK_POD
unset -f get_keycloak_user_id
```

---

## Git Safety Check

Verify that test-user secrets remain ignored:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/test-users.env \
  .local/keycloak/test-users/viewer-user-password \
  .local/keycloak/test-users/deployer-user-password \
  .local/keycloak/test-users/admin-user-password
```

Verify that no secret is staged:

```bash
git diff --cached --name-only |
grep -E \
  'test-users\.env|test-users/.+-password' &&
echo "ERROR: Test-user credentials are staged" ||
echo "PASS: No test-user credentials are staged"
```

Stage only non-secret configuration and automation:

```bash
git add \
  infrastructure/keycloak/config/authorization.env \
  infrastructure/keycloak/variables.env.example \
  infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
```

Review:

```bash
git diff --cached --name-only
```

The secret files must not appear.

---

## Expected Token Role Expansion

Later, when tokens are issued, the expected effective roles are:

### `viewer-user`

```text
model-viewer
```

### `deployer-user`

```text
model-deployer
model-viewer
```

### `admin-user`

```text
platform-admin
model-deployer
model-viewer
```

### `service-account-ai-platform-service`

```text
model-deployer
model-viewer
```

The inherited roles are included because the higher-level roles are composite.

---

## Troubleshooting

### `401 Unauthorized` from `kcadm`

Possible cause:

```text
stale or invalid Admin CLI session
```

Corrective actions:

```bash
rm -f /tmp/ai-platform-roles-users-kcadm.config
```

Then authenticate again.

The automation script retries after:

```text
401 Unauthorized
Invalid token
Not authenticated
```

---

### Realm does not exist

Symptom:

```text
ERROR: Realm ai-platform does not exist.
```

Corrective action:

```bash
infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

Run the realm/client configuration first.

---

### Service-account user not found

Check the client:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get clients \
    -r ai-platform \
    -q clientId=ai-platform-service \
    --config /tmp/validate-roles-users.config
```

Confirm:

```text
serviceAccountsEnabled=true
```

Then inspect:

```text
clients/{CLIENT_UUID}/service-account-user
```

---

### Composite roles are not expanded in tokens

Check:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get \
    roles/model-deployer/composites/realm \
    -r ai-platform \
    --config /tmp/validate-roles-users.config
```

Also verify the client role scope and audience mapper configuration.

---

### User exists but cannot log in

Check:

```text
enabled=true
temporary password=false
requiredActions=[]
```

Reset the password:

```bash
/opt/keycloak/bin/kcadm.sh set-password \
  --realm ai-platform \
  --userid USER_ID \
  --new-password NEW_PASSWORD \
  --temporary=false
```

---

## Completion Criteria

```text
[✓] model-viewer realm role created
[✓] model-deployer realm role created
[✓] platform-admin realm role created
[✓] model-deployer inherits model-viewer
[✓] platform-admin inherits model-deployer
[✓] viewer-user created and enabled
[✓] deployer-user created and enabled
[✓] admin-user created and enabled
[✓] viewer-user assigned model-viewer
[✓] deployer-user assigned model-deployer
[✓] admin-user assigned platform-admin
[✓] service-account-ai-platform-service assigned model-deployer
[✓] no Keycloak realm-management privileges assigned
[✓] test-user passwords excluded from Git
[✓] script runs idempotently
[✓] roles and user definitions stored as code
```

---

## Resulting Identity Model

```text
Human users
├── viewer-user
│   └── model-viewer
├── deployer-user
│   └── model-deployer
│       └── model-viewer
└── admin-user
    └── platform-admin
        └── model-deployer
            └── model-viewer

Machine identity
└── service-account-ai-platform-service
    └── model-deployer
        └── model-viewer
```

This identity model is later consumed by Envoy Gateway authorization rules through the JWT claim:

```text
realm_access.roles
```
