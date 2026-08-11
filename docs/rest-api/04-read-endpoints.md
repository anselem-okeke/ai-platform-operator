# AI Platform REST API — Read Endpoints

## 1. Purpose

This document describes the read-only API surface of the AI Platform REST API.

It covers:

- listing `ModelService` resources
- retrieving one `ModelService`
- retrieving `ModelService.status`
- authentication and authorization requirements
- namespace scope
- response and error behavior
- Kubernetes interactions
- metrics and route normalization
- runtime validation

This document complements:

```text
00-overview-and-architecture.md
01-api-contract-and-project-structure.md
02-configuration-and-startup.md
03-kubernetes-client-and-rbac.md
```

---

## 2. Read Endpoint Summary

| Method | Path | Purpose | Required Role |
|---|---|---|---|
| `GET` | `/api/v1/model-services` | List ModelServices | viewer/deployer/admin |
| `GET` | `/api/v1/model-services/{name}` | Get ModelService | viewer/deployer/admin |
| `GET` | `/api/v1/model-services/{name}/status` | Get ModelService status | viewer/deployer/admin |

All read operations are scoped to:

```text
namespace: ai-platform
```

The caller cannot override the namespace.

---

## 3. Authentication Requirement

All `/api/v1/` routes require a valid Keycloak access token.

Header:

```http
Authorization: Bearer <JWT>
```

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Audience:

```text
ai-platform-gateway
```

A request that does not satisfy authentication must not reach the read handler.

---

## 4. Authorization Requirement

Read endpoints are available to:

```text
platform-viewer
platform-deployer
platform-admin
```

This means a user does not need mutation privileges to inspect ModelServices or their status.

The effective rule is:

```text
viewer OR deployer OR admin
```

---

## 5. List ModelServices

### Endpoint

```http
GET /api/v1/model-services
```

### Purpose

Returns ModelService resources from the managed namespace.

### Kubernetes mapping

Conceptually:

```text
REST GET collection
        |
        v
Kubernetes List
        |
        v
ModelServices in ai-platform
```

The API does not perform a cluster-wide list.

---

## 6. List Request Example

Obtain a fresh service token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Load it:

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

Call the API:

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

## 7. List Endpoint Security Properties

The list endpoint is intentionally constrained by:

```text
JWT authentication
role authorization
fixed namespace
Kubernetes ServiceAccount RBAC
NetworkPolicy
```

A valid viewer can list platform-managed ModelServices but does not gain arbitrary Kubernetes read access.

---

## 8. Get ModelService

### Endpoint

```http
GET /api/v1/model-services/{name}
```

Example:

```http
GET /api/v1/model-services/fraud-model
```

### Purpose

Returns a specific ModelService resource.

### Kubernetes mapping

```text
namespace = ai-platform
name      = {name from path}
```

The path variable identifies the resource name.

---

## 9. Get Request Example

```bash
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"

curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services/fraud-model

unset TOKEN
```

---

## 10. Get Not Found Behavior

If the resource does not exist, the expected API response class is:

```text
404 Not Found
```

A valid 404 is not counted as a service availability failure.

The availability model treats server-side `5xx` responses as service failures, not normal negative lookups.

---

## 11. Get Status Endpoint

### Endpoint

```http
GET /api/v1/model-services/{name}/status
```

Example:

```http
GET /api/v1/model-services/fraud-model/status
```

### Purpose

Returns the observed status of a ModelService.

The status is produced by the operator reconciliation loop.

---

## 12. Spec vs Status

The separation is:

```text
spec
  = desired state

status
  = observed / reconciled state
```

The REST API writes desired state through mutation endpoints.

The operator updates observed state.

The status endpoint exposes that observed state to API consumers.

---

## 13. Status Request Flow

```text
Client
  |
  | GET /api/v1/model-services/{name}/status
  v
Envoy
  |
  v
API authentication
  |
  v
API role authorization
  |
  v
Kubernetes get
  |
  v
ModelService.status
  |
  v
HTTP response
```

---

## 14. Why Status Has a Dedicated Endpoint

A dedicated status endpoint gives callers a platform-oriented way to inspect lifecycle state without requiring them to parse the entire Kubernetes object.

This is useful for workflows such as:

```text
create ModelService
     |
     v
poll status
     |
     v
observe readiness / failure / reconciliation result
```

---

## 15. Read Endpoint Error Classes

Expected classes include:

| Status | Meaning |
|---:|---|
| `200` | successful read |
| `401` | invalid/missing authentication |
| `403` | authenticated but unauthorized |
| `404` | ModelService not found |
| `5xx` | API/Kubernetes/backend failure |

A `404` is an expected resource-state response.

---

## 16. Edge vs Application Rejection

Read requests can be rejected before the Go API sees them.

Example:

```text
Client
  |
  v
Envoy SecurityPolicy
  |
  +--> reject 401/403
```

If the request is rejected at Envoy:

```text
no Go handler
no application-side request outcome
no application audit event
```

For accepted traffic, the Go API performs its own authentication/authorization as defense in depth.

---

## 17. Route Normalization

Read routes are normalized for metrics.

Collection route:

```text
/api/v1/model-services
```

Single resource:

```text
/api/v1/model-services/{name}
```

Status:

```text
/api/v1/model-services/{name}/status
```

Actual names such as:

```text
fraud-model
model-a
model-b
```

are not used as Prometheus route labels.

---

## 18. Why Route Normalization Matters

Without normalization:

```text
route="/api/v1/model-services/fraud-model"
route="/api/v1/model-services/model-a"
route="/api/v1/model-services/model-b"
```

would create unnecessary label cardinality.

The normalized route:

```text
/api/v1/model-services/{name}
```

keeps metrics bounded.

---

## 19. Prometheus Metrics for Reads

Read requests contribute to:

```text
ai_platform_api_http_requests_total
ai_platform_api_http_request_duration_seconds
ai_platform_api_http_requests_in_flight
```

Relevant labels include:

```text
method
route
status
```

For example:

```text
method="GET"
route="/api/v1/model-services"
status="200"
```

---

## 20. Grafana Read Traffic

The API dashboard includes:

```text
Request Rate by Route
Request Rate by Status
p95 Latency by Route
```

Read traffic was validated for:

```text
/api/v1/model-services
/api/v1/model-services/{name}
```

---

## 21. Example Traffic Generation

A validated traffic-generation pattern used:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

Collection requests:

```bash
for i in $(seq 1 100); do
  curl \
    --cacert .local/keycloak/fraud-model-root-ca.crt \
    -sS \
    -H "Authorization: Bearer ${TOKEN}" \
    https://api.ai-platform.local/api/v1/model-services \
    >/dev/null

  sleep 0.1
done
```

Single-resource requests:

```bash
for i in $(seq 1 50); do
  curl \
    --cacert .local/keycloak/fraud-model-root-ca.crt \
    -sS \
    -H "Authorization: Bearer ${TOKEN}" \
    https://api.ai-platform.local/api/v1/model-services/fraud-model \
    >/dev/null

  sleep 0.1
done
```

Then:

```bash
unset TOKEN
```

---

## 22. Raw Counter Validation

Query:

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api_http_requests_total{route!="/healthz",route!="/readyz"}' |
jq -r '
  .data.result[] | {
    route: .metric.route,
    status: .metric.status,
    value: .value[1]
  }
'
```

Observed examples included:

```text
/api/v1/model-services
status=200

/api/v1/model-services/{name}
status=200
```

---

## 23. Read Traffic and SLIs

Read traffic contributes to customer-facing:

```text
availability
error ratio
latency
request rate
```

Health and readiness probes are excluded.

This means real API reads are part of the service-quality signal while Kubernetes probes are not.

---

## 24. No Traffic Behavior

After a pod restart, process-local counters reset.

If only health/readiness requests occur:

```text
customer-facing read metrics may be empty
```

This is expected because those probe routes are excluded.

Fresh API traffic plus at least two Prometheus scrapes is needed before `rate(...[5m])` becomes useful again.

---

## 25. Kubernetes Comparison

Direct Kubernetes view:

```bash
kubectl get modelservices \
  -n ai-platform
```

API view:

```text
GET /api/v1/model-services
```

A focused resource comparison can use:

```bash
kubectl get modelservice \
  fraud-model \
  -n ai-platform \
  -o yaml
```

and:

```text
GET /api/v1/model-services/fraud-model
```

---

## 26. Read Handler Design

Read handlers are responsible for:

```text
extracting path data
calling Kubernetes abstraction
mapping result to API response
mapping errors to HTTP semantics
```

They do not perform reconciliation.

---

## 27. Read Authorization Design

Read access is deliberately broader than mutation access.

This supports operational users who need visibility without deployment/delete permissions.

Model:

```text
viewer
   -> observe

deployer
   -> observe + mutate

admin
   -> observe + mutate + delete
```

---

## 28. NetworkPolicy Consideration

External users reach read endpoints through Envoy.

Arbitrary pods are blocked from direct access to the API Service by NetworkPolicy.

Prometheus is separately allowed to scrape `/metrics`.

This preserves:

```text
external controlled access
internal least access
```

---

## 29. Read Endpoint Testing

The read API was validated through:

```text
Go tests
authenticated curl requests
CRUD E2E workflow
Prometheus traffic observation
Grafana visualization
```

The CRUD E2E workflow ultimately passed:

```text
PASS 20/20
```

---

## 30. Current Read Endpoint Status

```text
[✓] list endpoint implemented
[✓] get endpoint implemented
[✓] status endpoint implemented
[✓] JWT authentication enforced
[✓] viewer/deployer/admin read access
[✓] fixed namespace enforced
[✓] not-found behavior implemented
[✓] low-cardinality metrics
[✓] runtime traffic validated
[✓] dashboard visibility validated
```

The read endpoint portion of the API phase is complete.

---

## 31. Summary

The read API exposes a deliberately small contract:

```text
GET /api/v1/model-services
GET /api/v1/model-services/{name}
GET /api/v1/model-services/{name}/status
```

These routes:

```text
authenticate with Keycloak
authorize viewer/deployer/admin
operate only in ai-platform
use the API ServiceAccount for Kubernetes access
emit structured request metrics/logs
preserve low-cardinality route labels
```

The status endpoint completes the control-plane model by exposing observed state maintained by the operator.
