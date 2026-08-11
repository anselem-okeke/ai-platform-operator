# AI Platform REST API — Recovery and Troubleshooting

## 1. Purpose

This document is the recovery and troubleshooting guide for the AI Platform REST API.

It is organized around observable symptoms and the layers that can cause them.

The main principle is:

```text
identify the failing layer before changing configuration
```

The API architecture contains multiple independent layers:

```text
client
Keycloak
TLS
Envoy
SecurityPolicy
Go API
NetworkPolicy
Kubernetes RBAC
Kubernetes API
ModelService
Operator
Prometheus
Grafana
```

A failure in one layer should not automatically lead to changes in another.

---

## 2. First Response Checklist

When something is wrong, collect these first:

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=ai-platform-api \
  -o wide
```

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform
```

```bash
kubectl logs \
  -n ai-platform \
  deployment/ai-platform-api \
  --since=10m
```

```bash
kubectl get svc \
  ai-platform-api \
  -n ai-platform
```

Then determine whether the failure is:

```text
startup
authentication
authorization
Gateway/TLS
Kubernetes backend
NetworkPolicy
metrics
alerts
```

---

## 3. API Pod Not Running

Check:

```bash
kubectl get pods -n ai-platform
kubectl describe pod -n ai-platform <pod>
kubectl logs -n ai-platform <pod>
```

Common causes:

```text
image missing
OIDC discovery failure
bad CA mount
NetworkPolicy egress
invalid config
probe failure
runtime security incompatibility
```

---

## 4. Image Not Found

The development deployment uses:

```text
imagePullPolicy: Never
```

and image:

```text
ai-platform-api:dev
```

If the image is not present in kind:

```bash
docker build \
  -f Dockerfile.platform-api \
  -t ai-platform-api:dev .

kind load docker-image \
  ai-platform-api:dev \
  --name ai-platform-policy

kubectl rollout restart \
  deployment/ai-platform-api \
  -n ai-platform
```

---

## 5. Rollout Stuck

```bash
kubectl rollout status \
  deployment/ai-platform-api \
  -n ai-platform \
  --timeout=180s
```

If timeout occurs:

```bash
kubectl get rs,pods -n ai-platform
kubectl describe deployment ai-platform-api -n ai-platform
kubectl describe pod -n ai-platform <pod>
kubectl logs -n ai-platform <pod>
```

---

## 6. OIDC Discovery Failure

Symptoms:

```text
startup failure
CrashLoopBackOff
issuer/JWKS/TLS errors
connection timeout
```

Check:

```text
OIDC issuer
CA ConfigMap
CA mount
DNS
NetworkPolicy egress
Envoy proxy path
Keycloak availability
```

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Internal JWKS:

```text
http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
```

---

## 7. OIDC CA Problems

ConfigMap:

```text
ai-platform-api-oidc-ca
```

Inspect:

```bash
kubectl get configmap \
  ai-platform-api-oidc-ca \
  -n ai-platform \
  -o yaml
```

Inspect Deployment volume/mount:

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

## 8. NetworkPolicy Blocks OIDC

This occurred during the API hardening phase.

The initial policy matched the logical load-balancer address but not the post-DNAT path seen by the CNI.

The working approach allowed Envoy proxy pods using namespace/pod selectors and target port:

```text
10443
```

Relevant labels:

```text
app.kubernetes.io/component=proxy
app.kubernetes.io/name=envoy
gateway.envoyproxy.io/owning-gateway-name=shared-gateway
gateway.envoyproxy.io/owning-gateway-namespace=gateway-system
```

---

## 9. DNS Failure

CoreDNS:

```text
10.96.0.10
```

The API requires:

```text
UDP 53
TCP 53
```

Check CoreDNS:

```bash
kubectl get pods \
  -n kube-system \
  -l k8s-app=kube-dns
```

If DNS is blocked, both internal Keycloak resolution and other service discovery can fail.

---

## 10. Kubernetes API Connectivity Failure

Relevant addresses:

```text
10.96.0.1:443
172.19.0.7:6443
```

Check API logs for connection/timeouts.

Check NetworkPolicy egress.

Do not change RBAC until network connectivity has been ruled out.

---

## 11. Kubernetes RBAC Failure

Check:

```bash
kubectl get sa \
  ai-platform-api \
  -n ai-platform
```

```bash
kubectl get role \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

```bash
kubectl get rolebinding \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

Use:

```bash
kubectl auth can-i \
  get \
  modelservices.platform.anselem.dev \
  -n ai-platform \
  --as=system:serviceaccount:ai-platform:ai-platform-api
```

---

## 12. CRD Missing

Check:

```bash
kubectl get crd \
  modelservices.platform.anselem.dev
```

Without the CRD, the API cannot operate on `ModelService`.

---

## 13. External HTTPS Fails but Pod Is Healthy

This usually points to:

```text
Gateway
HTTPRoute
SecurityPolicy
certificate
TLS secret
DNS/hosts
Envoy service
```

Inspect:

```bash
kubectl get gateway \
  shared-gateway \
  -n gateway-system \
  -o yaml
```

```bash
kubectl get httproute \
  -n ai-platform
```

```bash
kubectl get securitypolicy \
  ai-platform-api-jwt-authorization \
  -n ai-platform \
  -o yaml
```

---

## 14. Certificate Problems

Certificate:

```text
api-ai-platform-local
```

Secret:

```text
api-ai-platform-local-tls
```

Check:

```bash
kubectl get certificate \
  api-ai-platform-local \
  -n gateway-system
```

```bash
kubectl get secret \
  api-ai-platform-local-tls \
  -n gateway-system
```

Do not print private-key data.

---

## 15. Curl Certificate Error

Use the trusted CA:

```bash
--cacert .local/keycloak/fraud-model-root-ca.crt
```

Do not normalize testing by using:

```text
-k
--insecure
```

as the final solution.

---

## 16. 401 Unauthorized

Possible causes:

```text
missing token
expired token
wrong issuer
wrong audience
bad signature
Envoy JWT rejection
API JWT rejection
```

Tokens expire quickly.

Get a fresh machine token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

---

## 17. 403 Forbidden

Identify whether the denial occurred at:

```text
Envoy
```

or:

```text
Go API
```

If no application request/audit evidence exists, the edge may have rejected it.

Common role failures:

```text
viewer -> mutation
deployer -> DELETE
```

---

## 18. Deployer DELETE Returns 403

This is expected.

Delete requires:

```text
platform-admin
```

Use interactive admin PKCE login when validating DELETE:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

---

## 19. 404 ModelService Not Found

Check direct Kubernetes state:

```bash
kubectl get modelservice \
  <name> \
  -n ai-platform
```

If Kubernetes also reports NotFound, the API behavior is correct.

---

## 20. 409 Conflict

A conflict can indicate:

```text
resource already exists
resource version conflict
concurrent modification
```

Review request intent and current Kubernetes state before retrying blindly.

---

## 21. 5xx From API

Check:

```text
API logs
Kubernetes API connectivity
RBAC
resource conversion
operator/CRD state
```

5xx responses contribute to the service error SLI.

---

## 22. API Works Externally but Prometheus Target Is Down

Check:

```text
ServiceMonitor
Service selector
Service port name
NetworkPolicy
/metrics
Prometheus target discovery
```

ServiceMonitor:

```text
config/platform-api/servicemonitor.yaml
```

---

## 23. Prometheus Target Query

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

---

## 24. Direct Pod Test Fails

If an arbitrary pod receives:

```text
000 / timeout
```

against the API Service, this can be expected due to NetworkPolicy.

Do not weaken the policy merely to make an arbitrary test pod work.

---

## 25. Prometheus Scrape Must Still Work

The correct hardened state is:

```text
arbitrary pod -> blocked
Prometheus    -> allowed
Envoy         -> allowed
```

---

## 26. Dashboard Shows No Data After Restart

Expected cause:

```text
new API process
counters reset
only probes have hit the service
probe routes excluded
```

Generate customer traffic and wait for new scrapes.

---

## 27. Fresh Traffic

```bash
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

Then generate requests to:

```text
/api/v1/model-services
/api/v1/model-services/fraud-model
```

Wait approximately:

```text
60-90 seconds
```

---

## 28. Request Rate Still Empty

Query raw counter first:

```promql
ai_platform_api_http_requests_total{
  route!="/healthz",
  route!="/readyz"
}
```

If counter exists but `rate()` is empty, wait for another scrape.

---

## 29. PrometheusRule Exists but Rule Missing

Check the required selector label:

```yaml
release: kps
```

The Kubernetes CR can exist without Prometheus selecting it.

---

## 30. Verify Live Rule Groups

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

## 31. Rule Health Error

Inspect:

```text
health
lastError
```

from `/api/v1/rules`.

Healthy:

```text
health=ok
lastError=null
```

---

## 32. Test Rule Syntax

Use exact cluster Prometheus image:

```bash
PROM_IMAGE="$(
  kubectl get pod \
    -n monitoring \
    prometheus-kps-kube-prometheus-stack-prometheus-0 \
    -o jsonpath='{.spec.containers[?(@.name=="prometheus")].image}'
)"
```

Then:

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  check rules \
  /rules/ai-platform-api.rules.yaml
```

Expected:

```text
SUCCESS: 17 rules found
```

---

## 33. Run Rule Unit Tests

```bash
docker run --rm \
  -v "$PWD/infrastructure/monitoring/rule-tests:/rules:ro" \
  -w /rules \
  --entrypoint /bin/promtool \
  "${PROM_IMAGE}" \
  test rules \
  ai-platform-api.test.yaml
```

Expected:

```text
SUCCESS
```

---

## 34. Production/Test Rule Drift

Compare:

```text
config/platform-api/prometheusrule.yaml
infrastructure/monitoring/rule-tests/ai-platform-api.rules.yaml
```

A previous bug occurred because only one representation had the corrected availability expression.

Always update both.

---

## 35. Availability Alert Missing Labels

If `job` and `namespace` disappear from `AIPlatformAPIDown`, confirm the expression uses:

```promql
max by (job, namespace)
```

not:

```promql
max
```

---

## 36. Alert Stuck Pending

Remember:

```text
for:
```

starts when the alert expression becomes active, not necessarily when the Kubernetes change that caused it was issued.

Target staleness/scrape timing can delay `activeAt`.

---

## 37. Burn Alert Unexpectedly Active

Check:

```text
5m error ratio
1h error ratio
6h error ratio
burn-rate recordings
recent test traffic
```

Synthetic rule tests should be used to validate rule logic before changing production thresholds.

---

## 38. Audit Event Missing

Ask:

```text
Did Envoy reject the request first?
Was the method POST/PUT/PATCH/DELETE?
Did the request reach application auth?
```

GET requests are intentionally not mutation-audited.

---

## 39. POST Audit Resource Name Empty

This is expected.

The middleware intentionally does not reread the request body after the handler.

For named routes, resource name comes from:

```text
r.PathValue("name")
```

---

## 40. Request IDs

Use:

```text
X-Request-ID
```

to correlate API request logs and audit events.

Do not expect this value in Prometheus labels.

---

## 41. Runtime Security Failure

If a new change breaks under:

```text
readOnlyRootFilesystem
non-root
drop ALL
```

fix the application to work within the hardened runtime.

Do not solve it by removing the hardening unless there is a justified requirement.

---

## 42. Kustomize Source vs Live Drift

Render:

```bash
kubectl kustomize \
  config/platform-api \
  >/tmp/platform-api.yaml
```

Then inspect the relevant resource.

Compare with:

```bash
kubectl get <resource> ... -o yaml
```

This separates source/Kustomize issues from controller reconciliation issues.

---

## 43. Safe Apply Workflow

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform-api
```

Then:

```bash
kubectl apply \
  -k config/platform-api
```

---

## 44. Recovery After Bad Deployment

If a new image/config causes failure:

```text
1. inspect rollout/pod logs
2. identify image vs config cause
3. revert Git change or restore known-good manifest/image
4. reload image into kind if needed
5. rollout restart
6. verify authenticated API
7. verify Prometheus
```

Avoid changing several security controls simultaneously.

---

## 45. Recovery After NetworkPolicy Regression

If policy blocks startup:

```text
1. inspect API logs
2. identify dependency host/path
3. inspect actual CNI-observed destination
4. add the minimum selector/port required
5. apply
6. verify API starts
7. verify arbitrary pod remains blocked
```

This preserves least privilege.

---

## 46. Recovery After Prometheus Rule Error

```text
1. do not edit live object manually
2. fix Git source
3. mirror fix in test-native rule file
4. promtool check rules
5. promtool test rules
6. kubectl apply --dry-run=server
7. kubectl apply
8. verify /api/v1/rules health
```

---

## 47. Recovery After Token Issues

Machine:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Interactive:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Do not reuse expired token files indefinitely.

---

## 48. Full Health Verification

After recovery:

```text
[ ] pod Running/Ready
[ ] recent logs clean
[ ] authenticated GET succeeds
[ ] expected role behavior works
[ ] Kubernetes read/write works
[ ] Envoy HTTPS works
[ ] Prometheus up=1
[ ] rule health=ok
[ ] dashboard target UP
```

---

## 49. Escalation by Layer

Use this mapping:

```text
Keycloak/OIDC       -> auth config, issuer, JWKS, CA
Envoy/Gateway       -> Gateway, HTTPRoute, SecurityPolicy
Go API              -> logs, handlers, middleware
Kubernetes backend  -> ServiceAccount, RBAC, CRD, API connectivity
Network             -> NetworkPolicy, DNS, CNI path
Observability       -> ServiceMonitor, PrometheusRule, Grafana ConfigMap
```

---

## 50. Known-Good Completion State

At the end of the API implementation phase:

```text
API pod healthy
external HTTPS working
JWT/roles working
CRUD E2E PASS 20/20
NetworkPolicy hardened
Prometheus target up
Grafana dashboard populated
17 Prometheus rules valid
promtool tests PASS
fast-burn inactive/ok
slow-burn inactive/ok
```

This is the baseline to compare against during future recovery.

---

## 51. Summary

Troubleshooting the API is a layer-isolation exercise.

Do not treat:

```text
401
403
5xx
timeout
No data
missing alert
```

as interchangeable symptoms.

The key recovery pattern is:

```text
observe
  ->
identify layer
  ->
make minimal correction
  ->
validate security still holds
  ->
re-run end-to-end checks
```

This preserves the security and reliability properties built into the REST API phase.
