# Git Safety and Secret Management

## Purpose

This document defines which OIDC, Keycloak, Vault, certificate, and token artifacts may be committed and which must remain local.

The main rule is:

```text
Git stores configuration and automation.
Git does not store credentials, private keys, tokens, or generated secrets.
```

---

## Safe to Commit

```text
Kubernetes manifests
Kustomization files
Keycloak configuration scripts
realm and client definitions
role definitions
validation scripts
Vault policy templates
CRD changes
controller code
RBAC manifests
example environment files with placeholders
documentation
```

---

## Never Commit

```text
client secrets
test-user passwords
bootstrap administrator passwords
PostgreSQL passwords
access tokens
refresh tokens
JWT files
token endpoint response JSON
private keys
TLS key material
local CA copies when treated as local artifacts
generated .local files
real secret environment files
```

---

## Protected Paths

```text
config/platform/keycloak/.secrets/
.local/keycloak/
*.jwt
*token-response*.json
*password*
*.key
```

The exact `.gitignore` rules should be reviewed to avoid hiding legitimate non-secret files too broadly.

---

## Recommended `.gitignore`

```gitignore
# Local Keycloak and OIDC material
.local/keycloak/

# Real Keycloak secret inputs
config/platform/keycloak/.secrets/

# Tokens
*.jwt
*token-response*.json

# Private keys
*.key
*.pem

# Python cache
__pycache__/
*.pyc
```

Avoid ignoring all `.crt` files globally when repository-managed public CA certificates are intentionally versioned.

---

## Verify Ignored Files

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/postgres.env \
  config/platform/keycloak/.secrets/bootstrap-admin.env \
  config/platform/keycloak/.secrets/service-client.env \
  config/platform/keycloak/.secrets/test-users.env \
  .local/keycloak/tokens/service-access-token.jwt \
  .local/keycloak/tokens/viewer-access-token.jwt \
  .local/keycloak/tokens/admin-access-token.jwt
```

Each path should show the matching `.gitignore` rule.

---

## Confirm Sensitive Files Are Not Tracked

```bash
git ls-files |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/keycloak/|\.jwt$|token-response|password|private.*key'
```

Expected:

```text
no output
```

A guarded check:

```bash
git ls-files |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/keycloak/|\.jwt$|token-response|password|private.*key' &&
{
  echo "ERROR: Sensitive material is tracked"
  exit 1
} || echo "PASS: No sensitive material is tracked"
```

---

## Review the Working Tree

```bash
git status --short
```

```bash
git diff --name-only
```

```bash
git diff
```

Review all untracked files before using broad staging commands.

Avoid:

```bash
git add .
```

Prefer explicit staging.

---

## Explicit Staging

```bash
git add \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go \
  config/crd/bases/platform.anselem.dev_modelservices.yaml \
  config/rbac/role.yaml \
  config/platform/authentication \
  config/platform/keycloak \
  config/platform/shared-gateway.yaml \
  config/samples/platform_v1alpha1_modelservice.yaml \
  infrastructure/keycloak \
  docs/oidc-jwt \
  .gitignore
```

Ignored secret files will not be staged, but explicit review remains required.

---

## Inspect Staged Files

```bash
git diff --cached --name-status
```

```bash
git diff --cached --stat
```

```bash
git diff --cached
```

Look for:

```text
password values
client secrets
access tokens
refresh tokens
private keys
unexpected base64 blobs
generated local files
```

---

## Secret Pattern Scan

```bash
git diff --cached |
grep -Ei \
  'access_token|refresh_token|client_secret|password[=:]|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY'
```

A guarded check:

```bash
git diff --cached |
grep -Ei \
  'access_token|refresh_token|client_secret[=:][[:space:]]*[^$<]|password[=:][[:space:]]*[^$<]|BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY' &&
{
  echo "WARNING: Review possible credential material above"
  exit 1
} || echo "PASS: No obvious staged credential values found"
```

This is a safety check, not a complete secret scanner.

---

## Check Staged Paths

```bash
git diff --cached --name-only |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/|\.jwt$|token-response|password$|private.*key'
```

Expected:

```text
no output
```

---

## Environment Examples

Safe example file:

```text
infrastructure/keycloak/variables.env.example
```

Use placeholders:

```env
POSTGRES_PASSWORD=replace-with-generated-password
KC_BOOTSTRAP_ADMIN_PASSWORD=replace-with-generated-password
CLIENT_SECRET=replace-with-generated-client-secret
VIEWER_PASSWORD=replace-with-generated-password
```

Do not copy real values into example files.

---

## Kubernetes Secret Manifests

Avoid committing plaintext `Secret` resources containing real values.

Prefer:

```text
Kustomize secretGenerator with ignored env files
External Secrets Operator
Vault integration
Sealed Secrets
SOPS-encrypted files
```

For this implementation, local secret env files are ignored and used to generate Kubernetes Secrets.

---

## Token Handling

Store tokens only under:

```text
.local/keycloak/tokens/
```

Restrict permissions:

```bash
chmod 700 .local/keycloak/tokens
chmod 600 .local/keycloak/tokens/*
```

Delete expired tokens when they are no longer needed:

```bash
find .local/keycloak/tokens \
  -type f \
  -name '*.jwt' \
  -delete
```

---

## Private Key Handling

Private TLS keys belong in Kubernetes Secrets or Vault-managed issuance workflows.

Never export `tls.key` into the repository.

When temporary local inspection is required:

```bash
umask 077
```

Delete the file afterward.

---

## Before Commit

Run:

```bash
find infrastructure/keycloak/scripts \
  -type f \
  -name '*.sh' \
  -print0 |
while IFS= read -r -d '' script; do
  bash -n "${script}"
done
```

```bash
find infrastructure/keycloak/scripts \
  -type f \
  -name '*.py' \
  -print0 |
while IFS= read -r -d '' script; do
  python3 -m py_compile "${script}"
done
```

Remove caches:

```bash
find infrastructure/keycloak \
  -type d \
  -name '__pycache__' \
  -prune \
  -exec rm -rf {} +
```

Run Go checks:

```bash
gofmt -w \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go
```

```bash
make generate
make manifests
make test
```

Restage generated changes.

---

## Commit

```bash
git commit \
  -m "feat: add Keycloak OIDC and gateway authorization" \
  -m "Deploy Keycloak with PostgreSQL and Vault-issued TLS." \
  -m "Add realm, clients, roles, users, PKCE and service-token automation." \
  -m "Protect ModelService routes with Envoy JWT validation and role authorization." \
  -m "Restrict workload and operator Kubernetes permissions."
```

---

## Verify the Commit

```bash
git show \
  --stat \
  --oneline \
  HEAD
```

```bash
git show \
  --name-only \
  --format='' \
  HEAD |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/|\.jwt$|token-response|password$|private.*key'
```

Expected:

```text
no output
```

---

## If a Secret Was Committed

Do not only delete the file in a later commit.

The secret remains in Git history.

Immediate actions:

```text
revoke or rotate the secret
remove it from the working tree
remove it from Git history
force-push only after coordinating with collaborators
notify affected users
```

Examples of credentials that must be rotated:

```text
Keycloak client secret
test-user password
bootstrap admin password
PostgreSQL password
Vault token
private key
access token
refresh token
```

---

## Completion Criteria

```text
[✓] real secret files ignored
[✓] local tokens ignored
[✓] private keys ignored
[✓] example files use placeholders
[✓] no sensitive file tracked
[✓] no sensitive file staged
[✓] staged content reviewed
[✓] scripts validated
[✓] generated files refreshed
[✓] final commit inspected
[✓] secret rotation procedure documented
```
