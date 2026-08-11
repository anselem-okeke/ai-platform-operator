# AI Platform REST API — Configuration and Startup

## 1. Purpose

This document explains how the AI Platform REST API is configured and started.

It records:

- the startup flow
- configuration responsibilities
- OIDC configuration
- Kubernetes client initialization
- logging initialization
- middleware/server construction
- readiness dependencies
- certificate trust requirements
- NetworkPolicy implications
- deployment/runtime configuration
- troubleshooting considerations discovered during implementation

This document complements:

```text
docs/rest-api/00-overview-and-architecture.md
docs/rest-api/01-api-contract-and-project-structure.md
```

The first document describes the overall architecture. The second describes the API contract and code layout. This document focuses on how the API process is configured and brought into a ready state.

---

# 2. Main Entry Point

The REST API executable starts from:

```text
cmd/platform-api/main.go
```

This is the composition root of the service.

Its job is not to implement endpoint logic directly.

Instead, it wires together the components required by the API:

```text
configuration
logging
OIDC authentication
Kubernetes client
API routes
middleware
HTTP server
```

Conceptually:

```text
main()
  |
  +--> load configuration
  |
  +--> initialize structured logger
  |
  +--> initialize Kubernetes client
  |
  +--> initialize OIDC verifier
  |
  +--> create handlers / stores
  |
  +--> create routes
  |
  +--> wrap routes with middleware
  |
  +--> start HTTP server
```

---

# 3. Startup Design Principle

The API follows an explicit startup model.

Dependencies that are required for correct API operation are initialized before the server is considered ready.

This is important because the API depends on more than the local Go process.

External/internal dependencies include:

```text
Kubernetes API
Keycloak OIDC discovery/JWKS
trusted CA material
network access to required endpoints
```

A running process is therefore not automatically equivalent to a ready API.

This distinction is why the deployment exposes both:

```text
/healthz
/readyz
```

---

# 4. Configuration Package

Runtime configuration is handled under:

```text
internal/api/config/
```

The configuration layer keeps environment-specific settings out of request handlers.

Its role is to provide the values needed by startup and dependency construction.

The API configuration includes settings related to:

- HTTP server behavior
- managed Kubernetes namespace
- OIDC issuer
- OIDC audience
- CA/trust configuration
- Kubernetes access
- logging/runtime behavior

The detailed internal field names should be read from the source code if exact field-by-field values are needed, but the architectural responsibilities are stable.

---

# 5. Managed Namespace

The API is intentionally restricted to:

```text
ai-platform
```

This namespace is part of the service configuration and security boundary.

The API does not expose arbitrary namespace selection to callers.

The startup/configuration model therefore assumes one platform-managed namespace for ModelService operations.

This influences:

```text
RBAC
Kubernetes client operations
validation
audit behavior
API semantics
```

---

# 6. OIDC Configuration

The API uses Keycloak for authentication.

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

The API must be able to trust and reach the identity provider during startup.

Authentication support is implemented under:

```text
internal/api/auth/
```

and initialized from the startup path.

---

# 7. OIDC Discovery

The API performs OIDC discovery during initialization.

This means startup requires successful connectivity to the configured identity provider path.

The OIDC verifier needs enough information to validate:

```text
issuer
signing keys
audience
token expiry/signature
```

The internal JWKS endpoint used in the cluster is:

```text
http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
```

The externally visible issuer remains:

```text
https://auth.ai-platform.local/realms/ai-platform
```

This separation between issuer identity and internal reachability is operationally important.

---

# 8. Trust Material

The API requires the correct CA trust for OIDC/TLS communication.

A Kubernetes ConfigMap is used for API OIDC CA trust:

```text
ai-platform-api-oidc-ca
```

This trust material is mounted into the API Deployment.

The API startup environment must therefore have:

```text
correct issuer
correct trust chain
correct mounted CA
correct network path
```

If any of those are wrong, OIDC initialization can fail before the API reaches readiness.

---

# 9. Why OIDC Startup Was Operationally Important

During NetworkPolicy hardening, the API initially failed after a more restrictive egress policy was applied.

The problem was not the Go HTTP server itself.

The API needed to perform OIDC discovery during startup, but the egress path to the identity provider was blocked.

The important lesson is:

```text
API startup dependencies must be reflected in NetworkPolicy
```

A policy that permits only Kubernetes API and DNS access is not sufficient when the service also performs OIDC discovery.

---

# 10. OIDC Traffic Path and Envoy

The identity-provider path involved Envoy networking.

A key troubleshooting finding was that the CNI observed traffic after destination NAT rather than only as traffic to the logical load-balancer IP.

The initial policy allowed the logical external/LB destination but did not match the packet path actually seen by the CNI.

The fix used namespace and pod selectors for the Envoy proxy target.

Relevant Envoy proxy labels include:

```text
app.kubernetes.io/component=proxy
app.kubernetes.io/name=envoy
gateway.envoyproxy.io/owning-gateway-name=shared-gateway
gateway.envoyproxy.io/owning-gateway-namespace=gateway-system
```

The relevant Envoy proxy target port is:

```text
10443
```

This is a critical startup/troubleshooting detail.

---

# 11. Kubernetes Client Initialization

The API uses the controller-runtime Kubernetes client.

Kubernetes integration is implemented under:

```text
internal/api/kubernetes/
```

At startup, the process initializes the client needed to manage:

```text
platform.anselem.dev/v1alpha1 ModelService
```

The API runs inside Kubernetes and uses its ServiceAccount identity.

ServiceAccount:

```text
ai-platform-api
```

The ServiceAccount token is intentionally mounted because the API must authenticate to the Kubernetes API.

---

# 12. Kubernetes Authentication

The API pod uses Kubernetes in-cluster credentials.

This means the Deployment retains:

```text
automountServiceAccountToken: true
```

This is required functionality.

The security boundary is provided by least-privilege RBAC rather than by removing the token.

The effective chain is:

```text
API pod
  |
  | mounted ServiceAccount credentials
  v
Kubernetes API
  |
  | Role / RoleBinding
  v
allowed ModelService operations
```

---

# 13. Kubernetes RBAC Startup Dependency

The API can start as a process even if RBAC is wrong, but requests that require Kubernetes access will fail.

Therefore Kubernetes connectivity and authorization must both be considered during readiness and troubleshooting.

Relevant resources include:

```text
ServiceAccount/ai-platform-api
Role/ai-platform-api
RoleBinding/ai-platform-api
```

in:

```text
ai-platform
```

The API is not given cluster-admin privileges.

---

# 14. Kubernetes API Network Access

The API requires egress to the Kubernetes API.

Relevant cluster service:

```text
10.96.0.1:443
```

The kind control-plane endpoint used in the lab is:

```text
172.19.0.7:6443
```

The NetworkPolicy must permit the path actually used by the API.

Blocking Kubernetes API egress would break CRUD operations even if the process remained healthy.

---

# 15. DNS Dependency

The API requires DNS for service discovery.

CoreDNS Service:

```text
10.96.0.10
```

The API NetworkPolicy permits DNS egress over:

```text
UDP 53
TCP 53
```

This is required for resolving internal service names such as Keycloak and other Kubernetes endpoints.

DNS must therefore be checked early when startup fails unexpectedly.

---

# 16. Structured Logging Initialization

Structured logging is initialized before serving requests.

Logging support is under:

```text
internal/api/logging/
```

The implementation uses:

```text
log/slog
```

and emits structured JSON logs.

This is important during startup because dependency failures should produce machine-readable diagnostic output rather than only opaque text.

---

# 17. Logging Goals

The logging model is designed to provide:

```text
startup diagnostics
request diagnostics
audit evidence
correlation by request ID
machine-readable logs
```

The API avoids logging JWT access tokens.

Startup logs should be treated as one of the primary sources of evidence when readiness fails.

---

# 18. HTTP Server Construction

Server construction is implemented under:

```text
internal/api/server.go
```

Routing is defined in:

```text
internal/api/routes.go
```

The startup flow constructs the HTTP handler tree and then wraps it with cross-cutting middleware.

The conceptual relationship is:

```text
main
 |
 v
server
 |
 v
middleware
 |
 v
routes
 |
 v
handlers
```

---

# 19. Public/Internal Operational Routes

Operational routes include:

```text
/healthz
/readyz
/metrics
```

These routes are not wrapped by the same application authentication chain as:

```text
/api/v1/
```

This allows Kubernetes probes and Prometheus to access the internal application directly.

However, external Envoy policy attachment may still require JWT before an external request reaches those endpoints.

Therefore:

```text
Go-level public
```

does not necessarily mean:

```text
externally unauthenticated
```

---

# 20. Protected Route Startup

Protected routes are registered under:

```text
/api/v1/
```

They use application authentication and role authorization.

The protected flow is:

```text
Authentication
  ->
AuditLogging
  ->
Protected router
  ->
RequireAnyRole
  ->
Handler
```

If OIDC verifier initialization fails, the protected API cannot operate correctly.

---

# 21. Outer Middleware

The main middleware chain is:

```text
RequestID
  ->
RequestLogging
  ->
RequestMetrics
  ->
ServeMux
```

This means every incoming request can receive correlation, logging, and metrics behavior before route-specific logic runs.

The ordering was deliberately chosen so final response status can be captured.

---

# 22. Request ID Startup Consideration

Request ID middleware requires no external dependency.

It uses:

```text
X-Request-ID
```

to propagate or establish correlation.

This middleware should remain available even when downstream handlers return errors.

That makes startup/runtime failure requests easier to trace.

---

# 23. Response Status Capture

The logging and metrics middleware use response recording so they can observe the final HTTP status written by downstream handlers.

Without this, middleware could incorrectly assume:

```text
200
```

when a handler actually returned:

```text
400
401
403
404
500
```

This matters for both logs and Prometheus metrics.

---

# 24. Metrics Initialization

Prometheus collectors are defined under:

```text
internal/api/metrics/
```

The API registers collectors for:

```text
ai_platform_api_http_requests_total
ai_platform_api_http_request_duration_seconds
ai_platform_api_http_requests_in_flight
```

Metrics initialization must happen before serving requests so the registry and middleware observe the complete process lifecycle.

---

# 25. `/metrics` Startup Behavior

The metrics endpoint is made available through the API server.

The request metrics middleware skips:

```text
/metrics
```

to prevent self-observation noise.

Prometheus scrapes this path through the internal Kubernetes Service.

---

# 26. Health vs Readiness

The deployment distinguishes process liveness from readiness.

Conceptually:

```text
healthz
  = process is alive

readyz
  = process is ready to serve
```

This distinction is important because a process may exist while one of its required dependencies is not usable.

The exact readiness implementation is defined in the Go source, but operationally these endpoints must be treated as different signals.

---

# 27. Deployment

The API Kubernetes Deployment is defined under:

```text
config/platform-api/
```

The Deployment includes:

- API container
- ServiceAccount
- probes
- resource requests/limits
- OIDC CA mount
- hardened security context
- image configuration

Image:

```text
ai-platform-api:dev
```

Development policy:

```text
imagePullPolicy: Never
```

---

# 28. Container Build

Build file:

```text
Dockerfile.platform-api
```

Builder:

```text
golang:1.26
```

Runtime:

```text
distroless static:nonroot
```

Runtime user:

```text
65532:65532
```

This keeps the runtime image small and removes shell/package-manager dependencies.

---

# 29. Standard Build and Rollout Flow

From the repository root:

```bash
docker build   -f Dockerfile.platform-api   -t ai-platform-api:dev .
```

Load into kind:

```bash
kind load docker-image   ai-platform-api:dev   --name ai-platform-policy
```

Restart deployment:

```bash
kubectl rollout restart   deployment/ai-platform-api   -n ai-platform
```

Wait for rollout:

```bash
kubectl rollout status   deployment/ai-platform-api   -n ai-platform   --timeout=180s
```

---

# 30. Pod Validation

Check API pod state:

```bash
kubectl get pods   -n ai-platform   -l app.kubernetes.io/name=ai-platform-api   -o wide
```

Expected healthy state:

```text
READY 1/1
STATUS Running
RESTARTS 0
```

A running pod alone is not sufficient evidence that OIDC, Kubernetes access, Gateway routing, and Prometheus scraping all work.

---

# 31. Deployment Security Context

Pod-level settings:

```text
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
seccompProfile: RuntimeDefault
```

Container-level settings:

```text
allowPrivilegeEscalation: false
capabilities:
  drop:
    - ALL
readOnlyRootFilesystem: true
```

These settings must remain compatible with startup.

The API must not depend on writing arbitrary files into the container root filesystem.

---

# 32. OIDC CA Mount

The OIDC CA ConfigMap is:

```text
ai-platform-api-oidc-ca
```

It is mounted into the API container so the API can validate the expected TLS trust path.

A missing or incorrectly mounted CA can manifest as OIDC startup failure.

Troubleshooting should therefore inspect both:

```text
ConfigMap contents
Deployment volume
Deployment volumeMount
```

---

# 33. Service

The API is exposed internally through a Kubernetes Service.

Service name:

```text
ai-platform-api
```

Namespace:

```text
ai-platform
```

Application port:

```text
8080
```

Envoy and Prometheus connect to the API Service/pods according to their respective network paths.

---

# 34. External Gateway Path

The external API endpoint is:

```text
https://api.ai-platform.local
```

Traffic reaches the API through the shared Envoy Gateway:

```text
gateway-system/shared-gateway
```

Flow:

```text
api.ai-platform.local
   |
   v
shared Gateway
   |
   v
HTTPRoute
   |
   v
Envoy SecurityPolicy
   |
   v
ai-platform-api Service
   |
   v
API pod
```

The API can be internally healthy even if this external path is broken.

Therefore startup troubleshooting and external-access troubleshooting must be separated.

---

# 35. API TLS

API certificate:

```text
api-ai-platform-local
```

TLS secret:

```text
api-ai-platform-local-tls
```

Namespace:

```text
gateway-system
```

The certificate is issued from the Vault PKI integration.

TLS termination occurs at Envoy Gateway rather than inside the Go API container.

Therefore the API process itself listens on internal HTTP while external users receive HTTPS.

---

# 36. NetworkPolicy and Startup

NetworkPolicy is one of the most important startup dependencies.

The API needs egress to:

```text
DNS
Kubernetes API
OIDC/Envoy path
```

Ingress must permit:

```text
Envoy -> API :8080
Prometheus -> API :8080
```

An over-restrictive policy can produce a pod that repeatedly fails readiness/startup despite correct application code.

---

# 37. Direct Access Is Intentionally Blocked

A validation pod in the same namespace was used to test direct API access.

Command pattern:

```bash
kubectl run api-network-test   -n ai-platform   --image=curlimages/curl:8.12.1   --restart=Never   --rm -i   --   curl   --connect-timeout 5   -sS   -o /dev/null   -w '%{http_code}\n'   http://ai-platform-api.ai-platform.svc.cluster.local:8080/healthz
```

Observed result:

```text
000
```

with a connection timeout.

This is expected under the hardened NetworkPolicy.

---

# 38. Prometheus Must Still Reach the API

Even while arbitrary pods are blocked, Prometheus must reach:

```text
/metrics
```

The NetworkPolicy therefore allows Prometheus based on namespace and pod labels.

Prometheus healthy scrape:

```promql
up{
  job="ai-platform-api",
  namespace="ai-platform"
}
```

Expected:

```text
1
```

This is an important post-startup validation.

---

# 39. Startup Verification Sequence

After a build or configuration change, use the following order.

### Step 1 — Build

```bash
docker build   -f Dockerfile.platform-api   -t ai-platform-api:dev .
```

### Step 2 — Load image

```bash
kind load docker-image   ai-platform-api:dev   --name ai-platform-policy
```

### Step 3 — Restart

```bash
kubectl rollout restart   deployment/ai-platform-api   -n ai-platform
```

### Step 4 — Wait for rollout

```bash
kubectl rollout status   deployment/ai-platform-api   -n ai-platform   --timeout=180s
```

### Step 5 — Check pod

```bash
kubectl get pods   -n ai-platform   -l app.kubernetes.io/name=ai-platform-api   -o wide
```

### Step 6 — Check logs

```bash
kubectl logs   -n ai-platform   deployment/ai-platform-api   --tail=200
```

### Step 7 — Validate external API

Use a fresh token and HTTPS request.

### Step 8 — Validate Prometheus target

Check:

```promql
up{job="ai-platform-api",namespace="ai-platform"}
```

---

# 40. Machine Authentication Startup Validation

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

Validate API:

```bash
curl   --cacert .local/keycloak/fraud-model-root-ca.crt   -sS   -H "Authorization: Bearer ${TOKEN}"   https://api.ai-platform.local/api/v1/model-services
```

Then:

```bash
unset TOKEN
```

This verifies more than process startup.

It proves:

```text
Keycloak
TLS
Envoy
JWT validation
API routing
Kubernetes read access
```

work together.

---

# 41. Interactive Admin Validation

For admin-level validation:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Load token:

```bash
ADMIN_TOKEN="$(
  cat .local/keycloak/tokens/user-access-token.jwt
)"
```

This is useful for validating operations such as DELETE that the machine/deployer identity cannot perform.

---

# 42. Common Startup Failure Categories

A startup/readiness problem usually falls into one of these categories:

```text
configuration
CA/trust
OIDC connectivity
DNS
NetworkPolicy
Kubernetes API connectivity
Kubernetes RBAC
image/build
probe configuration
runtime security
```

Troubleshooting should isolate the category rather than changing multiple layers at once.

---

# 43. Failure: OIDC Discovery Cannot Connect

Symptoms may include:

```text
pod restart
readiness failure
OIDC discovery error
TLS/connectivity error
```

Check:

```bash
kubectl logs   -n ai-platform   deployment/ai-platform-api   --tail=200
```

Then verify:

```text
OIDC issuer
OIDC CA mount
DNS
NetworkPolicy egress
Envoy proxy destination
```

The previous hardening incident showed that NetworkPolicy may need to match the post-DNAT proxy path.

---

# 44. Failure: Kubernetes Requests Return Errors

If the API starts but CRUD/read operations fail, check:

```text
ServiceAccount
Role
RoleBinding
Kubernetes API egress
namespace
CRD availability
```

Useful checks:

```bash
kubectl get serviceaccount   ai-platform-api   -n ai-platform
```

```bash
kubectl get role   ai-platform-api   -n ai-platform   -o yaml
```

```bash
kubectl get rolebinding   ai-platform-api   -n ai-platform   -o yaml
```

---

# 45. Failure: External HTTPS Does Not Work

If the pod is healthy but:

```text
https://api.ai-platform.local
```

fails, isolate the external path.

Check:

```text
Gateway
HTTPRoute
SecurityPolicy
TLS certificate/secret
Envoy Service
DNS/hosts resolution
JWT
```

Do not immediately modify the Go API.

A healthy internal API plus failed external access usually indicates Gateway/security/TLS routing rather than startup code.

---

# 46. Failure: Prometheus Target Is Down

If the API works externally but Prometheus shows:

```text
up = 0
```

check:

```text
ServiceMonitor
Service labels
Service port name
NetworkPolicy ingress from monitoring
API /metrics
Prometheus target discovery
```

The ServiceMonitor uses:

```text
port: http
path: /metrics
interval: 30s
```

---

# 47. Failure: Metrics Show No Customer Data

A newly started API process resets process-local counters.

Health/readiness traffic is excluded from customer SLIs.

Therefore immediately after restart:

```text
customer metrics may be empty
```

Generate customer traffic and allow at least two Prometheus scrapes before concluding metrics are broken.

With a 30-second scrape interval, wait approximately:

```text
60-90 seconds
```

after generating traffic.

---

# 48. Configuration Change Safety

Before applying Kubernetes configuration changes:

```bash
kubectl apply   --dry-run=server   -k config/platform-api
```

Then apply:

```bash
kubectl apply   -k config/platform-api
```

This is particularly useful for:

```text
Deployment changes
NetworkPolicy
ServiceMonitor
PrometheusRule
HTTPRoute
SecurityPolicy
RBAC
```

---

# 49. Kustomize Rendering

To inspect the complete rendered API configuration:

```bash
kubectl kustomize   config/platform-api   >/tmp/platform-api.yaml
```

Then inspect the relevant resource before applying.

Examples:

```bash
grep -A30   'kind: Deployment'   /tmp/platform-api.yaml
```

or:

```bash
grep -A30   'alert: AIPlatformAPIDown'   /tmp/platform-api.yaml
```

This is useful when source manifests and live behavior appear inconsistent.

---

# 50. Prometheus Rule Startup Validation

Prometheus rule syntax is validated with the same Prometheus version running in the cluster.

Image:

```text
quay.io/prometheus/prometheus:v3.13.2-distroless
```

Determine it dynamically:

```bash
PROM_IMAGE="$(
  kubectl get pod     -n monitoring     prometheus-kps-kube-prometheus-stack-prometheus-0     -o jsonpath='{.spec.containers[?(@.name=="prometheus")].image}'
)"
```

Check:

```bash
docker run --rm   -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro"   --entrypoint /bin/promtool   "${PROM_IMAGE}"   check rules   /rules/ai-platform-api.rules.yaml
```

Final validated result:

```text
SUCCESS: 17 rules found
```

---

# 51. Prometheus Rule Unit Test

Run:

```bash
docker run --rm   -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro"   -w /rules   --entrypoint /bin/promtool   "${PROM_IMAGE}"   test rules   ai-platform-api.test.yaml
```

Final validated result:

```text
SUCCESS
```

This should be run before deploying alert/SLI rule changes.

---

# 52. Configuration Drift Consideration

Prometheus test rules are stored in:

```text
infrastructure/monitoring/rule-tests/ai-platform-api.rules.yaml
```

Production rules are stored in:

```text
config/platform-api/prometheusrule.yaml
```

Because these are separate representations, they can drift.

This happened during the availability alert label-preservation fix when the test copy used:

```promql
max by (job, namespace)
```

before the production manifest was updated.

Therefore every rule change should verify both files.

---

# 53. PrometheusRule Selection Label

The production Prometheus resource uses a selector requiring:

```yaml
release: kps
```

The API PrometheusRule therefore requires that label.

Without it, the resource can exist in Kubernetes but not be loaded by Prometheus.

This distinction is important:

```text
kubectl get prometheusrule
```

proves the CR exists.

It does **not** prove Prometheus loaded it.

Always verify through:

```text
/api/v1/rules
```

as well.

---

# 54. Startup Validation Is Layered

A complete startup validation should prove all of these independently:

```text
process alive
pod ready
Kubernetes client works
OIDC works
external HTTPS works
authorization works
Prometheus scrape works
rules are healthy
```

No single command proves all of them.

A useful model is:

```text
Layer 1  process
Layer 2  Kubernetes pod
Layer 3  dependencies
Layer 4  API contract
Layer 5  Gateway/TLS
Layer 6  observability
```

---

# 55. Recommended Post-Rollout Checks

After each API rollout:

```bash
kubectl rollout status   deployment/ai-platform-api   -n ai-platform   --timeout=180s
```

Then:

```bash
kubectl get pods   -n ai-platform   -l app.kubernetes.io/name=ai-platform-api
```

Then inspect recent logs:

```bash
kubectl logs   -n ai-platform   deployment/ai-platform-api   --since=5m
```

Then validate an authenticated read.

Then verify Prometheus:

```promql
max(
  up{
    job="ai-platform-api",
    namespace="ai-platform"
  }
)
```

Expected:

```text
1
```

---

# 56. Startup Security Model

The API startup process must preserve the following security properties:

```text
non-root execution
least-privilege Kubernetes identity
restricted network access
trusted OIDC CA
no embedded static admin credentials
JWT validation
read-only root filesystem
```

Startup fixes should not bypass these controls simply to make the pod run.

For example:

```text
disabling NetworkPolicy
granting cluster-admin
running as root
disabling JWT verification
```

would solve symptoms by removing security boundaries and are therefore not acceptable final fixes.

---

# 57. Configuration Ownership

Configuration belongs in declarative Git-managed files wherever possible.

Examples:

```text
config/platform-api/
infrastructure/monitoring/
infrastructure/keycloak/
```

Runtime-generated sensitive values such as access tokens remain outside Git under:

```text
.local/
```

This separation is intentional.

---

# 58. Secrets and Tokens

Access tokens are short lived.

Keycloak tokens used during testing expire after approximately:

```text
300 seconds
```

Therefore startup/functional validation should obtain a fresh token before testing.

Machine flow:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Do not commit:

```text
.local/keycloak/tokens/
```

or copy token values into documentation.

---

# 59. Readiness of Dependencies

Operationally, readiness depends on the application being able to fulfill its contract.

The important dependency set is:

```text
configuration valid
OIDC trust valid
OIDC discovery reachable
Kubernetes credentials available
Kubernetes API reachable
required CRD available
server successfully initialized
```

External Gateway availability is a separate layer because the API pod can be ready while external routing is broken.

---

# 60. Startup Sequence Summary

The complete conceptual sequence is:

```text
1. Container starts as UID/GID 65532.
2. main.go loads API configuration.
3. Structured logger is initialized.
4. Kubernetes in-cluster client is initialized.
5. OIDC configuration/provider/verifier is initialized.
6. Required CA trust is used for identity-provider connectivity.
7. Kubernetes/store dependencies are constructed.
8. HTTP handlers are constructed.
9. Routes are registered.
10. Authentication/authorization middleware is attached.
11. Request ID/logging/metrics middleware is attached.
12. /healthz, /readyz, and /metrics are exposed.
13. HTTP server begins listening.
14. Kubernetes probes validate the process.
15. Envoy can route HTTPS requests to the Service.
16. Prometheus can scrape /metrics.
```

---

# 61. Operational Success Criteria

The API startup/configuration layer is considered healthy when:

```text
[✓] API pod Running
[✓] API pod Ready
[✓] no startup errors in logs
[✓] OIDC discovery succeeds
[✓] Kubernetes API access succeeds
[✓] authenticated GET works
[✓] external HTTPS works
[✓] Prometheus target is up
[✓] API Prometheus rules show health=ok
```

---

# 62. Current Implementation Status

```text
[✓] configuration loading implemented
[✓] structured logging initialized
[✓] Kubernetes client initialized
[✓] OIDC/JWT verifier initialized
[✓] OIDC CA mounted
[✓] fixed managed namespace configured
[✓] health endpoint configured
[✓] readiness endpoint configured
[✓] metrics endpoint configured
[✓] hardened Deployment configured
[✓] NetworkPolicy startup dependencies validated
[✓] external HTTPS path validated
[✓] Prometheus scrape validated
[✓] rule configuration validated
```

The configuration/startup portion of the AI Platform REST API is implementation-complete.

---

# 63. Relationship to Next Document

The next document is:

```text
03-kubernetes-client-and-rbac.md
```

It expands the Kubernetes integration covered here, including:

- controller-runtime client
- ModelService operations
- ServiceAccount
- Role
- RoleBinding
- namespace scope
- least privilege
- Kubernetes API access
- operational RBAC validation

---

# 64. Final Summary

The AI Platform REST API startup path is deliberately dependency-aware and security-conscious.

The process is not simply:

```text
start HTTP listener
```

It is:

```text
load configuration
   |
   v
initialize logging
   |
   v
initialize Kubernetes access
   |
   v
initialize OIDC trust/verifier
   |
   v
construct API dependencies
   |
   v
construct routes/middleware
   |
   v
start HTTP server
   |
   v
become operationally ready
```

Correct startup depends on:

```text
DNS
OIDC connectivity
CA trust
Kubernetes API connectivity
ServiceAccount/RBAC
NetworkPolicy
runtime security
```

The implementation has been validated across all of these layers, including the NetworkPolicy/OIDC startup failure case discovered during hardening.
