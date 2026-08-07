# Kubernetes Security Hardening

## Purpose

This document explains how the `ModelService` API, controller, workloads, ServiceAccounts, and operator RBAC were hardened.

The main goals were:

```text
do not mount Kubernetes API credentials into model-serving Pods
apply least privilege to the operator
prevent workload privilege escalation
keep security behavior declarative in the ModelService spec
```

---

## ModelService Security Field

The API includes:

```yaml
spec:
  security:
    automountServiceAccountToken: false
```

The secure default is:

```text
false
```

---

## Go API Definition

File:

```text
api/v1alpha1/modelservice_types.go
```

```go
// AutomountServiceAccountToken controls whether Kubernetes API credentials
// are automatically mounted into the workload Pod.
//
// Model-serving workloads should normally not require direct Kubernetes
// API access.
//
// +kubebuilder:default=false
// +optional
AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
```

A pointer is used so the controller can distinguish:

```text
field omitted
explicit false
explicit true
```

The platform resolves an omitted value to `false`.

---

## Generated CRD Schema

File:

```text
config/crd/bases/platform.anselem.dev_modelservices.yaml
```

Expected schema:

```yaml
automountServiceAccountToken:
  default: false
  description: >-
    AutomountServiceAccountToken controls whether Kubernetes API
    credentials are automatically mounted into the workload Pod.
  type: boolean
```

Generate:

```bash
make generate
make manifests
```

Validate:

```bash
kubectl explain \
  modelservice.spec.security.automountServiceAccountToken
```

---

## Controller Resolution

File:

```text
internal/controller/modelservice_controller.go
```

Helper:

```go
func resolveAutomountServiceAccountToken(
    modelService *platformv1alpha1.ModelService,
) bool {
    if modelService.Spec.Security.AutomountServiceAccountToken == nil {
        return false
    }

    return *modelService.Spec.Security.AutomountServiceAccountToken
}
```

Use the resolved value in the workload Pod:

```go
AutomountServiceAccountToken: boolPointer(
    resolveAutomountServiceAccountToken(modelService),
),
```

Use the same value in the generated ServiceAccount:

```go
AutomountServiceAccountToken: boolPointer(
    resolveAutomountServiceAccountToken(modelService),
),
```

---

## Sample ModelService

File:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

```yaml
spec:
  security:
    runAsNonRoot: true
    runAsUser: 101
    runAsGroup: 101
    fsGroup: 101
    readOnlyRootFilesystem: true
    automountServiceAccountToken: false
```

---

## Apply the Updated CRD and Operator

```bash
make generate
make manifests
make test
```

```bash
make install
make deploy
```

Restart the controller when required:

```bash
kubectl rollout restart \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system
```

Wait:

```bash
kubectl rollout status \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system \
  --timeout=180s
```

Reapply the sample:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

---

## Validate the ServiceAccount

```bash
kubectl get serviceaccount fraud-model \
  -n ai-platform \
  -o yaml
```

Expected:

```yaml
automountServiceAccountToken: false
```

Compact check:

```bash
kubectl get serviceaccount fraud-model \
  -n ai-platform \
  -o jsonpath='{.automountServiceAccountToken}{"\n"}'
```

Expected:

```text
false
```

---

## Validate the Pod Template

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.automountServiceAccountToken}{"\n"}'
```

Expected:

```text
false
```

Inspect a running Pod:

```bash
FRAUD_MODEL_POD="$(
  kubectl get pod \
    -n ai-platform \
    -l app.kubernetes.io/name=fraud-model \
    -o jsonpath='{.items[0].metadata.name}'
)"
```

```bash
kubectl get pod "${FRAUD_MODEL_POD}" \
  -n ai-platform \
  -o jsonpath='{.spec.automountServiceAccountToken}{"\n"}'
```

Expected:

```text
false
```

---

## Confirm No ServiceAccount Token Volume Is Mounted

List volumes:

```bash
kubectl get pod "${FRAUD_MODEL_POD}" \
  -n ai-platform \
  -o json |
jq '
  .spec.volumes |
  map(
    select(
      .projected.sources[]?.serviceAccountToken
    )
  )
'
```

Expected:

```json
[]
```

Check the standard token path:

```bash
kubectl exec \
  -n ai-platform \
  "${FRAUD_MODEL_POD}" \
  -- \
  sh -c '
    if [ -e /var/run/secrets/kubernetes.io/serviceaccount/token ]; then
      echo "ERROR: Kubernetes API token is mounted"
      exit 1
    fi

    echo "PASS: Kubernetes API token is not mounted"
  '
```

---

## Workload Security Context

Expected controls:

```text
runAsNonRoot: true
runAsUser: 101
runAsGroup: 101
fsGroup: 101
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
capabilities.drop:
  - ALL
```

Inspect:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o json |
jq '{
  podSecurityContext: .spec.template.spec.securityContext,
  containerSecurityContext:
    .spec.template.spec.containers[0].securityContext
}'
```

---

## Operator RBAC Boundary

File:

```text
config/rbac/role.yaml
```

The operator requires permissions to reconcile its managed resources.

### Allowed for ModelService

```text
get
list
watch
update
patch
```

### Status subresource

```text
get
update
patch
```

### Finalizers

```text
update
```

### Not allowed on parent ModelService

```text
create
delete
```

The operator manages child resources, not the creation or deletion of the parent CR itself.

---

## Managed Resource Permissions

The operator needs permissions for:

```text
Deployments
Services
ServiceAccounts
PersistentVolumeClaims
PodDisruptionBudgets
NetworkPolicies
HTTPRoutes
Events
```

It should not receive broad cluster-administrator permissions.

---

## Operator RBAC Validation

Resolve the operator ServiceAccount:

```bash
OPERATOR_SA="system:serviceaccount:ai-platform-operator-system:ai-platform-operator-controller-manager"
```

Check parent resource restrictions:

```bash
kubectl auth can-i create modelservices.platform.anselem.dev \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

```bash
kubectl auth can-i delete modelservices.platform.anselem.dev \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

Check status update:

```bash
kubectl auth can-i update modelservices.platform.anselem.dev/status \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

Expected:

```text
yes
```

Check Secret access:

```bash
kubectl auth can-i get secrets \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

Check Node access:

```bash
kubectl auth can-i list nodes \
  --as="${OPERATOR_SA}"
```

Expected:

```text
no
```

Check ServiceAccount token creation:

```bash
kubectl auth can-i create serviceaccounts/token \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

---

## Workload RBAC Validation

Resolve the workload identity:

```bash
WORKLOAD_SA="system:serviceaccount:ai-platform:fraud-model"
```

```bash
kubectl auth can-i list pods \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

```bash
kubectl auth can-i get secrets \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

```bash
kubectl auth can-i create pods \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

```bash
kubectl auth can-i create serviceaccounts/token \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

```bash
kubectl auth can-i get modelservices.platform.anselem.dev \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

Expected:

```text
no
```

---

## Validation Script

Repository file:

```text
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
```

The script should verify:

```text
workload ServiceAccount automount=false
Pod template automount=false
running Pod automount=false
no token path exists
workload cannot read Secrets
workload cannot list Pods
workload cannot request tokens
operator cannot create or delete ModelServices
operator can update ModelService status
operator cannot read Secrets
operator cannot request ServiceAccount tokens
```

Run:

```bash
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
```

---

## Why This Matters

A model-serving container normally only needs:

```text
application configuration
model files
network access to approved dependencies
```

It usually does not need Kubernetes API credentials.

Disabling automatic token mounting reduces the impact of:

```text
container compromise
remote code execution
dependency vulnerability
unexpected shell access
```

---

## Explicit `true`

The API technically supports:

```yaml
automountServiceAccountToken: true
```

This should only be used when a workload has a documented Kubernetes API requirement and a dedicated least-privilege Role and RoleBinding.

The secure platform recommendation remains:

```text
false
```

---

## Troubleshooting

### Field is absent from `kubectl explain`

Run:

```bash
make manifests
make install
```

Confirm the CRD was updated in the current cluster.

### ServiceAccount shows `false`, but old Pods still have tokens

Restart the Deployment:

```bash
kubectl rollout restart \
  deployment/fraud-model \
  -n ai-platform
```

Wait for replacement Pods.

### Operator fails after RBAC tightening

Inspect:

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager
```

Grant only the specific missing verb and resource when it is required for reconciliation.

### `kubectl auth can-i` result is unexpectedly `yes`

Inspect all bindings:

```bash
kubectl get rolebinding,clusterrolebinding \
  -A \
  -o yaml |
grep -n -B5 -A10 \
  'ai-platform-operator-controller-manager'
```

The permission may come from another binding.

---

## Files Created or Modified

```text
api/v1alpha1/modelservice_types.go
api/v1alpha1/zz_generated.deepcopy.go
internal/controller/modelservice_controller.go
internal/controller/modelservice_controller_test.go
config/crd/bases/platform.anselem.dev_modelservices.yaml
config/rbac/role.yaml
config/samples/platform_v1alpha1_modelservice.yaml
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
```

---

## Completion Criteria

```text
[✓] API field added
[✓] CRD default is false
[✓] controller secure fallback is false
[✓] ServiceAccount receives false
[✓] PodSpec receives false
[✓] running Pods have no Kubernetes token
[✓] non-root security context enabled
[✓] read-only root filesystem enabled
[✓] capabilities dropped
[✓] operator cannot create ModelServices
[✓] operator cannot delete ModelServices
[✓] operator can update status
[✓] operator cannot read Secrets
[✓] workload has no Kubernetes API permissions
[✓] validation script passes
```
