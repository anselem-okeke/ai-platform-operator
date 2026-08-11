# AI Platform REST API — Testing and End-to-End Validation

## 1. Purpose

This document records how the AI Platform REST API was tested and validated.

It covers:

- Go unit/integration testing
- authentication and authorization validation
- Kubernetes integration validation
- CRUD E2E testing
- runtime security validation
- NetworkPolicy validation
- audit validation
- Prometheus metrics validation
- Grafana validation
- Prometheus rule tests
- final completion evidence

---

## 2. Testing Layers

The API phase was validated at multiple levels:

```text
Go unit tests
    |
    v
middleware / handler / store tests
    |
    v
Kubernetes integration
    |
    v
external HTTPS/JWT testing
    |
    v
CRUD end-to-end workflow
    |
    v
security/network validation
    |
    v
monitoring/SLO validation
```

No single test layer was treated as sufficient.

---

## 3. Go Test Scope

The API test suite includes coverage around areas such as:

```text
routing
authentication
authorization
Kubernetes store/client behavior
request metrics
audit logging
validation
handler behavior
operator/security interactions
```

The exact test names remain defined by the repository source.

---

## 4. General Go Test Checkpoint

The standard repository test command is:

```bash
make test
```

This should be run after meaningful Go changes.

---

## 5. Middleware Metrics Tests

File:

```text
internal/api/middleware/metrics_test.go
```

These tests validate HTTP metric behavior including normalized routes and status capture.

---

## 6. Audit Middleware Tests

File:

```text
internal/api/middleware/audit_test.go
```

Validated scenarios include:

```text
successful mutation
denied mutation
GET skipped
```

---

## 7. Kubernetes Store Tests

The Kubernetes integration layer is tested separately from HTTP routing.

This validates behaviors such as:

```text
list
get
create
update
patch
delete
error propagation
not found
```

---

## 8. Authentication Tests

Authentication tests cover JWT/OIDC behavior and request protection.

The API must reject invalid/missing authentication before protected handler execution.

---

## 9. Authorization Tests

Role behavior is tested against:

```text
viewer
deployer
admin
```

The intended matrix is:

```text
viewer   -> reads
deployer -> reads + create/update/patch
admin    -> all, including delete
```

---

## 10. External Authentication Validation

Machine token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Interactive user token:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

These flows were used for live API validation.

---

## 11. CRUD E2E Script

Automated workflow:

```text
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

Final observed result:

```text
PASS 20/20
```

This is one of the strongest completion signals for the REST API phase.

---

## 12. What CRUD E2E Covers

The workflow validates the combined path through:

```text
Keycloak token
Envoy
SecurityPolicy
Go API
authorization
validation
Kubernetes CRUD
HTTP status handling
resource lifecycle
```

---

## 13. Read Endpoint Validation

Live reads were performed against:

```text
GET /api/v1/model-services
GET /api/v1/model-services/fraud-model
GET /api/v1/model-services/fraud-model/status
```

Successful reads also generated expected Prometheus route series.

---

## 14. Create Validation

A temporary resource was created through the API.

Observed successful create behavior included:

```text
HTTP 201
audit event outcome=success
```

---

## 15. Admin Delete Validation

Temporary resource:

```text
audit-probe
```

Admin DELETE:

```text
HTTP 204
```

Audit event contained:

```text
resource_name=audit-probe
username=admin-user
outcome=success
```

Kubernetes verification showed the resource was gone.

---

## 16. Deployer Delete Negative Test

A non-admin machine/deployer identity attempted DELETE.

Observed:

```text
HTTP 403
```

at Envoy.

This validated the edge role policy.

---

## 17. Why No Go Audit Event Appeared

Because Envoy rejected the request before the Go API:

```text
no application audit event
```

was expected.

This was treated as correct behavior rather than an audit defect.

---

## 18. Namespace Restriction Validation

The API is fixed to:

```text
ai-platform
```

and Kubernetes RBAC is also namespace scoped.

A useful negative RBAC test is:

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

---

## 19. Runtime Security Validation

The live Deployment was inspected and confirmed:

```text
runAsNonRoot true
runAsUser 65532
runAsGroup 65532
seccomp RuntimeDefault
allowPrivilegeEscalation false
drop ALL
readOnlyRootFilesystem true
```

The API remained functional under these controls.

---

## 20. NetworkPolicy Negative Test

An arbitrary pod attempted direct access:

```bash
kubectl run api-network-test \
  -n ai-platform \
  --image=curlimages/curl:8.12.1 \
  --restart=Never \
  --rm -i \
  -- \
  curl \
  --connect-timeout 5 \
  -sS \
  -o /dev/null \
  -w '%{http_code}\\n' \
  http://ai-platform-api.ai-platform.svc.cluster.local:8080/healthz
```

Observed:

```text
000
```

with timeout.

This proved arbitrary direct ingress was blocked.

---

## 21. Prometheus Positive Network Test

While arbitrary pod access was blocked, Prometheus remained able to scrape:

```text
/metrics
```

Healthy:

```promql
up{job="ai-platform-api",namespace="ai-platform"} = 1
```

This proved the policy was selective.

---

## 22. OIDC NetworkPolicy Regression Test

A hardened NetworkPolicy initially prevented OIDC discovery.

The failure was diagnosed as the CNI seeing post-DNAT Envoy traffic.

The policy was fixed to allow the Envoy proxy path on:

```text
10443
```

The API then started successfully again.

This was an important real-world troubleshooting validation.

---

## 23. External HTTPS Validation

Authenticated curl requests to:

```text
https://api.ai-platform.local
```

validated:

```text
TLS
Vault-issued certificate trust
Gateway routing
SecurityPolicy
JWT
Go API
Kubernetes read/write
```

---

## 24. Prometheus Metrics Validation

Raw counter:

```text
ai_platform_api_http_requests_total
```

was queried after generating traffic.

Observed series included:

```text
route="/api/v1/model-services"
status="200"
```

and:

```text
route="/api/v1/model-services/{name}"
status="200"
```

---

## 25. Route Normalization Validation

The live metrics confirmed resource names were normalized.

This validated the low-cardinality route strategy.

---

## 26. Histogram Validation

The p95 dashboard/query populated after traffic.

This proved the duration histogram was being scraped and evaluated correctly.

---

## 27. Grafana Validation

Dashboard:

```text
AI Platform API Overview
```

was visually validated with live traffic.

Observed:

```text
API Target UP
5xx Error Rate 0%
p95 populated
request route series populated
status series populated
```

---

## 28. Pod Restart Metrics Behavior

After scale down/up, request panels showed `No data`.

This was correctly diagnosed as:

```text
process-local counter reset
+
only health/readiness traffic
+
customer routes excluded
```

Fresh customer traffic restored the series.

---

## 29. PrometheusRule Selector Validation

The Prometheus resource selects rules with:

```yaml
release: kps
```

The API PrometheusRule initially lacked that label and was therefore not loaded.

Adding:

```yaml
release: kps
```

fixed discovery.

This validated the full operator selection path rather than only CR existence.

---

## 30. Availability Alert Live Test

`AIPlatformAPIDown` was tested by scaling the API to zero.

Observed progression included:

```text
inactive -> pending
```

The service was restored before the full firing duration during one live test.

Recovery returned the alert to:

```text
inactive
```

---

## 31. Availability Firing Logic

Full firing behavior was validated deterministically through `promtool`.

This avoided repeatedly taking the live API down.

---

## 32. Alert Label Bug Found by Unit Test

The first synthetic availability test failed because:

```promql
max(...)
```

dropped `job` and `namespace`.

The test exposed the problem.

The expression was corrected to:

```promql
max by (job, namespace) (...)
```

This is a good example of tests improving observability quality, not only checking syntax.

---

## 33. Prometheus Version Matching

Local `promtool` was not installed.

Instead, the exact Prometheus image running in the cluster was used.

Observed:

```text
promtool 3.13.2
quay.io/prometheus/prometheus:v3.13.2-distroless
```

This avoided testing rules with an older Ubuntu package version.

---

## 34. Rule Syntax Validation

Command:

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  check rules \
  /rules/ai-platform-api.rules.yaml
```

Final result:

```text
SUCCESS: 17 rules found
```

---

## 35. Rule Unit Tests

Command:

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  -w /rules \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  test rules \
  ai-platform-api.test.yaml
```

Final:

```text
SUCCESS
```

---

## 36. Synthetic Rule Coverage

The final synthetic suite covers:

```text
API down
5xx alert
p95 latency alert
SLI error ratio
fast burn
slow burn
```

---

## 37. SLI Runtime Validation

Observed:

```text
availability:1h  = 1
availability:24h = 1
error budget consumed:24h = 0
```

A combined sanity query returned:

```text
availability + error ratio = 1
```

for the observed successful traffic.

---

## 38. Live Burn Alert Validation

Final live SLO group showed:

```text
AIPlatformAPIErrorBudgetFastBurn
  inactive
  ok

AIPlatformAPIErrorBudgetSlowBurn
  inactive
  ok
```

with:

```text
lastError = null
```

---

## 39. Kustomize Validation

Before applying significant changes:

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform-api
```

Render:

```bash
kubectl kustomize \
  config/platform-api \
  >/tmp/platform-api.yaml
```

This was used to catch/verify source-to-live configuration alignment.

---

## 40. Live CR Validation

Example:

```bash
kubectl get prometheusrule \
  ai-platform-api \
  -n ai-platform \
  -o json
```

This proves Kubernetes stores the intended rule.

Prometheus `/api/v1/rules` then proves the rule is actually loaded.

---

## 41. Why Both Checks Matter

These are different:

```text
PrometheusRule exists in Kubernetes
```

vs:

```text
Prometheus loaded/evaluates it
```

Both were validated.

---

## 42. Final API Test Matrix

```text
[✓] Go unit tests
[✓] handler/router tests
[✓] authentication tests
[✓] authorization tests
[✓] Kubernetes store/client tests
[✓] metrics middleware tests
[✓] audit middleware tests
[✓] CRUD E2E PASS 20/20
[✓] machine-token access
[✓] admin PKCE access
[✓] deployer delete denial
[✓] admin delete success
[✓] delete confirmed in Kubernetes
[✓] NetworkPolicy negative test
[✓] Prometheus positive scrape
[✓] runtime security validated
[✓] external HTTPS validated
[✓] dashboard validated
[✓] 17 Prometheus rules syntax valid
[✓] synthetic Prometheus rule suite PASS
[✓] live SLO rules healthy
```

---

## 43. Completion Interpretation

The API implementation is not considered complete merely because:

```text
the server starts
```

It was validated across:

```text
functionality
security
networking
Kubernetes state
external exposure
observability
alerting
SLO behavior
```

This multi-layer validation is the basis for marking the implementation complete.

---

## 44. Recommended Regression Check

For future changes, run at minimum:

```text
1. make test
2. CRUD E2E workflow
3. promtool check rules
4. promtool test rules
5. kubectl apply --dry-run=server
6. rollout status
7. authenticated external read
8. Prometheus up query
```

Add role-specific/security tests when the change affects auth, networking, or RBAC.

---

## 45. Summary

The REST API has been tested as a complete platform path:

```text
client
  ->
Keycloak
  ->
Envoy
  ->
Go API
  ->
Kubernetes
  ->
ModelService
  ->
Operator
```

and as an operational system:

```text
API
  ->
metrics
  ->
Prometheus
  ->
Grafana
  ->
alerts
  ->
SLIs/SLO
```

The final test evidence supports marking the technical REST API implementation complete.
