# AI Platform REST API — Authentication and Authorization

## 1. Purpose

This document describes the authentication and authorization model of the AI Platform REST API.

It covers:

- Keycloak OIDC integration
- JWT validation
- issuer and audience
- interactive and machine clients
- viewer/deployer/admin roles
- application-level authorization
- Envoy SecurityPolicy enforcement
- defense in depth
- edge vs application denial behavior
- token handling
- validation flows used during implementation

---

## 2. Authentication Architecture

The external flow is:

```text
Client
   |
   | authenticate
   v
Keycloak
   |
   | JWT access token
   v
Client
   |
   | HTTPS + Authorization: Bearer
   v
Envoy Gateway
   |
   | SecurityPolicy
   v
AI Platform REST API
```

The API uses OIDC/JWT rather than Kubernetes credentials for user authentication.

---

## 3. Keycloak Realm

Realm:

```text
ai-platform
```

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Expected audience:

```text
ai-platform-gateway
```

Internal JWKS endpoint:

```text
http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
```

---

## 4. Token Validation

The API validates JWT access tokens using OIDC.

The validation model includes:

```text
signature
issuer
audience
expiration
subject
role claims
```

A caller-provided username or role header is not trusted.

Identity must come from the validated token.

---

## 5. Authentication Package

Authentication code lives under:

```text
internal/api/auth/
```

Responsibilities include:

```text
OIDC verifier construction
token validation
identity extraction
role extraction
request-context identity propagation
```

---

## 6. Keycloak Clients

The platform uses three relevant clients.

### `ai-platform-gateway`

Purpose:

```text
protected resource / API audience
```

### `ai-platform-cli`

Purpose:

```text
interactive users
```

Flow:

```text
Authorization Code + PKCE S256
```

### `ai-platform-service`

Purpose:

```text
machine-to-machine service access
```

---

## 7. Interactive Authentication

Interactive users authenticate with:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Callback:

```text
127.0.0.1:18080
```

The user token is stored under:

```text
.local/keycloak/tokens/user-access-token.jwt
```

This was used for admin-level API validation.

---

## 8. Machine Authentication

Machine token helper:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Output:

```text
.local/keycloak/tokens/service-access-token.jwt
```

Load:

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

Example:

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services
```

Then:

```bash
unset TOKEN
```

---

## 9. Token Lifetime

Access tokens used during validation are short lived.

Observed lifetime:

```text
approximately 300 seconds
```

A fresh token should be obtained before functional tests.

Expired tokens should be treated as an authentication issue, not an API availability issue.

---

## 10. Role Model

Logical platform roles:

```text
platform-viewer
platform-deployer
platform-admin
```

These roles map to different API permissions.

---

## 11. Authorization Matrix

| Operation | Viewer | Deployer | Admin |
|---|---:|---:|---:|
| List | Yes | Yes | Yes |
| Get | Yes | Yes | Yes |
| Status | Yes | Yes | Yes |
| Create | No | Yes | Yes |
| Update | No | Yes | Yes |
| Patch | No | Yes | Yes |
| Delete | No | No | Yes |

Deletion is intentionally admin-only.

---

## 12. Application Authorization

The Go API performs its own role checks.

Protected request flow:

```text
Authentication
   |
   v
AuditLogging
   |
   v
Protected router
   |
   v
RequireAnyRole
   |
   v
Handler
```

This ensures the handler only runs after the application authorization decision succeeds.

---

## 13. Envoy Authorization

The external API route is protected by Envoy `SecurityPolicy`.

The edge performs:

```text
JWT validation
role-based authorization
```

before forwarding allowed traffic to the API.

This means authorization exists at:

```text
Envoy edge
+
Go application
```

---

## 14. Defense in Depth

The complete authorization chain is:

```text
Keycloak JWT
    |
    v
Envoy SecurityPolicy
    |
    v
API JWT validation
    |
    v
API role authorization
    |
    v
fixed namespace
    |
    v
Kubernetes ServiceAccount RBAC
```

No single layer is treated as the only security boundary.

---

## 15. Why Both Envoy and API Authorization Exist

Envoy protects the API before traffic reaches the application.

Application authorization ensures security does not depend solely on edge configuration.

This protects against:

```text
route/policy mistakes
internal direct access paths
future topology changes
configuration drift
```

---

## 16. Edge Rejection vs API Rejection

A critical operational distinction:

### Rejected by Envoy

```text
Client -> Envoy -> 401/403
```

The Go API never sees the request.

Therefore:

```text
no Go handler
no application mutation audit event
no application-side request processing
```

### Rejected by API

```text
Client -> Envoy -> API -> 401/403
```

The API can observe and log the outcome.

---

## 17. Observed DELETE Authorization Behavior

A machine/deployer token attempted DELETE externally.

Result:

```text
HTTP 403
```

The denial occurred at Envoy.

Because the request never reached the API:

```text
no Go audit event was expected
```

This behavior was explicitly validated.

---

## 18. Admin DELETE Validation

Interactive admin authentication was performed through PKCE.

Admin DELETE returned:

```text
204 No Content
```

The application audit event contained:

```text
method=DELETE
route=/api/v1/model-services/{name}
resource_name=audit-probe
status=204
outcome=success
username=admin-user
```

This proved the admin authorization path works.

---

## 19. Authentication Failure

Expected class:

```text
401 Unauthorized
```

Examples:

```text
missing token
invalid signature
wrong issuer
wrong audience
expired token
```

Authentication failure should happen before resource mutation.

---

## 20. Authorization Failure

Expected class:

```text
403 Forbidden
```

Examples:

```text
viewer attempts POST
deployer attempts DELETE
valid identity lacks required platform role
```

---

## 21. Identity Data

The authenticated identity can include:

```text
subject
username
roles
```

These fields are available to audit logging after authentication succeeds.

JWT values themselves are never logged.

---

## 22. Audit Identity Fields

Mutation audit events can include:

```text
subject
username
roles
```

Example service identity:

```text
service-account-ai-platform-service
```

Example interactive identity:

```text
admin-user
```

---

## 23. Role Composition

Higher-privileged roles are designed to include lower-level capabilities.

Conceptually:

```text
viewer
  < deployer
  < admin
```

The effective API role matrix reflects this hierarchy.

---

## 24. Authentication Middleware Responsibility

Authentication middleware:

```text
extracts bearer token
validates token
builds identity
stores identity in request context
```

It should not perform handler-specific authorization decisions.

---

## 25. Authorization Middleware Responsibility

Authorization middleware:

```text
reads authenticated identity
checks required role(s)
allows or denies request
```

This keeps authentication and authorization separate.

---

## 26. Why Separation Matters

Separating:

```text
Who are you?
```

from:

```text
What are you allowed to do?
```

improves:

```text
testability
clarity
policy review
future role expansion
auditability
```

---

## 27. Public Operational Endpoints

At the Go application layer:

```text
/healthz
/readyz
/metrics
```

do not use the same application authentication chain as `/api/v1/`.

However, external Envoy policy may still protect a broader route.

Prometheus bypasses external edge auth by scraping the internal Service directly.

---

## 28. TLS and Authentication

Authentication is always used over HTTPS externally.

External hostname:

```text
https://api.ai-platform.local
```

TLS is terminated by Envoy Gateway.

The API container itself receives internal HTTP after edge termination.

---

## 29. API Audience

Expected audience:

```text
ai-platform-gateway
```

The audience check prevents a token issued for an unrelated resource from being accepted simply because it was signed by the same Keycloak realm.

---

## 30. Issuer Validation

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Issuer validation prevents tokens from unrelated identity providers or realms from being accepted.

---

## 31. JWKS Validation

The API obtains signing-key information through OIDC/JWKS.

Internal endpoint:

```text
http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
```

This requires startup network access.

---

## 32. OIDC CA Trust

The API mounts CA trust through:

```text
ConfigMap/ai-platform-api-oidc-ca
```

Incorrect CA configuration can cause OIDC initialization failure.

---

## 33. NetworkPolicy Dependency

OIDC discovery was one of the most important NetworkPolicy lessons from this phase.

The hardened policy initially blocked required OIDC connectivity.

The fix allowed the actual Envoy proxy target path seen by the CNI.

This means auth availability depends on:

```text
DNS
CA trust
NetworkPolicy egress
OIDC endpoint reachability
```

---

## 34. Token Storage

Local development/test tokens are written under:

```text
.local/keycloak/tokens/
```

This directory is runtime/test state and should not be committed.

Documentation must never contain live token values.

---

## 35. Token Logging Policy

The API does not log:

```text
Authorization header
JWT access token
refresh token
```

This is a deliberate security requirement.

---

## 36. Prometheus Label Policy

Identity data is also excluded from Prometheus labels.

Do not use:

```text
username
subject
roles
request_id
resource_name
```

as metric labels.

This protects both privacy and metric cardinality.

---

## 37. Authentication Test — Machine Token

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Then:

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

Read request:

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services
```

---

## 38. Authorization Test — Deployer Delete

Using a deployer/machine identity:

```text
DELETE /api/v1/model-services/{name}
```

should be denied.

Observed external behavior:

```text
403
```

at Envoy.

---

## 39. Authorization Test — Admin Delete

Obtain admin token:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Load:

```bash
ADMIN_TOKEN="$(
  cat .local/keycloak/tokens/user-access-token.jwt
)"
```

The admin path was validated successfully for DELETE.

---

## 40. Application Authorization Test Coverage

The API has tests around:

```text
authentication
role checks
protected routes
authorization outcomes
```

The exact current test functions live in the Go source tree and remain the source of truth.

---

## 41. External vs Internal Trust Boundary

External callers trust:

```text
Vault-issued TLS certificate
Envoy Gateway endpoint
Keycloak authentication
```

The API internally trusts:

```text
Keycloak/OIDC
Kubernetes ServiceAccount credentials
mounted CA material
```

These are separate trust boundaries.

---

## 42. Security Failure Classification

When a request fails, identify which layer failed:

```text
TLS
Envoy JWT
Envoy role policy
API JWT validation
API role authorization
Kubernetes RBAC
```

Do not treat every `401` or `403` as the same problem.

---

## 43. Example Failure Matrix

| Symptom | Likely Layer |
|---|---|
| TLS handshake failure | certificate/CA/Gateway |
| External 401 before app logs | Envoy JWT |
| External 403 before app logs | Envoy role policy |
| API-side 401 | application authentication |
| API-side 403 | application authorization |
| Backend Kubernetes forbidden | ServiceAccount RBAC |

---

## 44. Role-Based Access Intent

The roles were designed around platform responsibilities.

### Viewer

```text
observe resources and state
```

### Deployer

```text
create and modify deployments
```

### Admin

```text
full lifecycle including deletion
```

This keeps destructive actions more restricted.

---

## 45. Why DELETE Is Admin-Only

Delete has higher operational risk than create/update/patch.

Restricting it to admin reduces accidental destructive actions and gives the authorization model a meaningful privilege boundary.

---

## 46. Current Authentication/Authorization Status

```text
[✓] Keycloak realm integrated
[✓] OIDC issuer configured
[✓] audience validation configured
[✓] JWT validation implemented
[✓] machine client implemented
[✓] interactive PKCE flow implemented
[✓] viewer role implemented
[✓] deployer role implemented
[✓] admin role implemented
[✓] Envoy SecurityPolicy attached
[✓] API authorization middleware implemented
[✓] deployer DELETE denial validated
[✓] admin DELETE success validated
[✓] identity captured in audit events
[✓] token values excluded from logs
```

Authentication and authorization are implementation-complete.

---

## 47. Summary

The AI Platform REST API uses a layered security model:

```text
Keycloak
   |
   v
JWT access token
   |
   v
Envoy SecurityPolicy
   |
   v
Go OIDC authentication
   |
   v
Go role authorization
   |
   v
fixed namespace
   |
   v
Kubernetes RBAC
```

The design distinguishes authentication from authorization, separates edge and application enforcement, keeps destructive DELETE admin-only, and preserves identity for audit logging without exposing token values.
