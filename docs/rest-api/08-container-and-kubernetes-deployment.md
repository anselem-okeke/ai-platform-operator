# AI Platform REST API — Container and Kubernetes Deployment

## 1. Purpose

This document describes how the AI Platform REST API is packaged as a container and deployed to Kubernetes.

It records:

- container build strategy
- runtime image choices
- non-root execution
- Deployment configuration
- Service and ServiceAccount integration
- probes
- resource settings
- CA mounts
- rollout workflow
- kind image loading
- deployment validation
- runtime troubleshooting

This document follows:

```text
00-overview-and-architecture.md
01-api-contract-and-project-structure.md
02-configuration-and-startup.md
03-kubernetes-client-and-rbac.md
04-read-endpoints.md
05-authentication-and-authorization.md
06-create-update-patch-delete.md
07-validation-errors-and-namespace-restrictions.md
```

---

## 2. Container Build File

The API image is built from:

```text
Dockerfile.platform-api
```

The image build uses a multi-stage pattern.

Builder:

```text
golang:1.26
```

Runtime:

```text
distroless static:nonroot
```

The runtime image is deliberately minimal.

---

## 3. Why Distroless

The runtime image does not include a general-purpose shell or package manager.

Benefits include:

```text
smaller attack surface
fewer runtime packages
reduced image size
fewer unnecessary tools
non-root default runtime
```

The API should not depend on shell scripts or writable system directories at runtime.

---

## 4. Runtime Identity

The container runs as:

```text
UID 65532
GID 65532
```

This matches the non-root distroless runtime identity.

The Deployment explicitly confirms non-root execution rather than relying only on the image default.

---

## 5. Runtime Security Context

Pod-level settings include:

```text
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
seccompProfile: RuntimeDefault
```

Container-level settings include:

```text
allowPrivilegeEscalation: false
capabilities:
  drop:
    - ALL
readOnlyRootFilesystem: true
```

These controls were validated on the live Deployment.

---

## 6. ServiceAccount

The Deployment uses:

```text
ServiceAccount: ai-platform-api
```

The ServiceAccount token remains mounted because the API must authenticate to the Kubernetes API.

This is intentional:

```text
automountServiceAccountToken: true
```

The security boundary comes from least-privilege RBAC.

---

## 7. Kubernetes Namespace

The API runs in:

```text
ai-platform
```

The Service, Deployment, ServiceAccount, RBAC, NetworkPolicy, ServiceMonitor, and PrometheusRule are all managed around that namespace.

---

## 8. Development Image

The development image tag is:

```text
ai-platform-api:dev
```

The Deployment uses:

```text
imagePullPolicy: Never
```

because the image is loaded directly into the kind cluster.

---

## 9. Build Command

From repository root:

```bash
docker build \
  -f Dockerfile.platform-api \
  -t ai-platform-api:dev .
```

---

## 10. Load Image Into kind

Cluster name:

```text
ai-platform-policy
```

Load command:

```bash
kind load docker-image \
  ai-platform-api:dev \
  --name ai-platform-policy
```

This makes the locally built image available to kind nodes without a remote registry.

---

## 11. Rollout Restart

After loading a new image:

```bash
kubectl rollout restart \
  deployment/ai-platform-api \
  -n ai-platform
```

Then wait:

```bash
kubectl rollout status \
  deployment/ai-platform-api \
  -n ai-platform \
  --timeout=180s
```

---

## 12. Deployment Validation

Check pod state:

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=ai-platform-api \
  -o wide
```

Healthy state:

```text
READY 1/1
STATUS Running
RESTARTS 0
```

A healthy pod was repeatedly validated during the REST API phase.

---

## 13. Pod IP and Node

The kind environment assigns pod IPs from the cluster pod network.

Example observed API pod IPs changed across rollouts, including addresses in:

```text
10.244.42.0/24
```

The exact pod IP is ephemeral and should never be treated as stable configuration.

---

## 14. Kubernetes Service

Service name:

```text
ai-platform-api
```

Namespace:

```text
ai-platform
```

The application listens internally on:

```text
TCP 8080
```

The Service provides stable in-cluster addressing while pod IPs change.

---

## 15. Service Role in Architecture

The Service is used by:

```text
Envoy Gateway -> API
Prometheus -> API
```

The Service is not intended as an unrestricted general-purpose access path for arbitrary pods.

NetworkPolicy enforces that restriction.

---

## 16. Liveness Probe

The Deployment uses:

```text
/healthz
```

for liveness.

Purpose:

```text
detect dead/stuck process
```

The liveness endpoint is deliberately simple.

---

## 17. Readiness Probe

The Deployment uses:

```text
/readyz
```

for readiness.

Purpose:

```text
determine whether pod should receive traffic
```

Readiness is separate from process liveness.

---

## 18. Probe Traffic and Metrics

Health/readiness probe traffic is excluded from customer-facing SLIs.

This prevents Kubernetes probes from inflating:

```text
availability
request rate
latency
```

---

## 19. Resource Requests and Limits

The Deployment includes resource requests and limits.

These protect both:

```text
scheduler placement
runtime resource boundaries
```

The exact values should be read from the live/source Deployment manifest when operational tuning is required.

---

## 20. OIDC CA ConfigMap

The API Deployment mounts:

```text
ConfigMap/ai-platform-api-oidc-ca
```

This provides trust material required by the OIDC flow.

A missing CA mount can prevent the API from starting correctly.

---

## 21. OIDC CA Volume

The Deployment therefore contains:

```text
volume
volumeMount
```

for the CA ConfigMap.

Both must be present and aligned.

Troubleshooting should verify:

```text
ConfigMap exists
volume references ConfigMap
volumeMount references volume
application points to correct path
```

---

## 22. Read-Only Root Filesystem

Because:

```text
readOnlyRootFilesystem: true
```

the application cannot depend on writing runtime state under normal filesystem paths.

This is one reason transient tokens are stored outside the container in local test tooling rather than written by the API.

---

## 23. Distroless Troubleshooting

Because the image is distroless, commands such as:

```text
sh
bash
curl
apt
```

are not expected to exist inside the container.

Troubleshooting should use:

```text
kubectl logs
kubectl describe
ephemeral/debug pod
external curl pod
Prometheus metrics
```

instead of assuming an interactive shell in the API container.

---

## 24. Deployment Manifests

The API manifests are stored under:

```text
config/platform-api/
```

and assembled by:

```text
config/platform-api/kustomization.yaml
```

This keeps deployment configuration declarative and Git-managed.

---

## 25. Server-Side Dry Run

Before applying manifest changes:

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform-api
```

This validates resources against the live API server without persisting the change.

---

## 26. Apply

Apply:

```bash
kubectl apply \
  -k config/platform-api
```

This manages the complete API resource set.

---

## 27. Rendered Kustomize Inspection

Render:

```bash
kubectl kustomize \
  config/platform-api \
  >/tmp/platform-api.yaml
```

Inspect before applying when debugging configuration drift.

---

## 28. Deployment Security Validation

Inspect Deployment security context:

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

Verify:

```text
runAsNonRoot
runAsUser
runAsGroup
seccompProfile
allowPrivilegeEscalation
capabilities
readOnlyRootFilesystem
```

---

## 29. ServiceAccount Validation

Check:

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

## 30. Image Validation

Check image:

```bash
kubectl get deployment \
  ai-platform-api \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.containers[0].image}'

echo
```

Expected in the lab:

```text
ai-platform-api:dev
```

---

## 31. Rollout Troubleshooting

If rollout stalls:

```bash
kubectl rollout status \
  deployment/ai-platform-api \
  -n ai-platform \
  --timeout=180s
```

then inspect:

```bash
kubectl get pods -n ai-platform
kubectl describe pod -n ai-platform <pod>
kubectl logs -n ai-platform <pod>
```

Common causes:

```text
image not loaded into kind
OIDC startup failure
NetworkPolicy egress issue
probe failure
bad CA mount
Kubernetes client startup issue
invalid environment/config
```

---

## 32. Image Not Found in kind

Because:

```text
imagePullPolicy: Never
```

the pod fails if the image was not loaded into kind.

Fix:

```bash
kind load docker-image \
  ai-platform-api:dev \
  --name ai-platform-policy
```

Then restart rollout.

---

## 33. Crash During OIDC Initialization

Symptoms:

```text
CrashLoopBackOff
startup errors
OIDC discovery errors
```

Check:

```text
CA ConfigMap
NetworkPolicy egress
DNS
Keycloak/Envoy path
issuer configuration
```

This was a real hardening issue encountered during the project.

---

## 34. Pod Running but External API Broken

A healthy Deployment does not prove Gateway routing works.

If pod is healthy but external access fails, investigate:

```text
Envoy Gateway
HTTPRoute
SecurityPolicy
TLS certificate
DNS/hosts resolution
```

Do not rebuild the image unnecessarily.

---

## 35. Pod Running but CRUD Fails

Check:

```text
ServiceAccount
Role
RoleBinding
Kubernetes API connectivity
namespace restriction
CRD availability
```

---

## 36. Pod Running but Prometheus Down

If API works but Prometheus shows target down:

```text
ServiceMonitor
Service labels
port name
NetworkPolicy
/metrics
```

should be checked.

---

## 37. Current Deployment Status

```text
[✓] multi-stage container build
[✓] distroless runtime
[✓] non-root UID/GID
[✓] read-only root filesystem
[✓] dropped capabilities
[✓] no privilege escalation
[✓] RuntimeDefault seccomp
[✓] ServiceAccount attached
[✓] OIDC CA mounted
[✓] liveness/readiness probes
[✓] Service configured
[✓] kind image workflow validated
[✓] rollout workflow validated
[✓] deployment manifest stored in Git
```

---

## 38. Summary

The REST API is deployed as a hardened, non-root Go service:

```text
Dockerfile.platform-api
    |
    v
ai-platform-api:dev
    |
    v
kind cluster
    |
    v
Deployment/ai-platform-api
    |
    +--> ServiceAccount/RBAC
    +--> probes
    +--> CA mount
    +--> runtime hardening
    |
    v
Service/ai-platform-api
```

The deployment is intentionally minimal, declarative, and compatible with the security controls used throughout the platform.
