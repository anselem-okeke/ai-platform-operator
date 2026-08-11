# AI Platform REST API — Complete Command Reference

## 1. Purpose

This document collects the operational commands used throughout the AI Platform REST API phase.

It is intended as a quick command reference, not as a replacement for the explanatory documentation.

Run commands from:

```text
/mnt/data/ai-platform-operator
```

unless stated otherwise.

---

# 2. Environment

Repository:

```bash
cd /mnt/data/ai-platform-operator
```

Cluster:

```text
ai-platform-policy
```

Context:

```text
kind-ai-platform-policy
```

API namespace:

```text
ai-platform
```

Monitoring namespace:

```text
monitoring
```

Gateway namespace:

```text
gateway-system
```

---

# 3. Build API Image

```bash
docker build \
  -f Dockerfile.platform-api \
  -t ai-platform-api:dev .
```

---

# 4. Load Image Into kind

```bash
kind load docker-image \
  ai-platform-api:dev \
  --name ai-platform-policy
```

---

# 5. Restart API

```bash
kubectl rollout restart \
  deployment/ai-platform-api \
  -n ai-platform
```

---

# 6. Wait for Rollout

```bash
kubectl rollout status \
  deployment/ai-platform-api \
  -n ai-platform \
  --timeout=180s
```

---

# 7. API Pods

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=ai-platform-api \
  -o wide
```

---

# 8. API Logs

```bash
kubectl logs \
  -n ai-platform \
  deployment/ai-platform-api \
  --since=10m
```

---

# 9. API Service

```bash
kubectl get service \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 10. Deployment

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 11. ServiceAccount

```bash
kubectl get serviceaccount \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 12. Role

```bash
kubectl get role \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 13. RoleBinding

```bash
kubectl get rolebinding \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 14. Deployment ServiceAccount

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.serviceAccountName}'

echo
```

Expected:

```text
ai-platform-api
```

---

# 15. RBAC Can-I — Get

```bash
kubectl auth can-i \
  get \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

# 16. RBAC Can-I — List

```bash
kubectl auth can-i \
  list \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

# 17. RBAC Can-I — Create

```bash
kubectl auth can-i \
  create \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

# 18. RBAC Can-I — Update

```bash
kubectl auth can-i \
  update \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

# 19. RBAC Can-I — Patch

```bash
kubectl auth can-i \
  patch \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

# 20. RBAC Can-I — Delete

```bash
kubectl auth can-i \
  delete \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

# 21. Negative Cross-Namespace RBAC

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

# 22. ModelService CRD

```bash
kubectl get crd \
  modelservices.platform.anselem.dev
```

---

# 23. List ModelServices

```bash
kubectl get modelservices \
  -n ai-platform
```

---

# 24. Fraud Model

```bash
kubectl get modelservice \
  fraud-model \
  -n ai-platform \
  -o yaml
```

---

# 25. Machine Token

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

---

# 26. Load Machine Token

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

---

# 27. List API

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services
```

---

# 28. Get Fraud Model

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services/fraud-model
```

---

# 29. Get Fraud Model Status

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services/fraud-model/status
```

---

# 30. Clear Machine Token

```bash
unset TOKEN
```

---

# 31. Interactive Admin Login

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

---

# 32. Load Admin Token

```bash
ADMIN_TOKEN="$(
  cat .local/keycloak/tokens/user-access-token.jwt
)"
```

---

# 33. Clear Admin Token

```bash
unset ADMIN_TOKEN
```

---

# 34. CRUD E2E Workflow

```bash
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

Expected validated result:

```text
PASS 20/20
```

---

# 35. Go Tests

```bash
make test
```

---

# 36. Server-Side Dry Run

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform-api
```

---

# 37. Apply API Manifests

```bash
kubectl apply \
  -k config/platform-api
```

---

# 38. Render Kustomize

```bash
kubectl kustomize \
  config/platform-api \
  >/tmp/platform-api.yaml
```

---

# 39. NetworkPolicy

```bash
kubectl get networkpolicy \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 40. Direct Access Negative Test

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
  -w '%{http_code}\n' \
  http://ai-platform-api.ai-platform.svc.cluster.local:8080/healthz
```

Expected:

```text
000
```

with timeout.

---

# 41. Gateway

```bash
kubectl get gateway \
  shared-gateway \
  -n gateway-system \
  -o yaml
```

---

# 42. API HTTPRoute

```bash
kubectl get httproute \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 43. HTTP Redirect Route

```bash
kubectl get httproute \
  ai-platform-api-http-redirect \
  -n ai-platform \
  -o yaml
```

---

# 44. API SecurityPolicy

```bash
kubectl get securitypolicy \
  ai-platform-api-jwt-authorization \
  -n ai-platform \
  -o yaml
```

---

# 45. API Certificate

```bash
kubectl get certificate \
  api-ai-platform-local \
  -n gateway-system \
  -o yaml
```

---

# 46. API TLS Secret

```bash
kubectl get secret \
  api-ai-platform-local-tls \
  -n gateway-system
```

---

# 47. ServiceMonitor

```bash
kubectl get servicemonitor \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 48. PrometheusRule

```bash
kubectl get prometheusrule \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

# 49. Prometheus Port Forward

```bash
kubectl port-forward \
  -n monitoring \
  service/kps-kube-prometheus-stack-prometheus \
  19090:9090
```

---

# 50. Prometheus API Target

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=max(up{job="ai-platform-api",namespace="ai-platform"})' |
jq .
```

---

# 51. Raw Customer Request Metrics

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api_http_requests_total{route!="/healthz",route!="/readyz"}' |
jq -r '
  .data.result[] | {
    pod: .metric.pod,
    route: .metric.route,
    status: .metric.status,
    value: .value[1]
  }
'
```

---

# 52. Request Rate

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=sum by (route) (rate(ai_platform_api_http_requests_total{route!="/healthz",route!="/readyz"}[1m]))' |
jq .
```

---

# 53. p95 by Route

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=histogram_quantile(0.95,sum by (le,route)(rate(ai_platform_api_http_request_duration_seconds_bucket{route!="/healthz",route!="/readyz"}[1m])))' |
jq .
```

---

# 54. API SLI — Availability 5m

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:sli_availability_ratio:5m' |
jq .
```

---

# 55. API SLI — Error Ratio 5m

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:sli_error_ratio:5m' |
jq .
```

---

# 56. API SLI — p95 5m

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:sli_p95_latency_seconds:5m' |
jq .
```

---

# 57. Availability 1h

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:sli_availability_ratio:1h' |
jq .
```

---

# 58. Availability 24h

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:sli_availability_ratio:24h' |
jq .
```

---

# 59. Error Budget Consumed

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:slo_availability_error_budget_consumed_ratio:24h' |
jq .
```

---

# 60. Burn Rate 5m

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:slo_error_budget_burn_rate:5m' |
jq .
```

---

# 61. Burn Rate 1h

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:slo_error_budget_burn_rate:1h' |
jq .
```

---

# 62. Burn Rate 6h

```bash
curl -sG \
  'http://127.0.0.1:19090/api/v1/query' \
  --data-urlencode \
  'query=ai_platform_api:slo_error_budget_burn_rate:6h' |
jq .
```

---

# 63. List API Prometheus Rule Groups

```bash
curl -s \
  'http://127.0.0.1:19090/api/v1/rules' |
jq -r '
  .data.groups[]
  | select(.name | startswith("ai-platform-api"))
  | .name
'
```

---

# 64. SLO Alert State

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

---

# 65. Availability Alert State

```bash
curl -s \
  'http://127.0.0.1:19090/api/v1/rules' |
jq -r '
  .data.groups[].rules[]
  | select(.name == "AIPlatformAPIDown")
  | {
      state: .state,
      health: .health,
      query: .query
    }
'
```

---

# 66. Determine Prometheus Image

```bash
PROM_IMAGE="$(
  kubectl get pod \
    -n monitoring \
    prometheus-kps-kube-prometheus-stack-prometheus-0 \
    -o jsonpath='{.spec.containers[?(@.name=="prometheus")].image}'
)"

echo "${PROM_IMAGE}"
```

Validated image:

```text
quay.io/prometheus/prometheus:v3.13.2-distroless
```

---

# 67. promtool Version

```bash
docker run --rm \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  --version
```

---

# 68. Check Prometheus Rules

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  check rules \
  /rules/ai-platform-api.rules.yaml
```

Validated:

```text
SUCCESS: 17 rules found
```

---

# 69. Test Prometheus Rules

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  -w /rules \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  test rules \
  ai-platform-api.test.yaml
```

Validated:

```text
SUCCESS
```

---

# 70. Traffic Generation

```bash
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

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

```bash
unset TOKEN
```

---

# 71. Scale API to Zero for Availability Test

```bash
kubectl scale \
  deployment/ai-platform-api \
  -n ai-platform \
  --replicas=0
```

Use only when intentionally testing availability behavior.

---

# 72. Restore API

```bash
kubectl scale \
  deployment/ai-platform-api \
  -n ai-platform \
  --replicas=1
```

Then:

```bash
kubectl rollout status \
  deployment/ai-platform-api \
  -n ai-platform \
  --timeout=180s
```

---

# 73. Check API Pods After Restore

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=ai-platform-api \
  -o wide
```

---

# 74. Grafana Dashboard ConfigMap

```bash
kubectl get configmap \
  -n monitoring \
  -l grafana_dashboard=1
```

API dashboard source:

```text
infrastructure/monitoring/grafana-dashboard-platform-api.yaml
```

---

# 75. Monitoring Pods

```bash
kubectl get pods \
  -n monitoring
```

---

# 76. Keycloak Pods

```bash
kubectl get pods \
  -n keycloak
```

---

# 77. Envoy Pods

```bash
kubectl get pods \
  -n envoy-gateway-system
```

---

# 78. Final Verification Sequence

Recommended:

```bash
make test
```

```bash
infrastructure/platform-api/scripts/validate-api-crud-workflow.sh
```

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  check rules \
  /rules/ai-platform-api.rules.yaml
```

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  -w /rules \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  test rules \
  ai-platform-api.test.yaml
```

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform-api
```

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=ai-platform-api
```

Then perform one authenticated API GET and confirm Prometheus target health.

---

# 79. Summary

This command reference covers:

```text
build
deploy
rollout
RBAC
authentication
API reads
CRUD validation
Gateway/TLS
NetworkPolicy
Prometheus
SLIs/SLOs
alerts
rule tests
recovery
```

For reasoning and architecture, use the numbered REST API documentation chapters rather than relying only on this command list.
