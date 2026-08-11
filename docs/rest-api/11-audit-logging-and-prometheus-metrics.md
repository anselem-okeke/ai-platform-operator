# AI Platform REST API — Audit Logging and Prometheus Metrics

## 1. Purpose

This document describes the observability instrumentation implemented directly in the AI Platform REST API.

It covers:

- structured request logging
- request IDs
- mutation audit logging
- audit fields and outcomes
- edge-vs-application audit behavior
- Prometheus metrics
- middleware ordering
- route normalization
- cardinality control
- ServiceMonitor integration
- runtime validation

Grafana dashboards, alerting, SLI/SLOs, and burn-rate rules are documented separately in:

```text
12-grafana-alerting-and-slo.md
```

---

## 2. Observability Goals

The API observability model answers:

```text
Is the API reachable?
How much traffic is it receiving?
Which routes are used?
Which HTTP statuses are returned?
How long do requests take?
How many requests are in flight?
Who performed a mutation?
What was the mutation outcome?
```

---

## 3. Structured Logging

The API uses Go:

```text
log/slog
```

with structured JSON output.

Logging support lives under:

```text
internal/api/logging/
```

---

## 4. Request Logging

Every normal HTTP request passes through request logging middleware.

Important fields include:

```text
request_id
method
status
request context
```

The exact emitted field set remains defined by the current source.

---

## 5. Request ID

Correlation header:

```text
X-Request-ID
```

The API assigns or propagates a request ID.

The request ID is used across:

```text
request logs
audit logs
troubleshooting
```

---

## 6. Response Recorder

The request logging middleware uses a response recorder.

This captures the actual final HTTP status emitted downstream.

Without it, middleware could incorrectly log a default `200`.

---

## 7. Outer Middleware Ordering

The outer chain is:

```text
RequestID
  ->
RequestLogging
  ->
RequestMetrics
  ->
ServeMux
```

This ordering is intentional.

---

## 8. Protected Middleware Ordering

Protected API requests flow through:

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

Audit runs after authentication so user identity is available.

It wraps authorization/handler execution so final outcomes can be observed.

---

## 9. Mutation Audit Scope

Audit logging applies to:

```text
POST
PUT
PATCH
DELETE
```

It intentionally skips:

```text
GET
```

because the feature is designed as mutation audit logging.

---

## 10. Audit Event Identity

Audit events use:

```text
event=api_audit
```

and message:

```text
api_audit
```

---

## 11. Audit Fields

Recorded fields include:

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

Resource type:

```text
ModelService
```

---

## 12. Audit Outcomes

Possible outcomes include:

```text
success
unauthorized
denied
rejected
error
```

These distinguish different classes of mutation result.

---

## 13. Successful POST Audit Example

A successful create produced an event containing:

```text
event=api_audit
method=POST
route=/api/v1/model-services
resource_type=ModelService
status=201
outcome=success
username=service-account-ai-platform-service
```

---

## 14. POST Resource Name Behavior

For POST:

```text
resource_name
```

may be empty.

This is intentional because the audit middleware does not reread the already-consumed request body.

This avoids risky/complex request-body replay just to enrich an audit field.

---

## 15. Successful DELETE Audit Example

Admin DELETE produced:

```text
event=api_audit
method=DELETE
route=/api/v1/model-services/{name}
resource_type=ModelService
resource_name=audit-probe
status=204
outcome=success
username=admin-user
```

---

## 16. Resource Name from Path

For named mutation routes the middleware uses the path variable:

```text
r.PathValue("name")
```

This allows the audit event to record the affected ModelService name without parsing the body.

---

## 17. Token Safety

The API never logs:

```text
JWT access token
Authorization header
refresh token
```

This is a hard security requirement.

---

## 18. Identity in Audit, Not Metrics

Fields such as:

```text
subject
username
roles
resource_name
request_id
```

are useful in audit logs.

They are deliberately not used as Prometheus labels.

---

## 19. Edge-Denied Requests

If Envoy rejects a request:

```text
the Go API never sees it
```

Therefore:

```text
no Go audit event
```

is expected.

This happened during deployer DELETE validation.

---

## 20. Application-Denied Requests

If the request reaches the API and application authorization denies it, the audit middleware can record the identity and outcome.

This distinction is important during incident review.

---

## 21. Prometheus Metrics Package

Metrics collectors live under:

```text
internal/api/metrics/
```

Files include:

```text
doc.go
metrics.go
```

---

## 22. Request Counter

Metric:

```text
ai_platform_api_http_requests_total
```

Labels:

```text
method
route
status
```

This is a monotonically increasing counter.

---

## 23. Request Duration

Metric:

```text
ai_platform_api_http_request_duration_seconds
```

Labels:

```text
method
route
status
```

This is a histogram.

It supports:

```text
p95 latency
route latency
latency alerts
```

---

## 24. Requests In Flight

Metric:

```text
ai_platform_api_http_requests_in_flight
```

This tracks concurrent requests being processed.

---

## 25. Metrics Middleware

Request metrics middleware:

```text
increments in-flight
calls next handler
captures status
normalizes route
increments request counter
records duration
decrements in-flight
```

---

## 26. `/metrics` Is Skipped

The middleware skips:

```text
/metrics
```

so Prometheus scraping does not recursively generate more application request metrics.

---

## 27. Route Normalization

Known normalized routes:

```text
/healthz
/readyz
/metrics
/api/v1/model-services
/api/v1/model-services/{name}
/api/v1/model-services/{name}/status
unmatched
```

---

## 28. Why `r.Pattern` Was Not Enough

Nested `ServeMux` behavior meant relying directly on:

```text
r.Pattern
```

did not produce the desired stable metric route labels in all cases.

A dedicated normalization helper was introduced:

```text
metricRoute(r)
```

This maps actual URL paths to bounded templates.

---

## 29. Cardinality Protection

The API avoids labels such as:

```text
model name
username
subject
request ID
token
```

This protects Prometheus from unbounded cardinality.

---

## 30. Example Correct Route Label

Request:

```text
GET /api/v1/model-services/fraud-model
```

Metric route:

```text
/api/v1/model-services/{name}
```

---

## 31. Example Incorrect Design Avoided

The API does not emit:

```text
route="/api/v1/model-services/fraud-model"
```

because every unique ModelService would create another time series.

---

## 32. Metrics Tests

Metrics middleware tests live under:

```text
internal/api/middleware/metrics_test.go
```

The tests validate request observation behavior and route handling.

---

## 33. Audit Tests

Audit middleware tests live under:

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

## 34. ServiceMonitor

Prometheus discovers the API through:

```text
config/platform-api/servicemonitor.yaml
```

Namespace:

```text
ai-platform
```

---

## 35. ServiceMonitor Selection

Service selector:

```text
app.kubernetes.io/name=ai-platform-api
```

Endpoint:

```text
port: http
path: /metrics
scheme: http
interval: 30s
scrapeTimeout: 10s
```

---

## 36. Prometheus Target

The target is identified with:

```text
job="ai-platform-api"
namespace="ai-platform"
```

Healthy target:

```promql
up{
  job="ai-platform-api",
  namespace="ai-platform"
}
```

returns:

```text
1
```

---

## 37. NetworkPolicy and Metrics

Prometheus is explicitly allowed through NetworkPolicy.

Arbitrary pods are blocked.

This validates that monitoring access is intentional rather than a side effect of an open Service.

---

## 38. Raw Counter Query

Example:

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api_http_requests_total{route!="/healthz",route!="/readyz"}' |
jq .
```

---

## 39. Request Rate Query

```promql
sum by (route) (
  rate(
    ai_platform_api_http_requests_total{
      route!="/healthz",
      route!="/readyz"
    }[1m]
  )
)
```

The dashboard uses a short window for lab visibility.

---

## 40. p95 Query

```promql
histogram_quantile(
  0.95,
  sum by (le, route) (
    rate(
      ai_platform_api_http_request_duration_seconds_bucket{
        route!="/healthz",
        route!="/readyz"
      }[1m]
    )
  )
)
```

---

## 41. 5xx Error Ratio

The operational error-ratio pattern uses:

```promql
sum(
  rate(
    ai_platform_api_http_requests_total{
      route!="/healthz",
      route!="/readyz",
      status=~"5.."
    }[5m]
  )
)
```

over total customer-facing request rate.

---

## 42. Zero-Safe 5xx Numerator

When there are no 5xx series, the numerator uses:

```promql
or vector(0)
```

so zero errors are represented as zero rather than no series.

---

## 43. No-Traffic Semantics

No customer traffic is not automatically converted to a fabricated availability value.

This matters because:

```text
no traffic != failure
```

The design preserves meaningful no-data behavior where appropriate.

---

## 44. Process-Local Metric Reset

Metrics are in-process.

After pod restart:

```text
counters reset
```

A newly started pod may expose only health/readiness traffic initially.

Customer-facing dashboards/SLIs can therefore show no data until new API traffic arrives.

---

## 45. Prometheus Scrape Timing

ServiceMonitor interval:

```text
30s
```

After generating new traffic, allow roughly:

```text
60-90 seconds
```

for enough scrape/rule samples to appear.

---

## 46. Traffic Validation

Collection traffic:

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

Named-resource traffic:

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

---

## 47. Observed Route Metrics

Observed route labels included:

```text
/api/v1/model-services
/api/v1/model-services/{name}
```

with successful status:

```text
200
```

---

## 48. Request Metrics and SLOs

These raw metrics feed:

```text
availability SLI
error-ratio SLI
p95 latency SLI
request-rate SLI
error-budget burn rate
```

The raw instrumentation is therefore the foundation for the later SLO layer.

---

## 49. Logging vs Metrics vs Audit

Each signal has a different purpose.

### Request logs

```text
individual request troubleshooting
```

### Prometheus metrics

```text
aggregate service behavior
```

### Audit logs

```text
security/change accountability
```

The API intentionally implements all three.

---

## 50. Current Observability Status

```text
[✓] structured JSON logging
[✓] request IDs
[✓] response status capture
[✓] mutation audit middleware
[✓] identity in audit events
[✓] token-safe logging
[✓] request counter
[✓] duration histogram
[✓] in-flight gauge
[✓] low-cardinality route labels
[✓] ServiceMonitor
[✓] Prometheus scrape healthy
[✓] arbitrary direct pod blocked
[✓] audit tests
[✓] metrics tests
[✓] live mutation audit validated
[✓] live request metrics validated
```

---

## 51. Summary

The API observability layer combines:

```text
structured request logs
+
mutation audit events
+
Prometheus metrics
```

with strict separation of concerns.

Audit logs capture identity and mutation context.

Prometheus captures bounded aggregate service behavior.

Request IDs connect operational evidence without turning high-cardinality data into metrics.

This instrumentation provides the foundation for the Grafana dashboards, alerts, SLIs, SLOs, and burn-rate rules documented in the next chapter.
