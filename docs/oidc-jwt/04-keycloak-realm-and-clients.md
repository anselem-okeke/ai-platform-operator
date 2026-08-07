# Keycloak Realm and Clients

## Objective

Configure a dedicated Keycloak realm for the AI Platform and create the OpenID Connect clients required for:

- validating JWTs at Envoy Gateway;
- interactive user authentication through Authorization Code with PKCE;
- machine-to-machine authentication through Client Credentials.

The final identity layout is:

```text
Realm: ai-platform

Clients:
├── ai-platform-gateway   JWT audience and protected resource
├── ai-platform-cli       Public client for human users
└── ai-platform-service   Confidential client for machine access
```

The external issuer is:

```text
https://auth.ai-platform.local/realms/ai-platform
```

---

## Architecture

```text
Human user
  ↓
ai-platform-cli
  ↓ Authorization Code + PKCE
Keycloak realm: ai-platform
  ↓
JWT access token
  ↓ aud=ai-platform-gateway
Envoy Gateway

Machine workload
  ↓
ai-platform-service
  ↓ Client Credentials
Keycloak realm: ai-platform
  ↓
JWT access token
  ↓ aud=ai-platform-gateway
Envoy Gateway
```

### Why a separate realm is used

The Keycloak `master` realm is reserved for Keycloak administration. AI Platform users, clients, roles, and tokens are placed in a dedicated realm:

```text
master
  └── Keycloak administration

ai-platform
  └── AI Platform authentication and authorization
```

This prevents application identities from being mixed with Keycloak administrative identities.

---

## Final configuration

| Item | Value |
|---|---|
| Realm | `ai-platform` |
| Display name | `AI Platform` |
| External Keycloak URL | `https://auth.ai-platform.local` |
| OIDC issuer | `https://auth.ai-platform.local/realms/ai-platform` |
| Resource-server audience | `ai-platform-gateway` |
| Human client | `ai-platform-cli` |
| Machine client | `ai-platform-service` |
| Public-client flow | Authorization Code + PKCE |
| Machine flow | Client Credentials |
| Direct password grant | Disabled |

---

## Prerequisites

Before continuing, verify that Keycloak is reachable through trusted HTTPS.

```bash
cd /mnt/data/ai-platform-operator

GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"

curl \
  --silent \
  --show-error \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/realms/master/.well-known/openid-configuration |
jq '{issuer, authorization_endpoint, token_endpoint, jwks_uri}'
```

Expected issuer:

```text
https://auth.ai-platform.local/realms/master
```

Also verify the Keycloak workload:

```bash
kubectl rollout status deployment/keycloak \
  -n keycloak \
  --timeout=180s

kubectl rollout status statefulset/keycloak-postgres \
  -n keycloak \
  --timeout=180s
```

---

## Files created or modified

```text
config/platform/keycloak/
├── kustomization.yaml
└── .secrets/
    └── service-client.env          # local only, ignored by Git

infrastructure/keycloak/
├── config/
│   └── realm.env
├── scripts/
│   └── configure-keycloak-realm-clients.sh
└── variables.env.example

.local/keycloak/
└── ai-platform-service-client-secret  # local only, ignored by Git
```

---

## Create the machine-client secret

Generate a strong client secret:

```bash
AI_PLATFORM_SERVICE_CLIENT_SECRET="$(
  openssl rand -base64 48 |
  tr -d '\n'
)"
```

Create the local Kustomize input:

```bash
mkdir -p config/platform/keycloak/.secrets

cat > config/platform/keycloak/.secrets/service-client.env <<EOF
CLIENT_ID=ai-platform-service
CLIENT_SECRET=${AI_PLATFORM_SERVICE_CLIENT_SECRET}
EOF

chmod 600 \
  config/platform/keycloak/.secrets/service-client.env
```

Store a restricted local copy for later token tests:

```bash
mkdir -p .local/keycloak
chmod 700 .local/keycloak

printf '%s\n' "${AI_PLATFORM_SERVICE_CLIENT_SECRET}" \
  > .local/keycloak/ai-platform-service-client-secret

chmod 600 \
  .local/keycloak/ai-platform-service-client-secret

unset AI_PLATFORM_SERVICE_CLIENT_SECRET
```

Confirm both files are ignored:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/service-client.env \
  .local/keycloak/ai-platform-service-client-secret
```

Expected: both paths are matched by `.gitignore`.

Confirm neither file is tracked:

```bash
git ls-files \
  config/platform/keycloak/.secrets/service-client.env \
  .local/keycloak/ai-platform-service-client-secret
```

Expected: no output.

---

## Update the example environment file

Use this non-secret example:

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
EOF
```

---

## Add the generated Secret to Kustomize

Update `config/platform/keycloak/kustomization.yaml` so the secret generators include the service client:

```yaml
secretGenerator:
  - name: keycloak-postgres-credentials
    namespace: keycloak
    envs:
      - .secrets/postgres.env

  - name: keycloak-bootstrap-admin
    namespace: keycloak
    envs:
      - .secrets/bootstrap-admin.env

  - name: ai-platform-service-client-credentials
    namespace: keycloak
    envs:
      - .secrets/service-client.env

generatorOptions:
  disableNameSuffixHash: true
  labels:
    app.kubernetes.io/part-of: ai-platform
    app.kubernetes.io/managed-by: kustomize
```

Apply the Keycloak Kustomization:

```bash
kubectl apply -k config/platform/keycloak
```

Verify only Secret metadata:

```bash
kubectl get secret \
  ai-platform-service-client-credentials \
  -n keycloak
```

Expected:

```text
NAME                                     TYPE     DATA
ai-platform-service-client-credentials   Opaque   2
```

Do not print the Secret values.

---

## Create the non-secret realm configuration

Create `infrastructure/keycloak/config/realm.env`:

```bash
mkdir -p infrastructure/keycloak/config

cat > infrastructure/keycloak/config/realm.env <<'EOF'
KEYCLOAK_REALM=ai-platform
KEYCLOAK_DISPLAY_NAME=AI Platform
KEYCLOAK_EXTERNAL_URL=https://auth.ai-platform.local
EOF
```

This file contains no secret and is safe to commit.

---

## Realm configuration

The realm is configured with the following settings:

```text
realm: ai-platform
displayName: AI Platform
enabled: true
registrationAllowed: false
resetPasswordAllowed: true
rememberMe: true
loginWithEmailAllowed: true
duplicateEmailsAllowed: false
verifyEmail: false
sslRequired: external
```

`sslRequired=external` permits the internal Keycloak Admin CLI to connect through loopback HTTP while external browser and token traffic uses HTTPS through Envoy Gateway.

---

## Client configuration

### `ai-platform-gateway`

Purpose:

```text
Protected resource and expected JWT audience
```

Configuration:

```text
clientId: ai-platform-gateway
bearerOnly: true
publicClient: false
standardFlowEnabled: false
implicitFlowEnabled: false
directAccessGrantsEnabled: false
serviceAccountsEnabled: false
```

This client does not log users in and does not obtain tokens. It represents the resource server that Envoy Gateway protects.

### `ai-platform-cli`

Purpose:

```text
Interactive human login
```

Configuration:

```text
clientId: ai-platform-cli
publicClient: true
bearerOnly: false
standardFlowEnabled: true
implicitFlowEnabled: false
directAccessGrantsEnabled: false
serviceAccountsEnabled: false
PKCE method: S256
```

Redirect URIs:

```text
http://127.0.0.1:18080/*
http://localhost:18080/*
```

Web origins:

```text
http://127.0.0.1:18080
http://localhost:18080
```

This client is public because it cannot safely store a client secret on a user workstation. Authorization Code with PKCE protects the authorization-code exchange.

### `ai-platform-service`

Purpose:

```text
Machine-to-machine authentication
```

Configuration:

```text
clientId: ai-platform-service
publicClient: false
bearerOnly: false
clientAuthenticatorType: client-secret
standardFlowEnabled: false
implicitFlowEnabled: false
directAccessGrantsEnabled: false
serviceAccountsEnabled: true
authorizationServicesEnabled: false
```

This client obtains tokens through the OAuth 2.0 Client Credentials flow.

---

## Audience mapper

Both token-producing clients receive an OIDC audience mapper:

```text
Mapper name:
  audience-ai-platform-gateway

Included client audience:
  ai-platform-gateway

Add to access token:
  true

Add to ID token:
  false
```

This ensures access tokens contain:

```json
{
  "aud": "ai-platform-gateway"
}
```

or an audience array containing the same value.

Envoy Gateway later validates this audience before forwarding traffic to the protected model route.

---

## Idempotent configuration script

The implementation uses:

```text
infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

The script performs the following actions:

```text
1. Resolves the running Keycloak Pod.
2. Reads the service-client secret from the Kubernetes Secret.
3. Authenticates kcadm against the master realm.
4. Creates or updates the ai-platform realm.
5. Re-authenticates after changing realm configuration.
6. Creates or updates ai-platform-gateway.
7. Creates or updates ai-platform-cli.
8. Creates or updates ai-platform-service.
9. Creates or updates audience mappers.
10. Removes the temporary kcadm session file.
```

The script is designed to be run repeatedly without creating duplicate resources.

Make it executable and validate its syntax:

```bash
chmod +x \
  infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh

bash -n \
  infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

Load the non-secret configuration:

```bash
set -a
source infrastructure/keycloak/config/realm.env
set +a
```

Run the configuration:

```bash
infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

Expected ending:

```text
Realm and clients configured successfully.
Realm:            ai-platform
Audience client:  ai-platform-gateway
User client:      ai-platform-cli
Service client:   ai-platform-service
External issuer:  https://auth.ai-platform.local/realms/ai-platform
```

Run the script a second time to prove idempotency:

```bash
infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

The second run should update or confirm existing objects rather than create duplicates.

---

## Validate the realm through HTTPS

Resolve the Gateway IP:

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

Query OIDC discovery:

```bash
curl \
  --silent \
  --show-error \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/realms/ai-platform/.well-known/openid-configuration |
jq '{
  issuer,
  authorization_endpoint,
  token_endpoint,
  jwks_uri
}'
```

Expected:

```json
{
  "issuer": "https://auth.ai-platform.local/realms/ai-platform",
  "authorization_endpoint": "https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/auth",
  "token_endpoint": "https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/token",
  "jwks_uri": "https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs"
}
```

All published endpoints must use the external HTTPS hostname.

---

## Validate the clients with `kcadm`

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
      --config /tmp/validate-kcadm.config
  '
```

List the required clients:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh \
    --config /tmp/validate-kcadm.config \
    get clients \
    --realm ai-platform |
jq '
  map(
    select(
      .clientId == "ai-platform-gateway" or
      .clientId == "ai-platform-cli" or
      .clientId == "ai-platform-service"
    )
  )
  |
  map({
    clientId,
    enabled,
    publicClient,
    bearerOnly,
    standardFlowEnabled,
    directAccessGrantsEnabled,
    serviceAccountsEnabled
  })
'
```

Expected properties:

```text
ai-platform-gateway
  enabled=true
  bearerOnly=true
  publicClient=false
  standardFlowEnabled=false
  directAccessGrantsEnabled=false
  serviceAccountsEnabled=false

ai-platform-cli
  enabled=true
  publicClient=true
  bearerOnly=false
  standardFlowEnabled=true
  directAccessGrantsEnabled=false
  serviceAccountsEnabled=false

ai-platform-service
  enabled=true
  publicClient=false
  bearerOnly=false
  standardFlowEnabled=false
  directAccessGrantsEnabled=false
  serviceAccountsEnabled=true
```

Remove the temporary Admin CLI session:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  rm -f /tmp/validate-kcadm.config

unset KEYCLOAK_POD
```

---

## Troubleshooting

### `HTTP 401 Unauthorized` during client configuration

Observed symptom:

```text
Updating realm ai-platform...
Configuring resource-server audience client...
HTTP 401 Unauthorized
```

Likely cause:

```text
The kcadm admin session was stale, invalid, or not reused correctly after realm changes.
```

Correction:

```text
- authenticate through a dedicated login function;
- remove stale kcadm configuration before login;
- re-authenticate after creating or updating the realm;
- retry once when an Admin API command returns 401;
- keep the temporary kcadm config inside the Keycloak Pod.
```

### Realm discovery works but clients are missing

Check all client IDs:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get clients \
    -r ai-platform \
    --config /tmp/validate-kcadm.config |
jq 'map(.clientId)'
```

Rerun the configuration script when the realm exists but one or more required clients are absent.

### OIDC discovery publishes HTTP or the wrong hostname

Required Keycloak environment values:

```text
KC_HOSTNAME=https://auth.ai-platform.local
KC_HTTP_ENABLED=true
KC_PROXY_HEADERS=xforwarded
```

Also confirm that the request passes through the `keycloak-https` Gateway listener.

### Duplicate clients appear

The script must resolve each client by exact `clientId` and update its internal UUID.

Check for duplicates:

```bash
kubectl exec \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -- \
  /opt/keycloak/bin/kcadm.sh get clients \
    -r ai-platform \
    --config /tmp/validate-kcadm.config |
jq '
  group_by(.clientId)
  | map(select(length > 1))
  | map({clientId: .[0].clientId, count: length})
'
```

Expected:

```json
[]
```

---

## Git safety

Confirm the service-client credential files are ignored:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/service-client.env \
  .local/keycloak/ai-platform-service-client-secret
```

Confirm they are not tracked:

```bash
git ls-files \
  config/platform/keycloak/.secrets/service-client.env \
  .local/keycloak/ai-platform-service-client-secret
```

Expected: no output.

Stage only non-secret implementation files:

```bash
git add \
  config/platform/keycloak/kustomization.yaml \
  infrastructure/keycloak/config/realm.env \
  infrastructure/keycloak/variables.env.example \
  infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

The following must not be staged:

```text
config/platform/keycloak/.secrets/service-client.env
.local/keycloak/ai-platform-service-client-secret
```

---

## Completion criteria

```text
[✓] ai-platform realm exists
[✓] Realm is enabled
[✓] External issuer uses trusted HTTPS
[✓] OIDC discovery publishes the correct external endpoints
[✓] ai-platform-gateway resource client exists
[✓] ai-platform-cli public client exists
[✓] Authorization Code flow is enabled for ai-platform-cli
[✓] PKCE S256 is configured
[✓] Direct password grant is disabled
[✓] ai-platform-service confidential client exists
[✓] Service accounts are enabled for ai-platform-service
[✓] Service-client secret is excluded from Git
[✓] Audience mapper is attached to ai-platform-cli
[✓] Audience mapper is attached to ai-platform-service
[✓] Access tokens contain audience ai-platform-gateway
[✓] Configuration script is idempotent
[✓] Realm and client configuration is stored as code
```

---

## Result

The AI Platform now has a dedicated OIDC realm and three clearly separated clients:

```text
ai-platform-gateway
  → protected resource and JWT audience

ai-platform-cli
  → interactive users through Authorization Code + PKCE

ai-platform-service
  → machine clients through Client Credentials
```

The next configuration layer adds application roles, test users, composite-role inheritance, and the service-account role assignment.
