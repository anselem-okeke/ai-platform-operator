# AI Platform REST API — Grafana, Alerting, and SLO

## 1. Purpose

This document describes the monitoring, dashboarding, alerting, SLI/SLO, and error-budget implementation for the AI Platform REST API.

It records:

- the kube-prometheus-stack deployment used by the API
- ServiceMonitor integration
- Grafana provisioning
- dashboard panels
- Prometheus alert rules
- SLI recording rules
- the 99.9% availability SLO
- error-budget consumption
- fast- and slow-burn alerts
- promtool validation
- runtime behavior discovered during testing

---

## 2. Monitoring Stack

The monitoring stack is based on:

```text
kube-prometheus-stack
```

Namespace:

```text
monitoring
```

Helm release:

```text
kps
```

Observed chart/version information during the implementation included:

```text
kube-prometheus-stack-88.2.0
Prometheus 3.13.2
Grafana 13.1.3
```

The Prometheus image used for matching rule tests is:

```text
quay.io/prometheus/prometheus:v3.13.2-distroless
```

---

## 3. Prometheus Service

Prometheus Service:

```text
kps-kube-prometheus-stack-prometheus
```

Namespace:

```text
monitoring
```

Port:

```text
9090
```

A common local access method is:

```bash
kubectl port-forward \
  -n monitoring \
  service/kps-kube-prometheus-stack-prometheus \
  19090:9090
```

Prometheus can then be queried through:

```text
http://127.0.0.1:19090
```

---

## 4. ServiceMonitor

The API is discovered through:

```text
config/platform-api/servicemonitor.yaml
```

Namespace:

```text
ai-platform
```

Service selector:

```text
app.kubernetes.io/name=ai-platform-api
```

Endpoint settings:

```text
port: http
path: /metrics
scheme: http
interval: 30s
scrapeTimeout: 10s
```

---

## 5. Prometheus Target

Healthy target identity:

```text
job="ai-platform-api"
namespace="ai-platform"
```

Query:

```promql
up{
  job="ai-platform-api",
  namespace="ai-platform"
}
```

Expected healthy value:

```text
1
```

---

## 6. NetworkPolicy Interaction

Prometheus is explicitly allowed to reach the API through the API NetworkPolicy.

Prometheus pod labels used for the allow rule include:

```text
app.kubernetes.io/name=prometheus
operator.prometheus.io/name=kps-kube-prometheus-stack-prometheus
```

The monitoring path remains available while arbitrary pod access is blocked.

---

## 7. Grafana Provisioning

Grafana dashboards are provisioned declaratively through ConfigMaps.

The Grafana sidecar watches for:

```text
grafana_dashboard=1
```

The dashboard ConfigMap is:

```text
infrastructure/monitoring/grafana-dashboard-platform-api.yaml
```

This keeps the dashboard in Git instead of relying on manual UI-only configuration.

---

## 8. Grafana Datasource

Datasource:

```text
Prometheus
```

Datasource UID:

```text
prometheus
```

URL:

```text
http://kps-kube-prometheus-stack-prometheus.monitoring:9090/
```

The dashboard uses the datasource variable:

```text
${DS_PROMETHEUS}
```

---

## 9. Dashboard Identity

Dashboard title:

```text
AI Platform API Overview
```

Dashboard UID:

```text
ai-platform-api-overview
```

---

## 10. Dashboard Panels

The dashboard contains eight main panels:

```text
1. API Target
2. API Requests/sec
3. 5xx Error Rate
4. API p95 Latency
5. Requests In Flight
6. Request Rate by Route
7. Request Rate by Status
8. p95 Latency by Route
```

These panels cover the core API operational signals:

```text
availability
traffic
errors
latency
concurrency
route distribution
status distribution
```

---

## 11. API Target Panel

The target panel uses absent-safe logic.

Conceptually:

```promql
max(
  up{
    job="ai-platform-api",
    namespace="ai-platform"
  }
)
or
vector(0)
```

This avoids a missing target silently becoming only `No data`.

Operational interpretation:

```text
1 -> UP
0 -> DOWN
```

---

## 12. Request Rate Panel

The dashboard uses short-window request-rate queries for lab visibility.

Example:

```promql
sum(
  rate(
    ai_platform_api_http_requests_total{
      route!="/healthz",
      route!="/readyz"
    }[1m]
  )
)
```

The short window makes traffic bursts visible quickly during testing.

---

## 13. 5xx Error Rate Panel

The numerator is zero-safe:

```promql
(
  sum(
    rate(
      ai_platform_api_http_requests_total{
        route!="/healthz",
        route!="/readyz",
        status=~"5.."
      }[5m]
    )
  )
  or vector(0)
)
```

The denominator is protected with `clamp_min`.

This allows healthy traffic with no 5xx series to display:

```text
0%
```

instead of `No data`.

---

## 14. p95 Latency

Latency is derived from:

```text
ai_platform_api_http_request_duration_seconds_bucket
```

using:

```promql
histogram_quantile(0.95, ...)
```

The dashboard includes both:

```text
overall p95
p95 by route
```

---

## 15. Requests In Flight

Metric:

```text
ai_platform_api_http_requests_in_flight
```

This is expected to frequently display:

```text
0
```

in a low-traffic lab environment.

That is a valid value, not a failure.

---

## 16. Route Normalization in Grafana

Dashboard route series use normalized labels such as:

```text
/api/v1/model-services
/api/v1/model-services/{name}
```

Actual ModelService names are not exposed as metric labels.

This protects Prometheus cardinality.

---

## 17. Dashboard Validation

The dashboard was validated with generated API traffic.

Observed healthy behavior included:

```text
API Target              UP
5xx Error Rate          0.00%
p95 latency             well below 500 ms
route panels            populated
status panels           populated
```

---

## 18. Pod Restart and No Data

Application metrics are process-local.

After the API pod is recreated:

```text
counters reset
```

Health/readiness traffic continues, but those routes are excluded from customer-facing panels.

Therefore:

```text
No data
```

on request-rate/latency panels immediately after restart is expected until new customer traffic arrives.

---

## 19. Traffic Generation

Machine token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

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

Named resource:

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

## 20. Scrape Timing

The ServiceMonitor interval is:

```text
30 seconds
```

After fresh traffic, allow roughly:

```text
60-90 seconds
```

for rate/histogram calculations and recording rules to become useful.

---

## 21. PrometheusRule

Production rule resource:

```text
config/platform-api/prometheusrule.yaml
```

The rule must have:

```yaml
release: kps
```

because Prometheus selects rule objects with that label.

This was discovered during live rule-loading validation.

---

## 22. Rule Groups

The live API rule groups are:

```text
ai-platform-api.sli
ai-platform-api.slo
ai-platform-api.availability
ai-platform-api.errors
ai-platform-api.latency
```

---

## 23. Rule Count

Final validated rule count:

```text
17
```

Syntax validation result:

```text
SUCCESS: 17 rules found
```

Rule-test result:

```text
SUCCESS
```

---

## 24. Availability Alert

Alert:

```text
AIPlatformAPIDown
```

Purpose:

```text
detect zero healthy scrape targets or a missing target
```

Expression:

```promql
(
  max by (job, namespace) (
    up{
      job="ai-platform-api",
      namespace="ai-platform"
    }
  ) == 0
)
or
absent(
  up{
    job="ai-platform-api",
    namespace="ai-platform"
  }
)
```

Duration:

```text
for: 2m
```

Severity:

```text
critical
```

---

## 25. Label-Preservation Fix

The initial rule used:

```promql
max(...)
```

which dropped:

```text
job
namespace
```

from the alert result.

The corrected expression uses:

```promql
max by (job, namespace) (...)
```

This makes alert labels consistent across target-down and target-absent branches.

---

## 26. Availability Alert Validation

Live validation confirmed:

```text
healthy  -> inactive
outage   -> pending
recovery -> inactive
```

The full firing logic is validated by `promtool`.

---

## 27. High 5xx Alert

Alert:

```text
AIPlatformAPIHigh5xxRate
```

Threshold:

```text
5xx ratio > 5%
```

Duration:

```text
for: 5m
```

Severity:

```text
warning
```

Health/readiness routes are excluded.

---

## 28. High p95 Alert

Alert:

```text
AIPlatformAPIHighP95Latency
```

Threshold:

```text
p95 > 0.5 seconds
```

Duration:

```text
for: 5m
```

Severity:

```text
warning
```

Synthetic histogram testing validates the rule.

---

## 29. SLI Recording Rules

Short-window recordings:

```text
ai_platform_api:sli_availability_ratio:5m
ai_platform_api:sli_error_ratio:5m
ai_platform_api:sli_p95_latency_seconds:5m
ai_platform_api:sli_request_rate:5m
```

Longer windows:

```text
ai_platform_api:sli_availability_ratio:1h
ai_platform_api:sli_availability_ratio:24h
ai_platform_api:sli_error_ratio:1h
ai_platform_api:sli_error_ratio:6h
```

---

## 30. Availability SLI

Availability is request-based.

Successful availability response:

```text
non-5xx
```

Service failure:

```text
5xx
```

Health/readiness traffic is excluded.

---

## 31. Why 4xx Is Not Availability Failure

Examples:

```text
401 -> caller authentication issue
403 -> authorization decision
404 -> valid negative lookup
409 -> conflict
```

These responses can indicate that the API is working correctly.

The service SLI therefore focuses on server-side `5xx`.

---

## 32. Availability SLO

Engineering objective:

```text
99.9%
```

Allowed error ratio:

```text
0.001
```

This is an engineering SLO for the lab platform, not a contractual SLA.

---

## 33. Error-Budget Consumption

Recording rule:

```text
ai_platform_api:slo_availability_error_budget_consumed_ratio:24h
```

Interpretation:

```text
0.0 -> no budget consumed
0.5 -> half consumed
1.0 -> all budget consumed
>1  -> SLO violated
```

Healthy validation returned:

```text
0
```

---

## 34. Runtime SLI Validation

Observed healthy values included:

```text
availability:1h  = 1
availability:24h = 1
error budget consumed:24h = 0
```

A sanity check also showed:

```text
availability + error ratio = 1
```

for observed request traffic.

---

## 35. Error-Budget Burn Rate

Burn rate:

```text
observed error ratio
--------------------
allowed error ratio
```

For the 99.9% SLO:

```text
allowed error ratio = 0.001
```

Examples:

```text
0.001 -> 1x
0.010 -> 10x
0.020 -> 20x
```

---

## 36. Burn-Rate Recording Rules

Implemented:

```text
ai_platform_api:slo_error_budget_burn_rate:5m
ai_platform_api:slo_error_budget_burn_rate:1h
ai_platform_api:slo_error_budget_burn_rate:6h
```

---

## 37. Fast-Burn Alert

Alert:

```text
AIPlatformAPIErrorBudgetFastBurn
```

Condition:

```text
5m burn > 14.4
AND
1h burn > 14.4
```

Duration:

```text
for: 2m
```

Severity:

```text
critical
```

Label:

```text
slo=availability
```

---

## 38. Slow-Burn Alert

Alert:

```text
AIPlatformAPIErrorBudgetSlowBurn
```

Condition:

```text
1h burn > 6
AND
6h burn > 6
```

Duration:

```text
for: 15m
```

Severity:

```text
warning
```

Label:

```text
slo=availability
```

---

## 39. Live SLO Alert State

Final live verification showed:

```text
AIPlatformAPIErrorBudgetFastBurn
  state: inactive
  health: ok
  lastError: null

AIPlatformAPIErrorBudgetSlowBurn
  state: inactive
  health: ok
  lastError: null
```

This is the expected healthy condition.

---

## 40. promtool Version

The VM did not have a local `promtool`.

Instead, the exact cluster image was used.

Determine image:

```bash
PROM_IMAGE="$(
  kubectl get pod \
    -n monitoring \
    prometheus-kps-kube-prometheus-stack-prometheus-0 \
    -o jsonpath='{.spec.containers[?(@.name=="prometheus")].image}'
)"
```

Observed:

```text
quay.io/prometheus/prometheus:v3.13.2-distroless
```

---

## 41. Rule Syntax Check

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

## 42. Rule Tests

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  -w /rules \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  test rules \
  ai-platform-api.test.yaml
```

Final result:

```text
SUCCESS
```

---

## 43. Synthetic Test Coverage

Synthetic rule tests cover:

```text
API down
high 5xx
high p95 latency
SLI error ratio
fast burn
slow burn
```

This allows deterministic rule validation without deliberately damaging the live API for every failure mode.

---

## 44. Production/Test Rule Drift

Production rule source:

```text
config/platform-api/prometheusrule.yaml
```

Test-native Prometheus rules:

```text
infrastructure/monitoring/rule-tests/ai-platform-api.rules.yaml
```

These are separate files and can drift.

A real drift occurred during the `max by (job, namespace)` fix.

Every rule change should therefore update and validate both representations.

---

## 45. Verify Live Rule Groups

```bash
curl -s \
  'http://127.0.0.1:19090/api/v1/rules' |
jq -r '
  .data.groups[]
  | select(.name | startswith("ai-platform-api"))
  | .name
'
```

Expected:

```text
ai-platform-api.availability
ai-platform-api.errors
ai-platform-api.latency
ai-platform-api.sli
ai-platform-api.slo
```

---

## 46. Verify SLO Alerts

```bash
curl -s \
  'http://127.0.0.1:19090/api/v1/rules' |
jq -r '
  .data.groups[]
  | select(.name == "ai-platform-api.slo")
  | .rules[]
  | {
      name: .name,
      state: .state,
      health: .health,
      lastError: .lastError
    }
'
```

Healthy result:

```text
inactive
ok
null
```

for both burn alerts.

---

## 47. Current Monitoring/SLO Status

```text
[✓] Prometheus stack installed
[✓] ServiceMonitor
[✓] target healthy
[✓] Grafana datasource
[✓] declarative dashboard
[✓] 8 API dashboard panels
[✓] availability alert
[✓] 5xx alert
[✓] latency alert
[✓] SLI recording rules
[✓] 99.9% availability SLO
[✓] error-budget metric
[✓] fast-burn alert
[✓] slow-burn alert
[✓] matching promtool version
[✓] 17 rules syntax valid
[✓] synthetic rule suite PASS
[✓] live rule health verified
```

---

## 48. Summary

The REST API observability model is:

```text
raw HTTP metrics
      |
      v
ServiceMonitor
      |
      v
Prometheus
   |      |
   |      +--> alerts
   |      +--> recording rules
   |      +--> SLIs
   |      +--> SLO
   |      +--> error budget
   |      +--> burn rate
   |
   v
Grafana
```

The implementation goes beyond basic dashboards by validating request-based SLIs, a 99.9% availability objective, error-budget consumption, and multi-window burn-rate alerts using deterministic `promtool` tests.
