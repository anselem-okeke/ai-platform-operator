# AI Platform REST API — API Contract and Project Structure

## 1. Purpose

This document describes the public API contract and internal Go project structure of the AI Platform REST API.

It records:

- the API versioning model
- endpoint structure
- resource naming conventions
- request/response behavior
- authorization expectations per endpoint
- error semantics
- routing organization
- package layout
- responsibilities of the main Go packages
- how the HTTP layer maps to Kubernetes `ModelService` operations

This document complements:

```text
docs/rest-api/00-overview-and-architecture.md
```

The overview document explains the complete architecture. This document focuses specifically on **what the API exposes** and **how the Go implementation is organized internally**.

---

# 2. API Base Path

The REST API uses the versioned prefix:

```text
/api/v1
```

The primary managed resource is:

```text
ModelService
```

The resource collection base path is:

```text
/api/v1/model-services
```

External API hostname:

```text
https://api.ai-platform.local
```

Therefore the external ModelService collection URL is:

```text
https://api.ai-platform.local/api/v1/model-services
```

---

# 3. API Versioning Strategy

The initial REST API version is:

```text
v1
```

and is encoded directly in the URL:

```text
/api/v1/...
```

This keeps the external REST contract independent from the Kubernetes CRD version:

```text
platform.anselem.dev/v1alpha1
```

These are intentionally separate concerns.

```text
REST API version
    /api/v1
        |
        | platform-facing contract
        v

Kubernetes resource version
    platform.anselem.dev/v1alpha1
        |
        | controller/operator contract
        v
    ModelService CR
```

The REST API can therefore evolve without requiring every API contract change to map directly to a CRD version change.

---

# 4. Kubernetes Resource Managed by the API

The API manages:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
```

The REST API creates, reads, updates, patches, and deletes `ModelService` resources in:

```text
namespace: ai-platform
```

The namespace is fixed by the service and is not caller-selectable.

---

# 5. Endpoint Summary

Implemented endpoints:

| Method | Path | Authentication | Required Role | Purpose |
|---|---|---:|---|---|
| `GET` | `/healthz` | No app JWT | None | Liveness |
| `GET` | `/readyz` | No app JWT | None | Readiness |
| `GET` | `/metrics` | No app JWT internally | None | Prometheus scrape |
| `GET` | `/api/v1/model-services` | Yes | viewer/deployer/admin | List ModelServices |
| `GET` | `/api/v1/model-services/{name}` | Yes | viewer/deployer/admin | Get ModelService |
| `GET` | `/api/v1/model-services/{name}/status` | Yes | viewer/deployer/admin | Get ModelService status |
| `POST` | `/api/v1/model-services` | Yes | deployer/admin | Create ModelService |
| `PUT` | `/api/v1/model-services/{name}` | Yes | deployer/admin | Update/replace ModelService |
| `PATCH` | `/api/v1/model-services/{name}` | Yes | deployer/admin | Partial update |
| `DELETE` | `/api/v1/model-services/{name}` | Yes | admin | Delete ModelService |

---

# 6. Health Endpoint

## `GET /healthz`

Purpose:

```text
process liveness
```

Example:

```bash
curl -sS   http://ai-platform-api.ai-platform.svc.cluster.local:8080/healthz
```

Expected success status:

```text
200 OK
```

The endpoint is used by Kubernetes liveness checks and operational validation.

It is not part of the customer-facing SLI request set.

---

# 7. Readiness Endpoint

## `GET /readyz`

Purpose:

```text
application readiness
```

Example:

```bash
curl -sS   http://ai-platform-api.ai-platform.svc.cluster.local:8080/readyz
```

Expected healthy status:

```text
200 OK
```

The endpoint is used by Kubernetes readiness checks.

Like `/healthz`, it is excluded from customer-facing request-rate, error-ratio, and latency SLIs.

---

# 8. Metrics Endpoint

## `GET /metrics`

Purpose:

```text
Prometheus metrics exposition
```

The endpoint is scraped by the `ServiceMonitor`.

Prometheus reaches the API over the internal Service path rather than through the external Envoy Gateway.

The endpoint exposes application metrics such as:

```text
ai_platform_api_http_requests_total
ai_platform_api_http_request_duration_seconds
ai_platform_api_http_requests_in_flight
```

The metrics endpoint itself is excluded from request metrics instrumentation to avoid self-observation recursion/noise.

---

# 9. List ModelServices

## `GET /api/v1/model-services`

Purpose:

```text
return ModelService resources in the managed namespace
```

Required roles:

```text
platform-viewer
platform-deployer
platform-admin
```

Example:

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"

curl   --cacert .local/keycloak/fraud-model-root-ca.crt   -sS   -H "Authorization: Bearer ${TOKEN}"   https://api.ai-platform.local/api/v1/model-services
```

The API does not list arbitrary namespaces.

All reads are scoped to:

```text
ai-platform
```

---

# 10. Get ModelService

## `GET /api/v1/model-services/{name}`

Purpose:

```text
return a specific ModelService
```

Example:

```text
GET /api/v1/model-services/fraud-model
```

Required roles:

```text
platform-viewer
platform-deployer
platform-admin
```

The route uses a normalized route identity for metrics:

```text
/api/v1/model-services/{name}
```

The actual model name is **not** used as a Prometheus route label.

---

# 11. Get ModelService Status

## `GET /api/v1/model-services/{name}/status`

Purpose:

```text
return status information for a ModelService
```

Example:

```text
GET /api/v1/model-services/fraud-model/status
```

Required roles:

```text
platform-viewer
platform-deployer
platform-admin
```

This endpoint exposes the observed platform state without requiring the caller to inspect the complete Kubernetes resource directly.

The status is ultimately maintained by the AI Platform Operator during reconciliation.

---

# 12. Create ModelService

## `POST /api/v1/model-services`

Purpose:

```text
create a new ModelService desired-state object
```

Required roles:

```text
platform-deployer
platform-admin
```

The caller does not provide arbitrary namespace scope.

The API validates the request and writes the resulting resource into:

```text
ai-platform
```

A successful create returns:

```text
HTTP 201 Created
```

A successful create also emits a structured mutation audit event.

Observed audit behavior includes:

```text
event=api_audit
method=POST
route=/api/v1/model-services
status=201
outcome=success
```

---

# 13. Update ModelService

## `PUT /api/v1/model-services/{name}`

Purpose:

```text
replace or update the desired ModelService specification
```

Required roles:

```text
platform-deployer
platform-admin
```

Example:

```text
PUT /api/v1/model-services/fraud-model
```

The resource name is taken from the URL path.

The API validates the update before writing it to Kubernetes.

The operator then reconciles the changed desired state.

---

# 14. Patch ModelService

## `PATCH /api/v1/model-services/{name}`

Purpose:

```text
partially modify a ModelService
```

Required roles:

```text
platform-deployer
platform-admin
```

Example:

```text
PATCH /api/v1/model-services/fraud-model
```

This is intended for partial mutation where replacing the whole desired object is unnecessary.

As with other mutations, the request is validated and audited.

---

# 15. Delete ModelService

## `DELETE /api/v1/model-services/{name}`

Purpose:

```text
delete a ModelService
```

Required role:

```text
platform-admin
```

Deletion is intentionally more restrictive than create/update/patch.

Example:

```text
DELETE /api/v1/model-services/audit-probe
```

A validated admin deletion returned:

```text
HTTP 204 No Content
```

and produced an application audit event containing:

```text
event=api_audit
method=DELETE
route=/api/v1/model-services/{name}
resource_name=audit-probe
status=204
outcome=success
username=admin-user
```

The resource was subsequently confirmed absent in Kubernetes.

---

# 16. Role Matrix

The API authorization contract is:

| API operation | `platform-viewer` | `platform-deployer` | `platform-admin` |
|---|---:|---:|---:|
| `GET /api/v1/model-services` | Allow | Allow | Allow |
| `GET /api/v1/model-services/{name}` | Allow | Allow | Allow |
| `GET /api/v1/model-services/{name}/status` | Allow | Allow | Allow |
| `POST /api/v1/model-services` | Deny | Allow | Allow |
| `PUT /api/v1/model-services/{name}` | Deny | Allow | Allow |
| `PATCH /api/v1/model-services/{name}` | Deny | Allow | Allow |
| `DELETE /api/v1/model-services/{name}` | Deny | Deny | Allow |

This contract is enforced in the Go API and also reflected in the Envoy edge authorization design.

---

# 17. Authentication Contract

Protected routes expect:

```http
Authorization: Bearer <JWT>
```

The JWT is issued by Keycloak.

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Audience:

```text
ai-platform-gateway
```

The API validates identity before authorization.

A request without valid authentication must not reach protected handler logic.

---

# 18. Request Correlation Contract

The API uses:

```text
X-Request-ID
```

A request ID is assigned or propagated by middleware.

It is used for:

- structured request logging
- mutation audit logging
- troubleshooting
- correlating failures across middleware and handlers

Request IDs are **not** used as Prometheus labels.

---

# 19. HTTP Error Semantics

The API distinguishes authentication, authorization, validation, resource lookup, and server failures.

The expected response classes are:

| Status | Meaning |
|---:|---|
| `200` | Successful read/update-style response |
| `201` | Successful resource creation |
| `204` | Successful deletion with no response body |
| `400` | Invalid request / validation failure |
| `401` | Authentication failure |
| `403` | Authorization failure |
| `404` | Requested resource not found |
| `409` | Resource conflict where applicable |
| `5xx` | API/Kubernetes/backend failure |

The implementation uses centralized response/error handling rather than returning arbitrary handler-specific formats.

---

# 20. Availability Semantics and HTTP Status Codes

The SLI model intentionally treats:

```text
5xx
```

as service-side availability failures.

Non-5xx responses are not counted as service availability failures.

This means:

```text
401 -> authentication/caller issue
403 -> authorization decision
404 -> valid negative lookup
5xx -> server-side failure
```

This distinction is important because the REST API contract and the SLO model must agree on what constitutes an availability failure.

---

# 21. Request Validation

Mutation handlers validate incoming requests before writing Kubernetes resources.

Validation responsibilities are isolated under:

```text
internal/api/validation/
```

Validation prevents malformed or unsupported requests from being passed directly through to the Kubernetes API.

This is part of the API boundary:

```text
external request
      |
      v
REST API validation
      |
      v
approved ModelService representation
      |
      v
Kubernetes
```

The REST API is therefore not a raw pass-through proxy.

---

# 22. Namespace Enforcement

Namespace scope is not controlled by request parameters.

The API uses the fixed namespace:

```text
ai-platform
```

This applies consistently to:

- list
- get
- status
- create
- update
- patch
- delete

This prevents cross-namespace escalation through the REST contract.

---

# 23. Go Entry Point

Main executable entry point:

```text
cmd/platform-api/main.go
```

The entry point is responsible for application startup and dependency construction.

Its responsibilities include wiring:

- configuration
- logging
- Kubernetes client
- OIDC authentication
- API server
- routes
- middleware
- graceful server behavior

The detailed startup flow is documented separately in:

```text
02-configuration-and-startup.md
```

---

# 24. Main API Package

The top-level API package includes:

```text
internal/api/routes.go
internal/api/server.go
```

These files provide the central HTTP server/routing composition.

The architecture keeps route registration and server composition separate from individual handler implementation.

Conceptually:

```text
main
 |
 v
server
 |
 v
routes
 |
 +--> middleware
 |
 +--> handlers
```

---

# 25. `internal/api/auth`

Purpose:

```text
OIDC/JWT authentication and identity extraction
```

Responsibilities include:

- token validation
- issuer/audience trust
- authenticated identity representation
- role extraction
- identity propagation into request context

The authorization middleware consumes the authenticated identity established by this layer.

---

# 26. `internal/api/config`

Purpose:

```text
runtime configuration loading
```

Configuration includes the values needed to initialize the API, such as:

- listen/server configuration
- managed namespace
- OIDC issuer
- OIDC audience
- trust/CA settings
- Kubernetes integration parameters

Configuration parsing is separated from request handling.

---

# 27. `internal/api/handlers`

Purpose:

```text
HTTP endpoint implementation
```

Handlers implement the REST operations exposed by the router.

Responsibilities include:

- read operations
- status retrieval
- create
- update
- patch
- delete
- translating service/store outcomes into HTTP responses

Handlers do not implement the operator reconciliation loop.

---

# 28. `internal/api/kubernetes`

Purpose:

```text
Kubernetes persistence/integration layer
```

This package encapsulates interaction with the Kubernetes API through the controller-runtime client.

Responsibilities include operations such as:

- list ModelServices
- get ModelService
- create ModelService
- update ModelService
- patch ModelService
- delete ModelService

This keeps Kubernetes API details out of HTTP routing code.

The logical layering is:

```text
HTTP handler
     |
     v
Kubernetes abstraction/store
     |
     v
controller-runtime client
     |
     v
Kubernetes API
```

---

# 29. `internal/api/logging`

Purpose:

```text
structured logging support
```

The API uses Go `slog` and emits structured JSON logs.

The logging package centralizes logger construction/configuration instead of scattering logger initialization through handlers.

---

# 30. `internal/api/metrics`

Purpose:

```text
Prometheus collector definitions
```

Main collectors:

```text
ai_platform_api_http_requests_total
ai_platform_api_http_request_duration_seconds
ai_platform_api_http_requests_in_flight
```

The metrics package owns the collector definitions while middleware owns request-time observation.

---

# 31. `internal/api/middleware`

Purpose:

```text
cross-cutting HTTP behavior
```

This package contains middleware for concerns including:

- request IDs
- request logging
- authentication
- authorization
- audit logging
- Prometheus request metrics

The middleware ordering is deliberate and documented in the architecture overview.

Important implementation areas include:

```text
logging.go
metrics.go
audit.go
```

The package also contains corresponding tests.

---

# 32. `internal/api/request`

Purpose:

```text
request-specific helper types and parsing
```

This package keeps inbound HTTP request concerns separated from Kubernetes persistence types and response helpers.

This reduces coupling between the external REST contract and the internal Kubernetes object representation.

---

# 33. `internal/api/response`

Purpose:

```text
consistent HTTP response writing
```

This package centralizes response/error serialization behavior.

Handlers therefore do not need to invent different response conventions independently.

This is especially important for predictable API error handling.

---

# 34. `internal/api/validation`

Purpose:

```text
input validation
```

Validation is kept separate from:

- routing
- authentication
- persistence
- reconciliation

This makes the API boundary explicit and testable.

---

# 35. Routing Design

The API uses Go:

```text
http.ServeMux
```

Protected application paths live under:

```text
/api/v1/
```

Public/internal operational paths include:

```text
/healthz
/readyz
/metrics
```

The protected route path is wrapped with authentication and application authorization middleware.

This separates:

```text
operational endpoint access
```

from:

```text
user-facing API access
```

---

# 36. Route Normalization

A routing detail discovered during metrics implementation was that nested `ServeMux` behavior made direct reliance on `r.Pattern` insufficient for the desired metric labels.

The API therefore normalizes known paths explicitly.

Normalized metric routes include:

```text
/healthz
/readyz
/metrics
/api/v1/model-services
/api/v1/model-services/{name}
/api/v1/model-services/{name}/status
unmatched
```

This ensures:

```text
/api/v1/model-services/fraud-model
```

and:

```text
/api/v1/model-services/another-model
```

produce the same metric route label:

```text
/api/v1/model-services/{name}
```

---

# 37. Why Route Normalization Matters

Using raw paths as labels would create potentially unbounded cardinality:

```text
route="/api/v1/model-services/model-a"
route="/api/v1/model-services/model-b"
route="/api/v1/model-services/model-c"
...
```

The normalized design instead produces:

```text
route="/api/v1/model-services/{name}"
```

This is safer for Prometheus and makes dashboards easier to interpret.

---

# 38. Request Logging Middleware

The request logging middleware records request-level operational events.

A response recorder tracks the final HTTP status.

This avoids incorrectly logging only the default response status when handlers return different statuses.

The logging middleware sits outside protected routing so it can observe request outcomes consistently.

---

# 39. Request Metrics Middleware

Metrics middleware records:

```text
request count
request duration
requests in flight
```

The middleware:

- increments in-flight requests
- delegates to the next handler
- captures the final response status
- normalizes the route
- records request count
- records latency
- decrements in-flight requests

The `/metrics` endpoint is skipped by the middleware.

---

# 40. Audit Middleware

Audit middleware applies only to mutations:

```text
POST
PUT
PATCH
DELETE
```

It executes after authentication so identity is available.

It wraps authorization/handler execution so it can record:

- success
- unauthorized
- denied
- rejected
- error

For resource paths containing `{name}`, the middleware can use:

```text
r.PathValue("name")
```

to record the resource name.

For POST, the middleware intentionally does not reread the request body after the handler, so `resource_name` may remain empty.

---

# 41. API-to-Operator Separation

A critical design decision is that handlers do **not** create the full runtime workload directly.

For example, a create operation does this:

```text
POST /api/v1/model-services
       |
       v
validate request
       |
       v
create ModelService CR
       |
       v
return API response
```

Then separately:

```text
Operator watches ModelService
       |
       v
reconcile desired state
       |
       v
Deployment / Service / PVC / SA / policies / routes
```

This preserves Kubernetes control-loop semantics.

---

# 42. Status Read Model

The status endpoint is intentionally separate from the main resource endpoint:

```text
GET /api/v1/model-services/{name}/status
```

This gives platform consumers a direct way to retrieve observed lifecycle state.

The API is reading status; the operator is responsible for producing/updating status.

Conceptually:

```text
spec
  -> desired state

status
  -> observed/reconciled state
```

---

# 43. External vs Internal Endpoint Behavior

The application has internal operational endpoints, but the external Gateway policy may protect a broader route depending on Envoy SecurityPolicy attachment.

Therefore there is an important distinction between:

```text
Go application endpoint authentication
```

and:

```text
external Gateway authentication
```

For example, `/healthz` can be public at the Go application layer while an externally attached SecurityPolicy may still require JWT before the request reaches the application.

Prometheus avoids this problem by scraping the ClusterIP Service directly.

---

# 44. Kubernetes Service Identity

The API ServiceAccount is:

```text
ai-platform-api
```

The API requires Kubernetes credentials because it manages `ModelService` resources.

Therefore:

```text
automountServiceAccountToken: true
```

remains intentional.

This is not a security-hardening omission; it is required functionality combined with least-privilege RBAC.

---

# 45. API Deployment Identity

Runtime user:

```text
65532:65532
```

The runtime image is distroless/non-root.

The API is therefore designed so that its HTTP and Kubernetes functionality does not depend on root privileges or a writable root filesystem.

---

# 46. Testing Structure

The API implementation includes tests around multiple layers.

Examples of tested areas include:

```text
routing
authentication
authorization
Kubernetes integration
request metrics
audit logging
handler behavior
operator/security interactions
```

Prometheus rule tests are maintained separately under:

```text
infrastructure/monitoring/rule-tests/
```

End-to-end CRUD validation is maintained under:

```text
infrastructure/platform-api/scripts/
```

This separates:

```text
Go unit/integration behavior
```

from:

```text
API workflow validation
```

and from:

```text
Prometheus rule evaluation
```

---

# 47. E2E CRUD Validation

The automated workflow is:

```text
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

Observed result:

```text
PASS 20/20
```

This validates the API contract beyond unit-level behavior.

It provides evidence that authentication, authorization, routing, Kubernetes operations, and expected HTTP outcomes work together.

---

# 48. API Contract Security Boundaries

The effective security boundaries are:

```text
1. TLS
2. Envoy JWT validation
3. Envoy role policy
4. Go API JWT validation
5. Go API role authorization
6. request validation
7. fixed namespace
8. Kubernetes RBAC
9. NetworkPolicy
10. non-root runtime hardening
```

The API contract must be understood together with these layers.

An endpoint being present does not imply every authenticated caller can invoke it.

---

# 49. Example Request Flow — List

```text
GET /api/v1/model-services
        |
        v
Envoy validates token
        |
        v
API validates token
        |
        v
API checks viewer/deployer/admin
        |
        v
Kubernetes list in ai-platform
        |
        v
HTTP 200
```

---

# 50. Example Request Flow — Create

```text
POST /api/v1/model-services
        |
        v
Envoy validates token/role
        |
        v
API authentication
        |
        v
API requires deployer/admin
        |
        v
request validation
        |
        v
create ModelService in ai-platform
        |
        v
HTTP 201
        |
        +--> mutation audit event
        |
        v
operator reconciliation
```

---

# 51. Example Request Flow — Delete

```text
DELETE /api/v1/model-services/{name}
        |
        v
Envoy policy
        |
        v
API authentication
        |
        v
API requires platform-admin
        |
        v
Kubernetes delete
        |
        v
HTTP 204
        |
        +--> mutation audit event
```

A lower-privileged caller may be rejected at Envoy before the Go API sees the request.

---

# 52. API Contract Principles

The implemented API follows these principles:

### Versioned external contract

```text
/api/v1
```

### Kubernetes-native backend

The API manages the `ModelService` CR rather than maintaining a separate database of platform state.

### Fixed namespace

The API cannot be used to reach arbitrary Kubernetes namespaces.

### Explicit authorization

Each operation has a defined minimum role.

### Predictable HTTP semantics

Create, read, update, patch, delete, authentication, authorization, validation, and backend failure each map to expected HTTP status classes.

### Separate status endpoint

Observed lifecycle state is exposed explicitly.

### Observable by default

Requests are logged, metered, and mutations audited.

### Operator-driven lifecycle

The API changes desired state; reconciliation stays in the operator.

---

# 53. Current Contract Completion Status

```text
[✓] Versioned API prefix defined
[✓] Health endpoint implemented
[✓] Readiness endpoint implemented
[✓] Metrics endpoint implemented
[✓] List endpoint implemented
[✓] Get endpoint implemented
[✓] Status endpoint implemented
[✓] Create endpoint implemented
[✓] Update endpoint implemented
[✓] Patch endpoint implemented
[✓] Delete endpoint implemented
[✓] JWT authentication implemented
[✓] Role-based authorization implemented
[✓] Namespace restriction implemented
[✓] Validation implemented
[✓] Structured errors implemented
[✓] Request IDs implemented
[✓] Request logging implemented
[✓] Mutation audit logging implemented
[✓] Prometheus request metrics implemented
[✓] Kubernetes client abstraction implemented
[✓] Least-privilege ServiceAccount/RBAC implemented
[✓] CRUD E2E workflow validated
```

The API contract and project structure are therefore implementation-complete.

---

# 54. Relationship to the Remaining Documentation

This document focuses on the contract and code layout.

The following documents expand individual areas:

```text
02-configuration-and-startup.md
03-kubernetes-client-and-rbac.md
04-read-endpoints.md
05-authentication-and-authorization.md
06-create-update-patch-delete.md
07-validation-errors-and-namespace-restrictions.md
08-container-and-kubernetes-deployment.md
09-envoy-gateway-and-https.md
10-networkpolicy-and-runtime-hardening.md
11-audit-logging-and-prometheus-metrics.md
12-grafana-alerting-and-slo.md
13-testing-and-end-to-end-validation.md
14-recovery-and-troubleshooting.md
15-complete-command-reference.md
```

---

# 55. Summary

The AI Platform REST API exposes a small, versioned control-plane contract around `ModelService`.

The external contract is:

```text
health
readiness
metrics

list
get
status

create
update
patch
delete
```

The internal implementation is organized into dedicated packages for:

```text
auth
config
handlers
kubernetes
logging
metrics
middleware
request
response
validation
```

The API is intentionally constrained by:

```text
OIDC/JWT
role authorization
fixed namespace
Kubernetes RBAC
NetworkPolicy
runtime hardening
```

and integrates directly with the existing operator model:

```text
REST API
   ->
ModelService
   ->
Operator
   ->
Serving workload
```

This completes the API contract and project structure definition for the REST API phase.
