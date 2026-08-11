# AI Platform REST API

The **AI Platform REST API** is the authenticated platform-facing control plane used to manage `ModelService` resources without giving API consumers direct Kubernetes credentials.

It sits in front of the Kubernetes API and exposes a small, versioned REST contract for reading and managing `ModelService` objects in the platform namespace.

The API integrates:

```text
Keycloak / OIDC / JWT
Envoy Gateway / HTTPS
Go net/http
controller-runtime Kubernetes client
Kubernetes RBAC
NetworkPolicy
Vault PKI
Prometheus
Grafana
SLIs / SLOs / error-budget alerts
```

The implementation is designed around **least privilege**, **defense in depth**, **auditable mutations**, **low-cardinality observability**, and **operator-driven reconciliation**.

---

## REST API Phase Summary

The REST API phase added a complete platform API around the existing `ModelService` operator.

The high-level request path is:

```text
User / CLI / Automation
          |
          | OIDC / OAuth2
          v
       Keycloak
          |
          | JWT access token
          v
   Envoy Gateway
          |
          | HTTPS
          | JWT validation
          | role authorization
          v
AI Platform REST API
          |
          | request ID
          | authentication
          | authorization
          | validation
          | audit logging
          | Prometheus metrics
          v
   Kubernetes API
          |
          v
    ModelService CR
          |
          v
AI Platform Operator
          |
          v
 Serving Workload
```

The API does **not** replace the operator.

The API changes the desired `ModelService` state, while the operator remains responsible for reconciling that state into Kubernetes workloads and updating resource status.

---

## API Endpoint Summary

The API is exposed externally at:

```text
https://api.ai-platform.local
```

API version:

```text
/api/v1
```

Implemented endpoints:

| Method | Endpoint | Purpose | Minimum Role |
|---|---|---|---|
| `GET` | `/healthz` | Liveness | Internal/public application endpoint |
| `GET` | `/readyz` | Readiness | Internal/public application endpoint |
| `GET` | `/metrics` | Prometheus metrics | Internal monitoring |
| `GET` | `/api/v1/model-services` | List ModelServices | Viewer |
| `GET` | `/api/v1/model-services/{name}` | Get ModelService | Viewer |
| `GET` | `/api/v1/model-services/{name}/status` | Get observed status | Viewer |
| `POST` | `/api/v1/model-services` | Create ModelService | Deployer |
| `PUT` | `/api/v1/model-services/{name}` | Update ModelService | Deployer |
| `PATCH` | `/api/v1/model-services/{name}` | Partially update ModelService | Deployer |
| `DELETE` | `/api/v1/model-services/{name}` | Delete ModelService | Admin |

---

## Authorization Model

The API uses three platform roles:

| Role | Read | Create | Update | Patch | Delete |
|---|---:|---:|---:|---:|---:|
| `platform-viewer` | ✓ | — | — | — | — |
| `platform-deployer` | ✓ | ✓ | ✓ | ✓ | — |
| `platform-admin` | ✓ | ✓ | ✓ | ✓ | ✓ |

Authorization is enforced at several layers:

```text
Keycloak identity
      |
      v
Envoy SecurityPolicy
      |
      v
Go API authorization
      |
      v
fixed ai-platform namespace
      |
      v
Kubernetes ServiceAccount RBAC
```

This is intentional defense in depth.

---

## Key Implementation Highlights

The REST API phase includes:

- Go `net/http` API server
- versioned REST routes under `/api/v1`
- health and readiness endpoints
- configuration loading
- structured JSON logging using `slog`
- `X-Request-ID` correlation
- controller-runtime Kubernetes client
- fixed `ai-platform` namespace
- least-privilege Kubernetes ServiceAccount and RBAC
- Keycloak OIDC/JWT validation
- viewer/deployer/admin authorization
- complete ModelService CRUD
- request validation and error handling
- distroless non-root container
- Kubernetes Deployment and Service
- Vault-issued TLS certificate
- Envoy Gateway HTTPS exposure
- Envoy SecurityPolicy
- hardened NetworkPolicy
- runtime security context
- mutation audit logging
- Prometheus request metrics
- ServiceMonitor
- Grafana dashboard
- Prometheus alert rules
- request-based SLIs
- 99.9% availability SLO
- error-budget consumption
- fast- and slow-burn alerts
- unit/integration tests
- automated CRUD E2E validation
- deterministic `promtool` rule tests

---

## Documentation Structure

The REST API documentation is intentionally split into focused chapters so that architecture, implementation, security, observability, testing, and recovery can be read independently.

| Order | Document | Purpose |
|---:|---|---|
| 00 | [`00-overview-and-architecture.md`](00-overview-and-architecture.md) | Explains the complete REST API architecture, request flow, trust boundaries, Kubernetes/operator relationship, observability model, and major design decisions. |
| 01 | [`01-api-contract-and-project-structure.md`](01-api-contract-and-project-structure.md) | Defines the API contract, endpoint matrix, HTTP semantics, Go source structure, package responsibilities, routing, and middleware organization. |
| 02 | [`02-configuration-and-startup.md`](02-configuration-and-startup.md) | Documents startup flow, runtime configuration, OIDC initialization, CA trust, Kubernetes client construction, dependency checks, rollout behavior, and startup troubleshooting. |
| 03 | [`03-kubernetes-client-and-rbac.md`](03-kubernetes-client-and-rbac.md) | Covers the controller-runtime client, ModelService CRUD mapping, API ServiceAccount, namespace-scoped Role/RoleBinding, least privilege, and Kubernetes authorization checks. |
| 04 | [`04-read-endpoints.md`](04-read-endpoints.md) | Documents list, get, and status endpoints, read authorization, route normalization, Kubernetes reads, metrics, and runtime validation. |
| 05 | [`05-authentication-and-authorization.md`](05-authentication-and-authorization.md) | Explains Keycloak OIDC/JWT validation, issuer/audience, interactive and machine clients, viewer/deployer/admin roles, Envoy authorization, and defense in depth. |
| 06 | [`06-create-update-patch-delete.md`](06-create-update-patch-delete.md) | Documents POST, PUT, PATCH, and DELETE behavior, role restrictions, validation, Kubernetes mutations, operator reconciliation, and audit events. |
| 07 | [`07-validation-errors-and-namespace-restrictions.md`](07-validation-errors-and-namespace-restrictions.md) | Describes request validation, response/error semantics, authentication/authorization failures, fixed namespace enforcement, Kubernetes error translation, and SLO error classification. |
| 08 | [`08-container-and-kubernetes-deployment.md`](08-container-and-kubernetes-deployment.md) | Covers the container build, distroless runtime, non-root execution, Deployment, Service, probes, CA mount, kind image loading, Kustomize, and rollout validation. |
| 09 | [`09-envoy-gateway-and-https.md`](09-envoy-gateway-and-https.md) | Explains external HTTPS exposure through the shared Envoy Gateway, HTTPRoute, redirect route, Vault PKI certificate, SecurityPolicy, JWT enforcement, and Gateway troubleshooting. |
| 10 | [`10-networkpolicy-and-runtime-hardening.md`](10-networkpolicy-and-runtime-hardening.md) | Documents ingress/egress restrictions, Prometheus and Envoy access, direct-pod denial, the post-DNAT NetworkPolicy lesson, non-root execution, seccomp, capability drop, and read-only filesystem. |
| 11 | [`11-audit-logging-and-prometheus-metrics.md`](11-audit-logging-and-prometheus-metrics.md) | Covers structured request logging, request IDs, mutation audit events, audit outcomes, Prometheus counters/histograms/gauges, route normalization, cardinality control, and ServiceMonitor integration. |
| 12 | [`12-grafana-alerting-and-slo.md`](12-grafana-alerting-and-slo.md) | Documents the Grafana dashboard, Prometheus alerts, SLI recording rules, 99.9% availability SLO, error budget, fast/slow burn rates, and `promtool` validation. |
| 13 | [`13-testing-and-end-to-end-validation.md`](13-testing-and-end-to-end-validation.md) | Records unit/integration testing, CRUD E2E validation, role testing, runtime hardening checks, NetworkPolicy validation, metrics/Grafana checks, and Prometheus rule tests. |
| 14 | [`14-recovery-and-troubleshooting.md`](14-recovery-and-troubleshooting.md) | Provides symptom-based recovery procedures for startup, OIDC, RBAC, Gateway/TLS, NetworkPolicy, 401/403/404/5xx errors, metrics, Prometheus rules, and runtime failures. |
| 15 | [`15-complete-command-reference.md`](15-complete-command-reference.md) | Provides the complete operational command reference for build, deploy, auth, RBAC, API testing, Gateway, NetworkPolicy, Prometheus, SLOs, rule tests, and recovery. |

---

## Recommended Reading Paths

### New Engineer

```text
00 -> 01 -> 05 -> 03 -> 06 -> 09 -> 10 -> 11 -> 12 -> 13
```

This sequence starts with architecture, then moves through the API contract, authentication, Kubernetes access, CRUD, external exposure, hardening, observability, and testing.

### Operations / Troubleshooting

```text
14 -> 15 -> 10 -> 09 -> 12
```

This sequence starts with symptom-based recovery and then moves into the command reference and the most operationally relevant security/monitoring chapters.

### Project / Interview Review

```text
00 -> 01 -> 05 -> 10 -> 12 -> 13
```

This provides the clearest overview of architecture, API design, security, reliability, and validation.

---

## Observability Summary

The API exposes:

```text
ai_platform_api_http_requests_total
ai_platform_api_http_request_duration_seconds
ai_platform_api_http_requests_in_flight
```

Prometheus discovers the API through:

```text
ServiceMonitor/ai-platform-api
```

The Grafana dashboard is:

```text
AI Platform API Overview
```

and contains:

```text
API Target
API Requests/sec
5xx Error Rate
API p95 Latency
Requests In Flight
Request Rate by Route
Request Rate by Status
p95 Latency by Route
```

---

## SLI / SLO Summary

The API uses request-based availability.

Current engineering objective:

```text
Availability SLO: 99.9%
Allowed error ratio: 0.1%
p95 latency objective: < 500 ms
```

A request is treated as available when it returns a non-5xx response.

This intentionally means that valid:

```text
401
403
404
409
```

responses are not counted as service availability failures.

---

## Error-Budget Alerts

Fast burn:

```text
AIPlatformAPIErrorBudgetFastBurn

5m > 14.4x
AND
1h > 14.4x
for 2m
severity=critical
```

Slow burn:

```text
AIPlatformAPIErrorBudgetSlowBurn

1h > 6x
AND
6h > 6x
for 15m
severity=warning
```

---

## Validation Evidence

The REST API implementation was validated across application, Kubernetes, networking, security, and observability layers.

Key results:

```text
CRUD E2E
  PASS 20/20

Prometheus rule syntax
  SUCCESS: 17 rules found

Prometheus rule unit tests
  SUCCESS

Prometheus target
  up=1

Fast-burn alert
  inactive / health=ok

Slow-burn alert
  inactive / health=ok

NetworkPolicy
  arbitrary pod -> blocked
  Envoy        -> allowed
  Prometheus   -> allowed

Runtime hardening
  non-root
  UID/GID 65532
  allowPrivilegeEscalation=false
  drop ALL capabilities
  readOnlyRootFilesystem=true
  RuntimeDefault seccomp
```

---

## Repository Locations

Main Go entry point:

```text
cmd/platform-api/main.go
```

API source:

```text
internal/api/
```

Deployment manifests:

```text
config/platform-api/
```

Container build:

```text
Dockerfile.platform-api
```

Monitoring:

```text
infrastructure/monitoring/
```

CRUD E2E script:

```text
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

Keycloak helpers:

```text
infrastructure/keycloak/scripts/get-machine-token.sh
infrastructure/keycloak/scripts/pkce-login.py
```

---

## REST API Phase Completion

The implementation checklist for this phase is:

```text
[✓] API architecture defined
[✓] API contract and endpoints defined
[✓] Go API project structure created
[✓] Health and readiness endpoints added
[✓] Configuration loading added
[✓] Structured logging added
[✓] Kubernetes client added
[✓] Least-privilege API ServiceAccount and RBAC added
[✓] List ModelServices endpoint added
[✓] Get ModelService endpoint added
[✓] Get ModelService status endpoint added
[✓] Keycloak JWT validation added
[✓] API role-based authorization added
[✓] Create ModelService endpoint added
[✓] Update ModelService endpoint added
[✓] Patch ModelService endpoint added
[✓] Delete ModelService endpoint added
[✓] Request validation and error handling added
[✓] Namespace restrictions added
[✓] Container image and Kubernetes manifests added
[✓] API exposed through Envoy Gateway HTTPS
[✓] Envoy SecurityPolicy attached to API route
[✓] NetworkPolicy and Pod security added
[✓] Audit logging and Prometheus metrics added
[✓] Unit and integration tests added
[✓] End-to-end API workflow validated
[✓] Manifests and scripts stored in Git
[✓] Recovery and troubleshooting documented
[✓] REST API documentation completed
[✓] AI Platform REST API phase complete
```

---

## Final Result

The completed API phase provides a production-style platform control-plane path:

```text
authenticated client
      |
      v
HTTPS / Envoy
      |
      v
JWT + role authorization
      |
      v
Go REST API
      |
      v
validated ModelService operation
      |
      v
Kubernetes API
      |
      v
AI Platform Operator
```

with:

```text
least privilege
network isolation
runtime hardening
structured audit logging
Prometheus metrics
Grafana dashboards
SLIs
SLOs
error-budget burn alerts
automated tests
recovery documentation
```

This documentation set is the permanent technical record for the AI Platform REST API phase.
