# AI Platform REST API — Create, Update, Patch, and Delete

## 1. Purpose

This document describes the mutation API of the AI Platform REST API.

It covers:

- `POST`
- `PUT`
- `PATCH`
- `DELETE`
- authorization requirements
- validation
- Kubernetes mutation behavior
- audit logging
- operator reconciliation
- HTTP response semantics
- end-to-end validation

---

## 2. Mutation Endpoint Summary

| Method | Path | Purpose | Required Role |
|---|---|---|---|
| `POST` | `/api/v1/model-services` | Create | deployer/admin |
| `PUT` | `/api/v1/model-services/{name}` | Update/replace | deployer/admin |
| `PATCH` | `/api/v1/model-services/{name}` | Partial update | deployer/admin |
| `DELETE` | `/api/v1/model-services/{name}` | Delete | admin |

All mutations operate only in:

```text
ai-platform
```

---

## 3. Mutation Request Pipeline

Every protected mutation follows the same high-level path:

```text
Client
  |
  v
Envoy JWT/role policy
  |
  v
API authentication
  |
  v
AuditLogging
  |
  v
API role authorization
  |
  v
request validation
  |
  v
Kubernetes mutation
  |
  v
HTTP response
```

The operator then reconciles the changed `ModelService`.

---

## 4. Create ModelService

Endpoint:

```http
POST /api/v1/model-services
```

Required role:

```text
platform-deployer
or
platform-admin
```

Expected success:

```text
201 Created
```

---

## 5. Create Behavior

The create path:

```text
POST body
  |
  v
validate
  |
  v
construct ModelService
  |
  | namespace forced to ai-platform
  v
Kubernetes create
  |
  v
201
```

The caller cannot choose an arbitrary namespace.

---

## 6. Create and Operator Reconciliation

A successful create means:

```text
desired state accepted into Kubernetes
```

It does not necessarily mean the serving workload is already ready.

After creation:

```text
ModelService appears
   |
   v
operator sees resource
   |
   v
operator reconciles dependent resources
   |
   v
status changes
```

---

## 7. Create Audit Event

Mutation audit logging records successful create requests.

Observed event fields included:

```text
event=api_audit
method=POST
route=/api/v1/model-services
resource_type=ModelService
status=201
outcome=success
username=service-account-ai-platform-service
```

For POST, `resource_name` may be empty because the audit middleware intentionally does not reread the consumed request body.

---

## 8. Update ModelService

Endpoint:

```http
PUT /api/v1/model-services/{name}
```

Required roles:

```text
platform-deployer
platform-admin
```

Purpose:

```text
replace/update desired ModelService state
```

---

## 9. Update Path Identity

The resource name comes from:

```text
{name}
```

in the URL.

Example:

```text
PUT /api/v1/model-services/fraud-model
```

The API applies the operation to:

```text
ai-platform/fraud-model
```

---

## 10. Update Flow

```text
HTTP PUT
   |
   v
authenticate
   |
   v
authorize deployer/admin
   |
   v
validate request
   |
   v
load/update ModelService
   |
   v
Kubernetes update
   |
   v
response
```

---

## 11. Patch ModelService

Endpoint:

```http
PATCH /api/v1/model-services/{name}
```

Required roles:

```text
platform-deployer
platform-admin
```

Purpose:

```text
partial desired-state update
```

Patch is used when replacing the whole resource is unnecessary.

---

## 12. Patch Flow

```text
HTTP PATCH
   |
   v
authenticate
   |
   v
authorize deployer/admin
   |
   v
validate patch
   |
   v
apply partial change
   |
   v
Kubernetes mutation
```

---

## 13. Delete ModelService

Endpoint:

```http
DELETE /api/v1/model-services/{name}
```

Required role:

```text
platform-admin
```

Expected success:

```text
204 No Content
```

Deletion is deliberately more restricted than create/update/patch.

---

## 14. Delete Flow

```text
HTTP DELETE
   |
   v
Envoy policy
   |
   v
API authentication
   |
   v
admin authorization
   |
   v
Kubernetes delete
   |
   v
204
```

---

## 15. Deployer DELETE Denial

A machine/deployer identity attempted DELETE externally.

Observed result:

```text
403
```

The request was rejected by Envoy before reaching the Go API.

Therefore no Go mutation audit event was expected.

---

## 16. Admin DELETE Success

An interactive admin token was used for DELETE.

Observed result:

```text
204
```

Audit event:

```text
event=api_audit
method=DELETE
route=/api/v1/model-services/{name}
resource_name=audit-probe
status=204
outcome=success
username=admin-user
```

---

## 17. Delete Kubernetes Verification

After successful deletion:

```bash
kubectl get modelservice \
  audit-probe \
  -n ai-platform
```

returned:

```text
NotFound
```

This proved that the API mutation affected actual Kubernetes state.

---

## 18. Mutation Audit Scope

Audited methods:

```text
POST
PUT
PATCH
DELETE
```

GET is intentionally excluded from mutation audit logging.

---

## 19. Audit Fields

Audit events include fields such as:

```text
event
request_id
method
route
resource_type
resource_name
status
outcome
subject
username
roles
```

The token is never logged.

---

## 20. Audit Outcomes

Possible application audit outcomes include:

```text
success
unauthorized
denied
rejected
error
```

This allows mutation analysis to distinguish caller/auth failures from validation and backend failures.

---

## 21. Request Validation

Every mutation is validated before Kubernetes state is modified.

Validation belongs under:

```text
internal/api/validation/
```

This prevents the API from acting as a raw Kubernetes pass-through.

---

## 22. Namespace Enforcement

Mutation bodies cannot redirect operations into another namespace.

The API always targets:

```text
ai-platform
```

This applies to:

```text
POST
PUT
PATCH
DELETE
```

---

## 23. Create Conflict

If a create conflicts with an existing resource, the API should return an appropriate conflict response rather than silently overwrite it.

Semantic class:

```text
409 Conflict
```

The exact response body format is handled by the response layer.

---

## 24. Not Found on Update/Patch/Delete

When the target resource does not exist, the expected class is:

```text
404 Not Found
```

This is distinct from:

```text
403 authorization failure
409 conflict
5xx backend failure
```

---

## 25. Server-Side Mutation Failure

Unexpected Kubernetes/backend failures map to:

```text
5xx
```

These count against the request-based availability/error SLI.

---

## 26. Mutation Metrics

Mutations contribute to:

```text
ai_platform_api_http_requests_total
ai_platform_api_http_request_duration_seconds
ai_platform_api_http_requests_in_flight
```

Normalized routes include:

```text
/api/v1/model-services
/api/v1/model-services/{name}
```

---

## 27. Low-Cardinality Mutation Metrics

The actual model name is not used as the metric route label.

Example:

```text
DELETE /api/v1/model-services/audit-probe
```

is recorded as:

```text
route="/api/v1/model-services/{name}"
```

---

## 28. Mutation Logging

Normal request logs and audit logs serve different purposes.

### Request log

Operational HTTP behavior:

```text
request ID
method
status
timing
```

### Audit log

Security/change evidence:

```text
who
what mutation
which resource
outcome
```

Both are useful and intentionally separate.

---

## 29. Mutation Authorization Matrix

| Method | Viewer | Deployer | Admin |
|---|---:|---:|---:|
| POST | Deny | Allow | Allow |
| PUT | Deny | Allow | Allow |
| PATCH | Deny | Allow | Allow |
| DELETE | Deny | Deny | Allow |

---

## 30. Create Example Authentication

Machine token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Load:

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

The exact create payload should follow the API contract implemented in the repository.

Do not bypass API validation by submitting raw arbitrary Kubernetes YAML.

---

## 31. Admin Authentication for Delete

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Load:

```bash
ADMIN_TOKEN="$(
  cat .local/keycloak/tokens/user-access-token.jwt
)"
```

This flow was used for admin-only deletion validation.

---

## 32. CRUD E2E Workflow

Automated validation script:

```text
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

Observed result:

```text
PASS 20/20
```

This is important because it validates multiple mutation steps together rather than only isolated handlers.

---

## 33. What the E2E Workflow Proves

The successful workflow provides evidence across:

```text
authentication
authorization
routing
validation
Kubernetes CRUD
HTTP status handling
resource lifecycle
```

It is stronger than a unit test alone.

---

## 34. Mutation vs Reconciliation

The API controls the request/desired-state transaction.

The operator controls reconciliation.

Therefore:

```text
HTTP mutation success
```

and:

```text
serving workload healthy
```

are separate states.

The status endpoint is used to observe reconciliation progress/outcome.

---

## 35. Update Idempotency Consideration

PUT is intended to represent the desired full update/replace semantics of the API.

Repeated submission of the same desired state should not introduce unintended differences.

The Kubernetes/operator architecture supports idempotent reconciliation of the resulting desired state.

---

## 36. Patch Intent

PATCH is for targeted change.

This is operationally useful where automation wants to change one part of a ModelService without reconstructing the entire representation.

---

## 37. Delete Lifecycle

Deletion flow continues beyond the HTTP request:

```text
DELETE API
   |
   v
ModelService removed / deletion initiated
   |
   v
operator / Kubernetes ownership handling
   |
   v
dependent cleanup
```

The REST API is not responsible for manually deleting every dependent object.

---

## 38. Mutation Request IDs

Every request receives/propagates:

```text
X-Request-ID
```

Audit events include the request ID so a mutation can be correlated with normal request logs.

---

## 39. Mutation Security Layers

A mutation must pass:

```text
TLS
Envoy JWT validation
Envoy role authorization
API JWT validation
API role authorization
request validation
namespace restriction
Kubernetes RBAC
NetworkPolicy
```

This is intentionally stricter than direct Kubernetes access.

---

## 40. Why the API Does Not Accept Arbitrary Kubernetes Objects

The API is a platform contract around `ModelService`.

It is not:

```text
POST /kubernetes-object
```

or a general Kubernetes proxy.

This prevents callers from using the API to create unrelated resources.

---

## 41. Validation Before Mutation

The desired sequence is:

```text
parse
  ->
authenticate
  ->
authorize
  ->
validate
  ->
mutate
```

not:

```text
mutate
  ->
discover invalid input
```

---

## 42. Mutation Failure Evidence

When a mutation fails, use:

```text
HTTP status
request log
audit log
Envoy policy/logs
Kubernetes API error
operator status/logs
```

depending on where the failure occurred.

---

## 43. Edge-Denied Mutation

If Envoy denies a mutation:

```text
no application audit event
```

This is expected, not a logging bug.

The edge is the source of truth for that rejection.

---

## 44. API-Denied Mutation

If the request reaches the API and is denied by application authorization:

```text
audit middleware can capture the outcome
```

because authenticated identity is already present.

---

## 45. Validation-Rejected Mutation

Validation failure should:

```text
not modify Kubernetes state
return a client error
produce suitable request/audit evidence
```

This protects the control plane from malformed desired state.

---

## 46. Successful Mutation

A successful mutation should:

```text
modify desired Kubernetes state
return expected HTTP status
emit normal request metrics/logging
emit mutation audit event
trigger operator reconciliation where applicable
```

---

## 47. Current Mutation Status

```text
[✓] POST implemented
[✓] PUT implemented
[✓] PATCH implemented
[✓] DELETE implemented
[✓] deployer/admin mutation authorization
[✓] admin-only delete
[✓] validation before mutation
[✓] fixed namespace
[✓] Kubernetes CRUD integration
[✓] structured audit logging
[✓] mutation metrics
[✓] admin delete validated
[✓] deployer delete denial validated
[✓] deleted resource verified absent
[✓] CRUD E2E PASS 20/20
```

Mutation functionality is implementation-complete.

---

## 48. Summary

The mutation API is:

```text
POST   /api/v1/model-services
PUT    /api/v1/model-services/{name}
PATCH  /api/v1/model-services/{name}
DELETE /api/v1/model-services/{name}
```

with:

```text
deployer/admin -> create/update/patch
admin          -> delete
```

Every mutation is constrained by authentication, role authorization, validation, fixed namespace, Kubernetes RBAC, audit logging, and operator reconciliation.
