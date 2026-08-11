# AI Platform REST API — Kubernetes Client and RBAC

## 1. Purpose

This document describes how the AI Platform REST API integrates with Kubernetes and how least-privilege RBAC is enforced.

It records:

- the controller-runtime client model
- the `ModelService` resource handled by the API
- namespace scoping
- ServiceAccount identity
- Role and RoleBinding design
- allowed Kubernetes operations
- the separation between HTTP authorization and Kubernetes authorization
- failure modes and troubleshooting
- validation commands used during the API phase

This document complements:

```text
docs/rest-api/00-overview-and-architecture.md
docs/rest-api/01-api-contract-and-project-structure.md
docs/rest-api/02-configuration-and-startup.md
```

---

# 2. Kubernetes Resource Managed by the API

The REST API manages the custom resource:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
```

The API does not create arbitrary Kubernetes resources directly.

Instead, it changes the desired state represented by `ModelService`, and the AI Platform Operator performs reconciliation.

The control flow is:

```text
REST API
   |
   | create / read / update / patch / delete
   v
ModelService CR
   |
   v
AI Platform Operator
   |
   v
Deployment / Service / PVC / SA / Policy / Route
```

This preserves the Kubernetes control-loop architecture.

---

# 3. Kubernetes Client Library

The API uses:

```text
controller-runtime client
```

The Kubernetes integration code lives under:

```text
internal/api/kubernetes/
```

The controller-runtime client is appropriate here because the API already operates in the same ecosystem as the operator and works with typed/custom Kubernetes resources.

The client is responsible for persistence and retrieval of `ModelService` objects.

---

# 4. API/Kubernetes Layering

The API separates HTTP concerns from Kubernetes persistence.

Conceptually:

```text
HTTP request
   |
   v
handler
   |
   v
validation
   |
   v
Kubernetes abstraction/store
   |
   v
controller-runtime client
   |
   v
Kubernetes API
```

This separation allows:

- HTTP behavior to be tested independently
- Kubernetes operations to be isolated
- error translation to remain consistent
- authorization to happen before persistence
- the API to avoid embedding Kubernetes calls directly in route registration

---

# 5. Managed Namespace

The API is restricted to:

```text
ai-platform
```

This is one of the most important security boundaries in the API.

Callers do not provide arbitrary namespace values.

Every `ModelService` operation is scoped to the fixed platform namespace.

This includes:

```text
list
get
status
create
update
patch
delete
```

---

# 6. Why Namespace Restriction Matters

Without a fixed namespace, the API could become a generic Kubernetes proxy.

For example, a caller might attempt:

```text
namespace=kube-system
namespace=monitoring
namespace=gateway-system
```

The API deliberately prevents this class of access.

The security model is:

```text
REST contract
    |
    | fixed namespace
    v
ai-platform only
```

This reduces the authorization surface and makes the platform easier to reason about.

---

# 7. Kubernetes ServiceAccount

The API runs as:

```text
ServiceAccount: ai-platform-api
```

Namespace:

```text
ai-platform
```

The ServiceAccount is attached to the API Deployment.

Because the API must call the Kubernetes API, the ServiceAccount token remains available to the pod.

This means:

```text
automountServiceAccountToken: true
```

is intentional.

The security control is least-privilege RBAC, not removal of Kubernetes credentials.

---

# 8. Kubernetes Authentication Flow

Inside the cluster:

```text
API pod
   |
   | ServiceAccount token
   v
Kubernetes API
   |
   | authenticated as ServiceAccount
   v
RBAC evaluation
```

This is separate from the user-facing authentication flow:

```text
Client
   |
   | Keycloak JWT
   v
REST API
```

The two identities must not be confused.

---

# 9. Two Authorization Boundaries

The platform uses two major authorization layers.

## Application authorization

This answers:

```text
Is this caller allowed to perform this REST operation?
```

Examples:

```text
viewer   -> reads
deployer -> reads + mutations except delete
admin    -> full CRUD
```

## Kubernetes RBAC

This answers:

```text
Is the API process itself allowed to perform this Kubernetes operation?
```

These are independent controls.

The effective flow is:

```text
user JWT
   |
   v
API role authorization
   |
   v
API ServiceAccount
   |
   v
Kubernetes RBAC
   |
   v
ModelService operation
```

---

# 10. Defense in Depth

The API intentionally does not rely on only one authorization layer.

Even if an application authorization bug allowed a request to pass, Kubernetes RBAC still limits what the API process can do.

Likewise, Kubernetes RBAC does not replace user-level authorization because every request reaches Kubernetes under the same ServiceAccount identity.

Therefore:

```text
application RBAC
    +
Kubernetes RBAC
```

are both required.

---

# 11. RBAC Resources

The API uses:

```text
ServiceAccount/ai-platform-api
Role/ai-platform-api
RoleBinding/ai-platform-api
```

Namespace:

```text
ai-platform
```

These resources are stored under:

```text
config/platform-api/
```

and included in:

```text
config/platform-api/kustomization.yaml
```

---

# 12. Role Scope

The Role is namespace-scoped.

This is deliberate.

Using a Role rather than a ClusterRole keeps the API's Kubernetes authority restricted to:

```text
ai-platform
```

where the API-managed `ModelService` resources live.

The API does not require cluster-wide administrative access.

---

# 13. ModelService Permissions

The API requires Kubernetes permissions corresponding to its REST contract.

Conceptually, it needs operations equivalent to:

```text
get
list
create
update
patch
delete
```

for:

```text
ModelService
```

within:

```text
ai-platform
```

The exact Role manifest remains the source of truth for the current verb/resource list.

---

# 14. REST-to-Kubernetes Operation Mapping

| REST operation | Kubernetes action |
|---|---|
| `GET /api/v1/model-services` | list |
| `GET /api/v1/model-services/{name}` | get |
| `GET /api/v1/model-services/{name}/status` | get/read status |
| `POST /api/v1/model-services` | create |
| `PUT /api/v1/model-services/{name}` | update |
| `PATCH /api/v1/model-services/{name}` | patch |
| `DELETE /api/v1/model-services/{name}` | delete |

The HTTP role matrix can be stricter than the ServiceAccount's Kubernetes permissions.

For example, the ServiceAccount may technically have Kubernetes delete permission because the API must support admin DELETE, but application authorization prevents a deployer from invoking that path.

---

# 15. Why Kubernetes RBAC Cannot Express User Roles Here

All API requests reach Kubernetes using:

```text
ServiceAccount/ai-platform-api
```

Therefore Kubernetes sees one service identity, not the original Keycloak user.

Kubernetes RBAC cannot directly distinguish:

```text
platform-viewer
platform-deployer
platform-admin
```

for individual REST callers.

That distinction is enforced in the application and at Envoy.

The architecture is therefore:

```text
Keycloak identity
    |
    v
Envoy/API authorization
    |
    v
shared API ServiceAccount
    |
    v
Kubernetes RBAC
```

---

# 16. Kubernetes Client Initialization

The API initializes the controller-runtime client during startup.

The client must understand:

```text
platform.anselem.dev/v1alpha1
ModelService
```

and be able to use in-cluster configuration.

Startup or request failures can occur if:

```text
CRD missing
scheme registration incorrect
ServiceAccount missing
RBAC incorrect
Kubernetes API unreachable
NetworkPolicy blocks egress
```

---

# 17. Kubernetes API Connectivity

Relevant Kubernetes Service:

```text
10.96.0.1:443
```

The kind control-plane endpoint used in the lab is:

```text
172.19.0.7:6443
```

The API NetworkPolicy must permit the Kubernetes API path actually used by the pod.

A working ServiceAccount and Role do not help if network access to the API server is blocked.

---

# 18. DNS Dependency

The API also requires DNS for service resolution.

CoreDNS Service:

```text
10.96.0.10
```

Required DNS egress:

```text
UDP 53
TCP 53
```

DNS is particularly important for:

```text
Keycloak service discovery
internal Kubernetes service names
```

---

# 19. Read Operations

## List

The list handler asks the Kubernetes layer for `ModelService` resources in:

```text
ai-platform
```

It does not perform a cluster-wide list.

## Get

The get handler resolves:

```text
namespace=ai-platform
name={path parameter}
```

## Status

The status handler reads the resource's observed status.

The API does not generate the lifecycle status itself; the operator does.

---

# 20. Create Operation

The create path is:

```text
HTTP POST
   |
   v
authentication
   |
   v
authorization
   |
   v
validation
   |
   v
construct ModelService
   |
   v
Kubernetes create
   |
   v
HTTP 201
```

The API controls the namespace.

The caller cannot redirect the create into another namespace.

---

# 21. Update Operation

The update path is:

```text
HTTP PUT
   |
   v
authentication
   |
   v
authorization
   |
   v
validation
   |
   v
get/update ModelService
   |
   v
Kubernetes update
```

The update operates on:

```text
ai-platform/{name}
```

where `{name}` comes from the URL path.

---

# 22. Patch Operation

The patch path supports partial desired-state changes.

Conceptually:

```text
HTTP PATCH
   |
   v
validate requested change
   |
   v
target ai-platform/{name}
   |
   v
Kubernetes patch/update semantics
```

This avoids requiring callers to replace the whole resource when only part of the desired state needs to change.

---

# 23. Delete Operation

Delete is restricted at the application layer to:

```text
platform-admin
```

The Kubernetes layer performs deletion of:

```text
ModelService ai-platform/{name}
```

The operator then observes the deleted custom resource and performs any cleanup governed by owner references/finalization behavior.

A successful API delete returns:

```text
204 No Content
```

---

# 24. Error Translation

Kubernetes errors must be translated into API-facing HTTP responses.

Examples include:

```text
NotFound
AlreadyExists/conflict
validation/backend error
authorization failure
```

The HTTP layer should not expose raw internal Go/Kubernetes errors directly to callers.

The response package centralizes response behavior under:

```text
internal/api/response/
```

---

# 25. Resource Not Found

When a requested `ModelService` does not exist, the API returns a client-facing not-found response.

Expected HTTP status:

```text
404
```

This is not considered an availability failure in the SLO model.

A valid 404 means the API successfully answered that the requested resource does not exist.

---

# 26. Kubernetes Conflict

Conflicting mutations may produce a conflict response where applicable.

Expected semantic class:

```text
409 Conflict
```

This can happen in systems that use Kubernetes resource versions and optimistic concurrency.

The API should preserve the distinction between:

```text
invalid request
not found
conflict
server failure
```

---

# 27. Server-Side Kubernetes Failure

Unexpected Kubernetes failures are represented as:

```text
5xx
```

These are counted as service-side failures by the API availability/error SLI.

This connects the persistence layer to the observability model.

---

# 28. Namespace Security Check

A caller must not be able to provide a namespace such as:

```text
kube-system
monitoring
gateway-system
```

and cause the API to operate there.

The namespace boundary is enforced by the application design rather than by trusting request input.

This is one of the REST API phase's explicit security requirements.

---

# 29. ServiceAccount Verification

Check the ServiceAccount:

```bash
kubectl get serviceaccount   ai-platform-api   -n ai-platform   -o yaml
```

Confirm it exists and is referenced by the API Deployment.

---

# 30. Role Verification

Inspect the Role:

```bash
kubectl get role   ai-platform-api   -n ai-platform   -o yaml
```

Verify the Role grants only the permissions the API requires.

Look specifically at:

```text
apiGroups
resources
verbs
```

The Role should not contain unrelated broad permissions.

---

# 31. RoleBinding Verification

Inspect:

```bash
kubectl get rolebinding   ai-platform-api   -n ai-platform   -o yaml
```

Confirm:

```text
subject -> ServiceAccount/ai-platform-api
roleRef -> Role/ai-platform-api
```

---

# 32. Verify Deployment ServiceAccount

Run:

```bash
kubectl get deployment   ai-platform-api   -n ai-platform   -o jsonpath='{.spec.template.spec.serviceAccountName}'

echo
```

Expected:

```text
ai-platform-api
```

---

# 33. `kubectl auth can-i`

A useful RBAC validation technique is impersonating the ServiceAccount.

Example pattern:

```bash
kubectl auth can-i   get   modelservices.platform.anselem.dev   -n ai-platform   --as=system:serviceaccount:ai-platform:ai-platform-api
```

Expected:

```text
yes
```

Check list:

```bash
kubectl auth can-i   list   modelservices.platform.anselem.dev   -n ai-platform   --as=system:serviceaccount:ai-platform:ai-platform-api
```

Check create:

```bash
kubectl auth can-i   create   modelservices.platform.anselem.dev   -n ai-platform   --as=system:serviceaccount:ai-platform:ai-platform-api
```

Check update:

```bash
kubectl auth can-i   update   modelservices.platform.anselem.dev   -n ai-platform   --as=system:serviceaccount:ai-platform:ai-platform-api
```

Check patch:

```bash
kubectl auth can-i   patch   modelservices.platform.anselem.dev   -n ai-platform   --as=system:serviceaccount:ai-platform:ai-platform-api
```

Check delete:

```bash
kubectl auth can-i   delete   modelservices.platform.anselem.dev   -n ai-platform   --as=system:serviceaccount:ai-platform:ai-platform-api
```

These should match the Kubernetes permissions required by the API.

---

# 34. Verify No Cross-Namespace Authority

A useful negative test is:

```bash
kubectl auth can-i   list   modelservices.platform.anselem.dev   -n default   --as=system:serviceaccount:ai-platform:ai-platform-api
```

The expected result for namespace-scoped RBAC should be:

```text
no
```

This proves the Kubernetes identity itself is also namespace-limited.

---

# 35. Why a Role Is Preferred Here

A Role is preferred over a broad ClusterRole because the API manages one namespace.

Benefits:

```text
smaller blast radius
simpler review
easier reasoning
less accidental privilege
clearer security boundary
```

Cluster-wide access should only be introduced if the API contract itself changes to require it.

---

# 36. Avoid Wildcard Permissions

The API RBAC should avoid patterns like:

```yaml
apiGroups:
  - "*"
resources:
  - "*"
verbs:
  - "*"
```

Such permissions would undermine the least-privilege design.

The API only needs access to the specific resource types and verbs required by its contract.

---

# 37. CRD Availability

The client depends on the `ModelService` CRD being installed.

Check:

```bash
kubectl get crd   modelservices.platform.anselem.dev
```

If the CRD is missing, API persistence operations cannot succeed.

This should be distinguished from:

```text
RBAC denied
resource not found
network failure
```

---

# 38. ModelService Listing Validation

Direct Kubernetes verification:

```bash
kubectl get modelservices   -n ai-platform
```

Compare with API output:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"

curl   --cacert .local/keycloak/fraud-model-root-ca.crt   -sS   -H "Authorization: Bearer ${TOKEN}"   https://api.ai-platform.local/api/v1/model-services

unset TOKEN
```

The API representation may differ from raw Kubernetes YAML, but resource identity/state should correspond.

---

# 39. Fraud Model Validation

The main reference resource used during API validation is:

```text
fraud-model
```

Example direct check:

```bash
kubectl get modelservice   fraud-model   -n ai-platform
```

Example API check:

```text
GET /api/v1/model-services/fraud-model
```

This is useful for comparing API and Kubernetes views.

---

# 40. Status Validation

Direct Kubernetes status:

```bash
kubectl get modelservice   fraud-model   -n ai-platform   -o yaml
```

API:

```text
GET /api/v1/model-services/fraud-model/status
```

The operator is the component that updates status.

The API simply provides a platform-friendly read path.

---

# 41. Audit Probe Resource

A temporary resource named:

```text
audit-probe
```

was used to validate create/delete audit behavior.

Successful admin deletion was followed by:

```bash
kubectl get modelservice   audit-probe   -n ai-platform
```

Expected:

```text
NotFound
```

This validated that the API's DELETE path actually affected Kubernetes state.

---

# 42. Kubernetes Client Tests

The Kubernetes integration layer has tests around store/client behavior.

This is important because handler correctness alone does not prove Kubernetes interactions are correct.

Test coverage should verify behaviors such as:

```text
list
get
create
update
patch
delete
not-found handling
error propagation
```

The repository's Go test suite remains the source of truth for the exact current test functions.

---

# 43. Operator Interaction

The API does not wait for every operator reconciliation to finish before considering the Kubernetes write itself successful.

The conceptual separation is:

```text
API success
   =
desired state accepted into Kubernetes

Operator success
   =
desired state reconciled into runtime resources
```

The status endpoint exists to expose the latter.

This distinction is essential in asynchronous control-plane APIs.

---

# 44. Desired vs Observed State

`ModelService.spec` represents:

```text
desired state
```

`ModelService.status` represents:

```text
observed/reconciled state
```

The API mutation path writes desired state.

The operator updates observed state.

Therefore a successful POST/PUT/PATCH does not imply that every downstream workload component is immediately healthy.

---

# 45. RBAC Failure Symptoms

A Kubernetes RBAC failure may appear as:

```text
403 Forbidden
```

from the Kubernetes API internally.

This is distinct from:

```text
403
```

returned to a caller because their platform role is insufficient.

Troubleshooting must identify which layer denied the operation.

Useful evidence:

```text
API logs
audit logs
kubectl auth can-i
Role/RoleBinding
Envoy logs/policy
```

---

# 46. Application 403 vs Kubernetes 403

Two different scenarios:

## Caller authorization failure

```text
viewer -> POST
```

The API/Envoy should reject the caller before Kubernetes mutation.

## API ServiceAccount authorization failure

```text
admin -> DELETE
API tries Kubernetes delete
Kubernetes RBAC denies ServiceAccount
```

This is an internal platform configuration problem.

The same HTTP class may appear externally, but the root causes are entirely different.

---

# 47. Kubernetes Network Failure

If Kubernetes API egress is blocked, requests may fail with:

```text
timeouts
connection errors
server-side 5xx
```

Check NetworkPolicy before changing RBAC.

The required path includes:

```text
Kubernetes Service/API endpoint
TCP 443/6443 as applicable
```

---

# 48. API Pod Identity Verification

Inspect the pod:

```bash
POD="$(
  kubectl get pods     -n ai-platform     -l app.kubernetes.io/name=ai-platform-api     -o jsonpath='{.items[0].metadata.name}'
)"

echo "${POD}"
```

Then:

```bash
kubectl get pod   "${POD}"   -n ai-platform   -o jsonpath='{.spec.serviceAccountName}'

echo
```

Expected:

```text
ai-platform-api
```

---

# 49. ServiceAccount Token Requirement

Because the runtime container is distroless and read-only, Kubernetes authentication must rely on the standard projected/mounted ServiceAccount credentials.

Do not attempt to solve Kubernetes client access by:

```text
embedding kubeconfig
copying admin credentials
mounting host kubeconfig
granting cluster-admin
```

The in-cluster ServiceAccount model is the intended design.

---

# 50. RBAC and Runtime Hardening Compatibility

The API security model combines:

```text
non-root container
read-only root filesystem
ServiceAccount token
namespace-scoped Role
NetworkPolicy
```

These controls are compatible.

A hardened pod can still authenticate to Kubernetes through its projected ServiceAccount token.

---

# 51. API Security Boundary Summary

The complete request path is:

```text
Keycloak user/service identity
          |
          v
Envoy authorization
          |
          v
API application authorization
          |
          v
fixed namespace
          |
          v
ServiceAccount ai-platform-api
          |
          v
Role ai-platform-api
          |
          v
ModelService in ai-platform
```

Every layer narrows the allowed operation.

---

# 52. Why the API Does Not Use User Kubernetes Credentials

The REST API deliberately prevents platform users from needing:

```text
kubeconfig
kubectl
direct Kubernetes RBAC
```

This creates a clearer platform abstraction.

Users authenticate to:

```text
Keycloak
```

and interact with:

```text
REST API
```

instead of being granted raw cluster access.

---

# 53. Benefits of This Model

The architecture provides:

```text
centralized authentication
centralized authorization
stable API contract
namespace restriction
auditable mutations
least-privilege Kubernetes identity
operator-driven lifecycle
```

This is stronger than distributing Kubernetes credentials to every consumer.

---

# 54. Troubleshooting Checklist

When Kubernetes operations fail, check in this order:

```text
1. Is the API pod healthy?
2. Is the request authenticated?
3. Is the caller authorized?
4. Is the ServiceAccount correct?
5. Does the Role contain the required verb/resource?
6. Does the RoleBinding bind the correct ServiceAccount?
7. Is the namespace correct?
8. Is the ModelService CRD installed?
9. Can the pod reach the Kubernetes API?
10. Is NetworkPolicy blocking egress?
11. What do API logs show?
```

---

# 55. Useful Inspection Commands

ServiceAccount:

```bash
kubectl get sa   ai-platform-api   -n ai-platform   -o yaml
```

Role:

```bash
kubectl get role   ai-platform-api   -n ai-platform   -o yaml
```

RoleBinding:

```bash
kubectl get rolebinding   ai-platform-api   -n ai-platform   -o yaml
```

Deployment:

```bash
kubectl get deployment   ai-platform-api   -n ai-platform   -o yaml
```

ModelServices:

```bash
kubectl get modelservices   -n ai-platform
```

CRD:

```bash
kubectl get crd   modelservices.platform.anselem.dev
```

---

# 56. Kustomize Validation

Before applying RBAC changes:

```bash
kubectl apply   --dry-run=server   -k config/platform-api
```

Render:

```bash
kubectl kustomize   config/platform-api   >/tmp/platform-api.yaml
```

Inspect the RBAC resources before applying.

Then:

```bash
kubectl apply   -k config/platform-api
```

---

# 57. Avoiding RBAC Drift

RBAC changes should be reviewed whenever API functionality changes.

For example:

```text
new read operation
new mutation operation
new Kubernetes resource type
new namespace requirement
```

may require RBAC review.

Do not automatically add broad permissions when a new endpoint is introduced.

Instead:

```text
API contract
    ->
required Kubernetes operation
    ->
minimal RBAC change
```

---

# 58. Current Kubernetes/RBAC Completion Status

```text
[✓] controller-runtime Kubernetes client added
[✓] ModelService CRUD integrated
[✓] fixed namespace enforced
[✓] API ServiceAccount created
[✓] namespace-scoped Role created
[✓] RoleBinding created
[✓] least-privilege design applied
[✓] Kubernetes API egress allowed
[✓] ServiceAccount token intentionally mounted
[✓] read operations validated
[✓] create/update/patch/delete validated
[✓] CRUD E2E workflow passed
[✓] deleted test resource verified absent
```

The Kubernetes client and RBAC portion of the REST API phase is implementation-complete.

---

# 59. Relationship to Next Document

The next document is:

```text
04-read-endpoints.md
```

It will document:

- list endpoint
- get endpoint
- status endpoint
- authentication/authorization expectations
- response behavior
- error handling
- route normalization
- example requests
- runtime validation

---

# 60. Final Summary

The AI Platform REST API uses one controlled Kubernetes identity:

```text
ServiceAccount/ai-platform-api
```

with namespace-scoped RBAC in:

```text
ai-platform
```

The API does not expose arbitrary namespace selection and does not distribute Kubernetes credentials to platform consumers.

The resulting security model is:

```text
Keycloak identity
     |
     v
Envoy/API authorization
     |
     v
fixed ai-platform namespace
     |
     v
ServiceAccount ai-platform-api
     |
     v
least-privilege Role
     |
     v
ModelService CR
     |
     v
AI Platform Operator
```

This preserves a strong separation between user identity, API authorization, Kubernetes authorization, and operator reconciliation.
