# AI Platform REST API Documentation

This directory contains the complete documentation set for the AI Platform REST API phase of the `ai-platform-operator` project.

The documentation records the implementation, security model, Kubernetes integration, external exposure, observability, testing, troubleshooting, and operational commands built during the API phase.

---

## Documentation Index

### 00 — Overview and Architecture

```text
00-overview-and-architecture.md
```

Start here.

Covers:

- purpose of the REST API
- high-level architecture
- Keycloak / Envoy / API / Kubernetes / Operator flow
- major implementation decisions
- observability architecture
- final implementation status

---

### 01 — API Contract and Project Structure

```text
01-api-contract-and-project-structure.md
```

Covers:

- `/api/v1` contract
- endpoint matrix
- role requirements
- HTTP semantics
- Go source structure
- package responsibilities
- routing and middleware organization

---

### 02 — Configuration and Startup

```text
02-configuration-and-startup.md
```

Covers:

- `cmd/platform-api/main.go`
- runtime configuration
- OIDC initialization
- CA trust
- Kubernetes client startup
- server/middleware construction
- startup dependencies
- rollout validation

---

### 03 — Kubernetes Client and RBAC

```text
03-kubernetes-client-and-rbac.md
```

Covers:

- controller-runtime client
- ModelService CRUD mapping
- `ServiceAccount/ai-platform-api`
- Role / RoleBinding
- namespace scoping
- least privilege
- Kubernetes API access
- `kubectl auth can-i`

---

### 04 — Read Endpoints

```text
04-read-endpoints.md
```

Covers:

- list ModelServices
- get ModelService
- get ModelService status
- read authorization
- route normalization
- read metrics
- runtime examples

---

### 05 — Authentication and Authorization

```text
05-authentication-and-authorization.md
```

Covers:

- Keycloak OIDC
- JWT validation
- issuer/audience
- `ai-platform-cli`
- `ai-platform-service`
- viewer/deployer/admin roles
- Envoy SecurityPolicy
- application authorization
- edge vs application rejection

---

### 06 — Create, Update, Patch, Delete

```text
06-create-update-patch-delete.md
```

Covers:

- POST
- PUT
- PATCH
- DELETE
- role restrictions
- audit events
- operator reconciliation
- admin DELETE validation
- CRUD E2E results

---

### 07 — Validation, Errors, and Namespace Restrictions

```text
07-validation-errors-and-namespace-restrictions.md
```

Covers:

- input validation
- response semantics
- 400/401/403/404/409/5xx
- fixed `ai-platform` namespace
- SLO interpretation of status codes
- Kubernetes error translation

---

### 08 — Container and Kubernetes Deployment

```text
08-container-and-kubernetes-deployment.md
```

Covers:

- `Dockerfile.platform-api`
- `golang:1.26` builder
- distroless runtime
- UID/GID 65532
- probes
- Service
- Deployment
- CA mount
- kind image loading
- rollout workflow

---

### 09 — Envoy Gateway and HTTPS

```text
09-envoy-gateway-and-https.md
```

Covers:

- `api.ai-platform.local`
- shared Gateway
- HTTPRoute
- HTTP redirect
- Vault-issued TLS
- Envoy SecurityPolicy
- edge JWT/roles
- Gateway troubleshooting

---

### 10 — NetworkPolicy and Runtime Hardening

```text
10-networkpolicy-and-runtime-hardening.md
```

Covers:

- Envoy-only ingress
- Prometheus ingress
- arbitrary pod denial
- DNS/Kubernetes/OIDC egress
- post-DNAT NetworkPolicy lesson
- non-root runtime
- read-only root filesystem
- capability drop
- seccomp

---

### 11 — Audit Logging and Prometheus Metrics

```text
11-audit-logging-and-prometheus-metrics.md
```

Covers:

- structured `slog` logging
- `X-Request-ID`
- mutation audit events
- audit outcomes
- request counters
- latency histogram
- in-flight requests
- route normalization
- ServiceMonitor

---

### 12 — Grafana, Alerting, and SLO

```text
12-grafana-alerting-and-slo.md
```

Covers:

- Grafana dashboard
- Prometheus alert rules
- availability alert
- 5xx alert
- p95 alert
- SLI recording rules
- 99.9% availability SLO
- error-budget consumption
- fast/slow burn alerts
- `promtool` validation

---

### 13 — Testing and End-to-End Validation

```text
13-testing-and-end-to-end-validation.md
```

Covers:

- Go tests
- metrics/audit tests
- authentication/authorization tests
- CRUD E2E
- NetworkPolicy validation
- runtime hardening validation
- Grafana validation
- Prometheus rule validation
- completion evidence

---

### 14 — Recovery and Troubleshooting

```text
14-recovery-and-troubleshooting.md
```

Use this during failures.

Covers:

- pod startup failures
- OIDC problems
- RBAC
- NetworkPolicy
- Gateway/TLS
- 401/403/404/5xx
- metrics `No data`
- missing Prometheus rules
- alert problems
- recovery workflows

---

### 15 — Complete Command Reference

```text
15-complete-command-reference.md
```

Quick operational command index for:

```text
build
deploy
auth
RBAC
API
Gateway
NetworkPolicy
Prometheus
Grafana
SLIs/SLOs
rule tests
recovery
```

---

# REST API Implementation Status

The implementation checklist is now:

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
[✓] REST API documentation set created
```

After these files are copied into the repository and committed, the documentation requirement for the REST API phase is complete.

---

# Key Completion Evidence

```text
CRUD E2E
  PASS 20/20

Prometheus syntax
  SUCCESS: 17 rules found

Prometheus rule tests
  SUCCESS

API burn alerts
  FastBurn -> inactive / health=ok
  SlowBurn -> inactive / health=ok

Prometheus target
  up=1

Runtime hardening
  non-root
  UID/GID 65532
  no privilege escalation
  drop ALL
  read-only root filesystem
  RuntimeDefault seccomp

NetworkPolicy
  arbitrary pod blocked
  Envoy allowed
  Prometheus allowed
```

---

# Suggested Repository Location

Copy this documentation set to:

```text
docs/rest-api/
```

Final structure:

```text
docs/rest-api/
├── 00-overview-and-architecture.md
├── 01-api-contract-and-project-structure.md
├── 02-configuration-and-startup.md
├── 03-kubernetes-client-and-rbac.md
├── 04-read-endpoints.md
├── 05-authentication-and-authorization.md
├── 06-create-update-patch-delete.md
├── 07-validation-errors-and-namespace-restrictions.md
├── 08-container-and-kubernetes-deployment.md
├── 09-envoy-gateway-and-https.md
├── 10-networkpolicy-and-runtime-hardening.md
├── 11-audit-logging-and-prometheus-metrics.md
├── 12-grafana-alerting-and-slo.md
├── 13-testing-and-end-to-end-validation.md
├── 14-recovery-and-troubleshooting.md
├── 15-complete-command-reference.md
└── README.md
```

---

# Architecture in One View

```text
User / CLI / Automation
          |
          | OIDC/OAuth2
          v
       Keycloak
          |
          | JWT
          v
   Envoy Gateway
          |
          | TLS
          | JWT
          | role policy
          v
AI Platform REST API
          |
          | auth
          | authorization
          | validation
          | audit
          | metrics
          v
   Kubernetes API
          |
          v
    ModelService
          |
          v
AI Platform Operator
          |
          v
 Serving Workload
```

Observability:

```text
API /metrics
    |
    v
ServiceMonitor
    |
    v
Prometheus
  |     |
  |     +--> alerts
  |     +--> SLIs/SLO
  |     +--> error budget
  |     +--> burn rate
  |
  v
Grafana
```

---

# Recommended Reading Order

For a new engineer:

```text
00
01
05
03
06
09
10
11
12
13
14
15
```

For operations/troubleshooting:

```text
14
15
12
10
09
```

For interview/project review:

```text
00
01
05
10
12
13
```

---

# Phase Completion

The REST API implementation itself is complete.

The documentation set captured here closes the remaining documentation gap.

Once these documents are copied into `docs/rest-api/` and committed, the checklist item can be marked:

```text
[✓] AI Platform REST API phase complete
```
