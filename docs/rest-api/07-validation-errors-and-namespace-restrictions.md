# AI Platform REST API — Validation, Error Handling, and Namespace Restrictions

## 1. Purpose

This document describes how the AI Platform REST API validates requests, returns errors, and enforces namespace restrictions.

It records:

- validation responsibilities
- HTTP error classes
- authentication/authorization errors
- Kubernetes error translation
- namespace enforcement
- security implications
- observability behavior for failures
- operational troubleshooting guidance

---

## 2. Validation as an API Boundary

The REST API is deliberately not a raw Kubernetes proxy.

Incoming mutation requests are validated before they are turned into `ModelService` changes.

The intended flow is:

```text
external request
      |
      v
parse request
      |
      v
authenticate
      |
      v
authorize
      |
      v
validate
      |
      v
Kubernetes mutation
```

Invalid requests should stop before Kubernetes state changes.

---

## 3. Validation Package

Validation code lives under:

```text
internal/api/validation/
```

Keeping validation separate from handlers improves:

```text
testability
consistency
reuse
clarity
```

---

## 4. Request Package

Inbound request types/helpers live under:

```text
internal/api/request/
```

This helps separate:

```text
external REST representation
```

from:

```text
Kubernetes persistence representation
```

---

## 5. Response Package

Response writing/error handling lives under:

```text
internal/api/response/
```

This prevents each handler from inventing its own response format and status behavior.

---

## 6. Error Classes

The API distinguishes common response classes.

| Status | Meaning |
|---:|---|
| `200` | successful request |
| `201` | resource created |
| `204` | successful deletion |
| `400` | invalid request |
| `401` | authentication failed |
| `403` | authorization failed |
| `404` | resource not found |
| `409` | resource conflict |
| `5xx` | server/Kubernetes/backend failure |

The exact response body shape should be read from the response package/source if field-level details are needed.

---

## 7. Validation Failure

A malformed or unsupported mutation should result in:

```text
4xx
```

rather than:

```text
5xx
```

because the server is functioning but the request is invalid.

A validation rejection must not change Kubernetes state.

---

## 8. Authentication Error

Authentication failures use:

```text
401 Unauthorized
```

Examples:

```text
missing Authorization header
expired JWT
invalid JWT signature
wrong issuer
wrong audience
```

The API should not continue into role authorization or mutation handling with an unauthenticated identity.

---

## 9. Authorization Error

Authorization failures use:

```text
403 Forbidden
```

Examples:

```text
viewer attempts POST
deployer attempts DELETE
caller lacks required platform role
```

A valid identity can still be unauthorized.

---

## 10. Edge vs Application Error

A `401` or `403` can originate from different layers.

### Envoy

```text
Client -> Envoy -> reject
```

### Go API

```text
Client -> Envoy -> API -> reject
```

Troubleshooting must identify the rejecting layer.

---

## 11. Resource Not Found

Missing ModelService:

```text
404 Not Found
```

Examples:

```text
GET unknown model
PUT unknown model
PATCH unknown model
DELETE unknown model
```

A valid 404 is not treated as a service availability failure.

---

## 12. Conflict

Kubernetes resource conflicts can map to:

```text
409 Conflict
```

This keeps concurrency/resource-existence problems distinct from generic server failures.

---

## 13. Server Failure

Unexpected API/Kubernetes failures map to:

```text
5xx
```

These are service-side failures and are counted by the availability/error SLI.

---

## 14. SLO Interpretation of Errors

The current request-based availability model treats:

```text
5xx -> service failure
non-5xx -> available service response
```

Therefore:

```text
401
403
404
409
```

do not automatically count as availability failures.

This is intentional.

---

## 15. Why 4xx Does Not Mean API Unavailable

Examples:

```text
401 -> caller did not authenticate correctly
403 -> policy denied the caller
404 -> requested resource absent
409 -> conflicting request/resource state
```

These responses can prove the API is functioning correctly.

---

## 16. Namespace Restriction

The API is fixed to:

```text
ai-platform
```

The caller cannot select:

```text
default
kube-system
monitoring
gateway-system
external-secrets
```

or any arbitrary namespace.

---

## 17. Namespace Enforcement Scope

The fixed namespace applies to:

```text
list
get
status
create
update
patch
delete
```

This is not only a create-time rule.

---

## 18. Why Namespace Restriction Is Security-Relevant

Without namespace restriction, an API caller could potentially try to use the platform API as a path into unrelated workloads or system namespaces.

The fixed namespace reduces:

```text
blast radius
authorization complexity
RBAC scope
audit ambiguity
policy complexity
```

---

## 19. Application Namespace Restriction + Kubernetes RBAC

Namespace isolation exists at two layers.

### Application

```text
always use ai-platform
```

### Kubernetes RBAC

```text
Role scoped to ai-platform
```

This creates defense in depth.

---

## 20. Negative Cross-Namespace RBAC Test

A useful check:

```bash
kubectl auth can-i \
  list \
  modelservices.platform.anselem.dev \
  -n default \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

Expected:

```text
no
```

This demonstrates the Kubernetes identity is also namespace-limited.

---

## 21. Valid Namespace RBAC Test

```bash
kubectl auth can-i \
  list \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

Expected:

```text
yes
```

---

## 22. Name Validation

Resource names come from the API contract.

For named routes:

```text
/api/v1/model-services/{name}
```

the API should reject malformed/unsupported names before unsafe backend behavior occurs.

The exact validation implementation remains in the source package.

---

## 23. Body Validation

POST/PUT/PATCH request bodies must be parsed and validated.

Validation should ensure the external representation can be safely converted into the desired `ModelService` state.

The API must not accept arbitrary fields merely because Kubernetes might ignore or reject them later.

---

## 24. Handler Error Translation

Handlers should translate internal errors into stable HTTP semantics.

Conceptually:

```text
Kubernetes NotFound
      ->
404

validation error
      ->
400

authorization failure
      ->
403

unexpected backend error
      ->
5xx
```

---

## 25. Why Raw Kubernetes Errors Should Not Leak

Returning raw internal errors can expose:

```text
cluster details
internal object paths
implementation internals
library messages
```

The response layer exists to maintain a stable external contract.

---

## 26. Request ID on Errors

Failures should still be correlated with:

```text
X-Request-ID
```

This is important for support/debugging.

A caller-facing error can be matched to structured application logs using the request ID.

---

## 27. Error Logging

Request logging records the final HTTP status through a response recorder.

This is important because middleware must observe statuses such as:

```text
400
401
403
404
500
```

instead of incorrectly assuming `200`.

---

## 28. Error Metrics

Failures contribute to:

```text
ai_platform_api_http_requests_total{status=...}
```

and latency metrics.

This supports:

```text
5xx alerting
status dashboards
SLO calculations
```

---

## 29. 5xx Alert

Alert:

```text
AIPlatformAPIHigh5xxRate
```

Threshold:

```text
> 5%
```

Duration:

```text
for: 5m
```

Health/readiness routes are excluded.

---

## 30. Error Ratio SLI

Recorded metric:

```text
ai_platform_api:sli_error_ratio:5m
```

Additional windows include:

```text
ai_platform_api:sli_error_ratio:1h
ai_platform_api:sli_error_ratio:6h
```

These feed error-budget burn calculations.

---

## 31. No 5xx Series Behavior

The 5xx numerator uses zero-safe behavior so an absent 5xx series means:

```text
0 observed 5xx
```

rather than causing the numerator to vanish.

The denominator is protected separately.

---

## 32. No Traffic Behavior

No customer traffic is not the same as:

```text
0% availability
```

or:

```text
100% availability
```

The SLI implementation intentionally preserves no-data semantics where appropriate.

---

## 33. Validation and Audit Logging

Mutation audit middleware can classify failures as:

```text
unauthorized
denied
rejected
error
```

This creates useful distinction between:

```text
auth failure
authorization failure
validation rejection
backend failure
```

---

## 34. Audit Scope

Only:

```text
POST
PUT
PATCH
DELETE
```

generate mutation audit events.

Read-only GET requests are not part of the mutation audit stream.

---

## 35. Envoy-Denied Errors

When Envoy rejects a request:

```text
application audit logging cannot record it
```

because the request never reaches the process.

This is expected.

Edge logs/policy state must be checked for those failures.

---

## 36. Application-Denied Errors

When a request reaches the application but fails authorization:

```text
application middleware can observe the identity and outcome
```

This is useful for audit evidence.

---

## 37. Validation-Rejected Errors

A validation-rejected mutation should:

```text
return a 4xx
not modify Kubernetes
emit request log
emit audit outcome where applicable
```

---

## 38. Backend Errors

If the API is authorized and the request is valid but Kubernetes fails unexpectedly:

```text
return 5xx
```

This should be visible in:

```text
request logs
Prometheus status metrics
5xx alerts
SLO/error-budget calculations
```

---

## 39. Kubernetes RBAC Errors

An internal Kubernetes `Forbidden` means the API ServiceAccount lacks required permission.

This is different from the caller lacking a platform role.

Check:

```bash
kubectl auth can-i ...
```

for the ServiceAccount.

---

## 40. Network Errors

A backend timeout may be caused by:

```text
NetworkPolicy
Kubernetes API connectivity
DNS
OIDC dependency
```

Do not immediately interpret every `5xx` as handler logic failure.

---

## 41. OIDC Errors

Startup/auth failures can be caused by:

```text
wrong issuer
wrong audience
expired token
bad CA
blocked OIDC network path
JWKS discovery failure
```

The hardening phase specifically exposed the NetworkPolicy/OIDC dependency.

---

## 42. Namespace Error Prevention

The best namespace error is one the caller cannot express.

The API design avoids accepting arbitrary namespace input rather than merely checking it late.

This is preferable to:

```text
accept namespace
then validate allowlist
```

because the contract stays smaller.

---

## 43. API Is Not a Generic Proxy

The API only manages:

```text
ModelService
```

in:

```text
ai-platform
```

It does not expose generic:

```text
/api/v1/namespaces/{namespace}/objects
```

behavior.

---

## 44. Error Contract Stability

Clients should be able to rely primarily on:

```text
HTTP status
documented API semantics
```

rather than internal Go error strings.

This supports future internal implementation changes.

---

## 45. Validation Test Coverage

Validation/error handling is covered by:

```text
Go tests
handler/router tests
auth tests
CRUD E2E workflow
manual authorization validation
```

The exact current test names live in the source tree.

---

## 46. CRUD E2E Validation

The workflow:

```text
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

passed:

```text
20/20
```

This provides evidence that validation/error behavior works in the integrated API flow.

---

## 47. Error Budget Relationship

The availability objective is:

```text
99.9%
```

Allowed service error ratio:

```text
0.001
```

Only service-side failure behavior should consume that budget.

This is why error classification matters.

---

## 48. Fast Burn Alert

```text
AIPlatformAPIErrorBudgetFastBurn
```

uses:

```text
5m > 14.4x
AND
1h > 14.4x
```

This detects rapid service-side error-budget consumption.

---

## 49. Slow Burn Alert

```text
AIPlatformAPIErrorBudgetSlowBurn
```

uses:

```text
1h > 6x
AND
6h > 6x
```

This detects sustained service degradation.

---

## 50. Troubleshooting Error Responses

Use this order:

```text
1. HTTP status
2. Was request rejected by Envoy?
3. Does API have a request log?
4. Does mutation audit show outcome?
5. Is token fresh?
6. Is caller role correct?
7. Did validation reject input?
8. Does resource exist?
9. Is ServiceAccount RBAC correct?
10. Can API reach Kubernetes?
```

---

## 51. Useful Token Refresh

Machine:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Interactive admin:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Short token lifetime means stale tokens are a common test artifact.

---

## 52. Useful Kubernetes Checks

Resource exists:

```bash
kubectl get modelservice \
  fraud-model \
  -n ai-platform
```

API ServiceAccount:

```bash
kubectl get sa \
  ai-platform-api \
  -n ai-platform
```

RBAC:

```bash
kubectl get role,rolebinding \
  -n ai-platform
```

---

## 53. Current Validation/Error/Namespace Status

```text
[✓] request validation implemented
[✓] response/error handling implemented
[✓] 400-class client errors supported
[✓] authentication errors separated
[✓] authorization errors separated
[✓] not-found behavior implemented
[✓] conflict semantics supported where applicable
[✓] server failures surfaced as 5xx
[✓] fixed namespace enforced
[✓] namespace-scoped RBAC enforced
[✓] request IDs available for correlation
[✓] failure statuses included in metrics
[✓] mutation failures auditable where request reaches API
[✓] E2E workflow validated
```

---

## 54. Summary

The API protects Kubernetes state through a small, explicit contract.

Requests are:

```text
authenticated
authorized
validated
namespace-constrained
translated into stable HTTP errors
observed through logs/metrics/audit
```

The API is intentionally limited to `ModelService` operations in `ai-platform`, and error classification is aligned with the request-based SLO model so client errors and service failures are not confused.
