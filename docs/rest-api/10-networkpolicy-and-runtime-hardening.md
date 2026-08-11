# AI Platform REST API — NetworkPolicy and Runtime Hardening

## 1. Purpose

This document describes the network and runtime hardening applied to the AI Platform REST API.

It records:

- ingress restrictions
- egress restrictions
- DNS access
- Kubernetes API access
- Envoy/OIDC egress path
- Prometheus access
- direct-access denial
- pod/container security context
- non-root runtime
- capabilities
- seccomp
- read-only filesystem
- validation and troubleshooting

---

## 2. Security Goals

The API should not be reachable from arbitrary pods and should not have unrestricted network egress.

The runtime should not require root privileges.

The combined design is:

```text
restricted ingress
restricted egress
non-root process
read-only root filesystem
no extra capabilities
no privilege escalation
seccomp RuntimeDefault
least-privilege Kubernetes RBAC
```

---

## 3. NetworkPolicy

The API NetworkPolicy is stored under:

```text
config/platform-api/
```

and included in:

```text
config/platform-api/kustomization.yaml
```

It protects the API pods in:

```text
ai-platform
```

---

## 4. Ingress Policy

Ingress is restricted to approved sources.

Required ingress paths:

```text
Envoy -> API :8080
Prometheus -> API :8080
```

Arbitrary application pods are not allowed direct API access.

---

## 5. Envoy Ingress

External requests arrive through Envoy.

The policy allows traffic from Envoy proxy pods to:

```text
TCP 8080
```

on API pods.

This preserves the intended external entry point.

---

## 6. Prometheus Ingress

Prometheus must scrape:

```text
/metrics
```

on API port:

```text
8080
```

The ingress policy therefore allows Prometheus from:

```text
namespace: monitoring
```

with pod labels including:

```text
app.kubernetes.io/name=prometheus
operator.prometheus.io/name=kps-kube-prometheus-stack-prometheus
```

---

## 7. Arbitrary Pod Denial

A direct-access validation command used:

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

with connection timeout.

This was expected.

---

## 8. Why Direct Access Is Blocked

The intended application path is:

```text
external client
   ->
Envoy
   ->
API
```

not:

```text
arbitrary pod
   ->
API Service
```

Blocking arbitrary direct access reduces bypass paths around edge controls.

---

## 9. Egress Policy

API egress is restricted to required dependencies.

Required categories:

```text
DNS
Kubernetes API
OIDC/Envoy path
```

There is no wildcard unrestricted egress in the hardened design.

---

## 10. DNS Egress

CoreDNS:

```text
10.96.0.10
```

Required:

```text
UDP 53
TCP 53
```

DNS is needed for:

```text
Keycloak service resolution
internal Kubernetes service names
```

---

## 11. Kubernetes API Egress

Kubernetes Service:

```text
10.96.0.1:443
```

Observed kind API endpoint:

```text
172.19.0.7:6443
```

The NetworkPolicy must allow the path the API actually uses.

---

## 12. OIDC Egress

The API performs OIDC discovery during startup.

Therefore it must reach the identity-provider path.

This became a real failure point during hardening.

---

## 13. Initial Hardening Failure

After tightening NetworkPolicy, the API failed OIDC discovery.

The logical assumption had been that allowing the load-balancer destination was enough.

It was not.

The CNI observed the packet after DNAT.

---

## 14. Post-DNAT Lesson

The traffic path as seen by the CNI effectively involved the Envoy proxy pod.

The fix therefore used:

```text
namespace selector
+
pod selector
+
target port 10443
```

instead of relying only on the load-balancer IP.

---

## 15. Envoy Proxy Labels

Relevant labels:

```text
app.kubernetes.io/component=proxy
app.kubernetes.io/name=envoy
gateway.envoyproxy.io/owning-gateway-name=shared-gateway
gateway.envoyproxy.io/owning-gateway-namespace=gateway-system
```

These selectors are more stable for policy intent than ephemeral Envoy pod IPs.

---

## 16. Envoy Target Port

Observed target port:

```text
10443
```

This was required for the corrected egress rule.

---

## 17. NetworkPolicy Troubleshooting Principle

Write policy according to:

```text
packet path seen by CNI
```

not only:

```text
logical client destination
```

This is one of the most important operational lessons from the API hardening phase.

---

## 18. Prometheus Verification After Hardening

After the NetworkPolicy fix:

```text
Prometheus scrape remained healthy
```

while:

```text
arbitrary pod access remained blocked
```

This proved the ingress policy was selective rather than simply permissive.

---

## 19. API Startup Verification After Hardening

The API could again complete OIDC initialization and reach:

```text
Running / Ready
```

after the corrected egress path was allowed.

---

## 20. Runtime Security Context

Pod-level controls:

```text
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
seccompProfile:
  type: RuntimeDefault
```

---

## 21. Container Security Context

Container-level controls:

```text
allowPrivilegeEscalation: false
capabilities:
  drop:
    - ALL
readOnlyRootFilesystem: true
```

These were validated on the live Deployment.

---

## 22. Non-Root Execution

The API runs as:

```text
65532:65532
```

The process does not require UID 0.

---

## 23. No Privilege Escalation

```text
allowPrivilegeEscalation: false
```

prevents gaining additional privileges through setuid-style mechanisms.

---

## 24. Drop All Capabilities

The container drops:

```text
ALL
```

Linux capabilities.

The HTTP API and Kubernetes client do not require privileged kernel capabilities.

---

## 25. Read-Only Root Filesystem

```text
readOnlyRootFilesystem: true
```

prevents application compromise from trivially modifying the root filesystem.

The application must therefore avoid runtime writes to normal root paths.

---

## 26. Seccomp

Pod uses:

```text
RuntimeDefault
```

seccomp.

This applies the runtime's default syscall filtering profile.

---

## 27. ServiceAccount Token

The pod still uses:

```text
automountServiceAccountToken: true
```

because Kubernetes API access is a functional requirement.

Runtime hardening should not break required functionality.

---

## 28. Distroless Runtime

Runtime:

```text
distroless static:nonroot
```

This complements the security context by removing unnecessary userland tooling.

---

## 29. Defense in Depth

The runtime/network model combines:

```text
Envoy edge policy
NetworkPolicy
application auth
Kubernetes RBAC
non-root runtime
read-only filesystem
capability drop
seccomp
```

No one layer is treated as sufficient by itself.

---

## 30. Verify Security Context

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

Check the pod and container `securityContext`.

---

## 31. Verify NetworkPolicy

```bash
kubectl get networkpolicy \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

Inspect:

```text
podSelector
policyTypes
ingress
egress
ports
namespaceSelectors
podSelectors
```

---

## 32. Verify Prometheus Still Works

Query:

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

## 33. Verify Direct Access Still Fails

Re-run the arbitrary-pod test.

Expected:

```text
timeout / 000
```

If it succeeds unexpectedly, inspect the ingress policy.

---

## 34. Verify External API Still Works

Obtain token and call:

```text
https://api.ai-platform.local/api/v1/model-services
```

This proves:

```text
Envoy ingress path still permitted
```

---

## 35. Verify OIDC Startup

After rollout, inspect logs:

```bash
kubectl logs \
  -n ai-platform \
  deployment/ai-platform-api \
  --since=5m
```

OIDC connectivity errors after a policy change usually indicate egress regression.

---

## 36. Common NetworkPolicy Failure Modes

```text
DNS blocked
Kubernetes API blocked
OIDC/Envoy path blocked
Prometheus selector mismatch
Envoy selector mismatch
wrong target port
wrong namespace selector
```

---

## 37. Avoid IP-Only Policies for Dynamic Pods

Pod IPs are ephemeral.

Prefer:

```text
namespaceSelector
podSelector
```

when expressing stable intent.

---

## 38. NetworkPolicy and Service DNAT

Remember that Kubernetes Service/load-balancer traffic can be transformed before policy evaluation.

This is why a logical Service/LB address may not be the correct object to match in the final policy.

---

## 39. Runtime Hardening Failure Modes

If the app fails after securityContext hardening, check whether it tries to:

```text
write root filesystem
bind privileged port
change UID
use required Linux capability
write temporary files to unavailable path
```

The API was adapted so none of these privileged behaviors are required.

---

## 40. Pod Security Validation Result

The hardened API was observed with:

```text
runAsNonRoot true
runAsUser 65532
runAsGroup 65532
seccomp RuntimeDefault
allowPrivilegeEscalation false
drop ALL
readOnlyRootFilesystem true
```

and remained functional.

---

## 41. Current Hardening Status

```text
[✓] ingress restricted
[✓] Envoy allowed
[✓] Prometheus allowed
[✓] arbitrary pod blocked
[✓] DNS egress allowed
[✓] Kubernetes API egress allowed
[✓] OIDC/Envoy egress allowed
[✓] wildcard egress avoided
[✓] non-root runtime
[✓] fixed UID/GID
[✓] no privilege escalation
[✓] drop ALL capabilities
[✓] read-only root filesystem
[✓] RuntimeDefault seccomp
[✓] hardened runtime validated
```

---

## 42. Summary

The API security model prevents both unnecessary network access and unnecessary runtime privilege.

```text
Network:
  Envoy -> API
  Prometheus -> API
  everything else denied unless required

Egress:
  DNS
  Kubernetes API
  OIDC/Envoy dependency

Runtime:
  UID/GID 65532
  non-root
  read-only root
  no capabilities
  no privilege escalation
  RuntimeDefault seccomp
```

The NetworkPolicy hardening work also produced a key operational lesson: policy must reflect the actual CNI-observed packet path, especially around Service/LB DNAT.
