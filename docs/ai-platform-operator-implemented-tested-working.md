# AI Platform Operator — Implemented, Tested, and Working

**Project:** `ai-platform-operator`  
**Repository path:** `/mnt/data/ai-platform-operator`  
**API group:** `platform.anselem.dev`  
**API version:** `v1alpha1`  
**Custom resource kind:** `ModelService`  
**Short name:** `ms`  
**Development cluster:** kind (`kind-ai-platform`)  
**Document status:** Baseline before production-hardening milestones  
**Last updated:** 2026-07-22

---

## 1. Purpose

This project implements a Kubernetes operator for deploying and managing model-serving workloads through a custom Kubernetes resource named `ModelService`.

A user declares the desired state through YAML:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: nginx:1.29-alpine
  replicas: 2
  port: 80
  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

The operator reads that custom resource and reconciles the following Kubernetes resources:

```text
ModelService
├── Deployment
│   └── Pods
├── Service
├── PersistentVolumeClaim
└── ModelService status
```

The operator continuously compares the desired state in `ModelService.spec` with the current cluster state and corrects drift when necessary.

---

## 2. Current Architecture

```text
User / kubectl
      |
      | creates or updates ModelService
      v
Kubernetes API Server
      |
      | reconciliation event
      v
ModelService Go Controller
      |
      +--------------------+
      |                    |
      v                    v
Deployment             ClusterIP Service
      |                    |
      v                    |
Model-serving Pods <-------+
      |
      v
PersistentVolumeClaim
      |
      v
Mounted model storage

The controller also writes:

ModelService.status
├── phase
├── readyReplicas
├── endpoint
├── observedGeneration
└── conditions
```

---

## 3. Project Structure

The important files currently used are:

```text
ai-platform-operator/
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go
│       ├── modelservice_types.go
│       └── zz_generated.deepcopy.go
├── cmd/
│   └── main.go
├── config/
│   ├── crd/
│   │   └── bases/
│   ├── rbac/
│   └── samples/
│       └── platform_v1alpha1_modelservice.yaml
├── internal/
│   └── controller/
│       ├── modelservice_controller.go
│       ├── modelservice_controller_test.go
│       └── suite_test.go
├── hack/
│   └── boilerplate.go.txt
├── Makefile
├── go.mod
└── go.sum
```

### File responsibilities

| File | Responsibility |
|---|---|
| `api/v1alpha1/modelservice_types.go` | Defines `ModelServiceSpec`, `ModelServiceStatus`, validation markers, defaults, printer columns, and status subresource |
| `api/v1alpha1/groupversion_info.go` | Registers `ModelService` and `ModelServiceList` with the runtime scheme |
| `internal/controller/modelservice_controller.go` | Implements reconciliation for Deployment, Service, PVC, and status |
| `internal/controller/modelservice_controller_test.go` | Tests controller behavior with `envtest` |
| `config/samples/platform_v1alpha1_modelservice.yaml` | Example `ModelService` resource |
| `config/crd/bases/...` | Generated CustomResourceDefinition manifest |
| `config/rbac/role.yaml` | Generated RBAC permissions |
| `cmd/main.go` | Starts the controller manager |

---

## 4. ModelService API

### 4.1 Spec fields

The custom resource supports the following desired-state fields.

#### `image`

```yaml
spec:
  image: nginx:1.29-alpine
```

Purpose:

- container image used by the Deployment;
- required field;
- validated as a non-empty string.

#### `replicas`

```yaml
spec:
  replicas: 2
```

Purpose:

- number of desired model-serving Pods;
- default: `1`;
- allowed range: `1` to `10`.

#### `port`

```yaml
spec:
  port: 80
```

Purpose:

- container port;
- Service port;
- readiness and liveness probe port;
- allowed range: `1` to `65535`.

#### `storage`

```yaml
spec:
  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

Purpose:

- enables or disables persistent storage;
- defines requested PVC size;
- defines the mount path inside the container.

Implemented fields:

| Field | Meaning |
|---|---|
| `enabled` | Whether the operator should manage a PVC and mount it |
| `size` | Requested storage quantity, such as `1Gi` |
| `mountPath` | Container path where the PVC is mounted |

---

## 5. ModelService Status

The operator writes runtime information to the status subresource.

Example:

```yaml
status:
  phase: Ready
  readyReplicas: 2
  endpoint: http://fraud-model.ai-platform.svc.cluster.local:80
  observedGeneration: 3
  conditions:
    - type: Available
      status: "True"
      reason: DeploymentAvailable
      message: Model service is ready
      observedGeneration: 3
      lastTransitionTime: "..."
```

### Status fields

| Field | Meaning |
|---|---|
| `phase` | High-level lifecycle state |
| `readyReplicas` | Number of ready replicas reported by the Deployment |
| `endpoint` | Internal Kubernetes DNS endpoint |
| `observedGeneration` | Latest `metadata.generation` processed by the controller |
| `conditions` | Kubernetes-style condition records |

### Implemented phases

#### `Provisioning`

Used when the desired number of replicas is not yet available.

Example message:

```text
Waiting for ready replicas: 0 of 1 ready
```

#### `Ready`

Used when:

- desired replicas are greater than zero;
- `readyReplicas` equals desired replicas;
- `availableReplicas` equals desired replicas.

Condition:

```yaml
type: Available
status: "True"
reason: DeploymentAvailable
message: Model service is ready
```

#### `Degraded`

Used when the Deployment reports:

```text
ProgressDeadlineExceeded
```

Condition:

```yaml
type: Available
status: "False"
reason: ProgressDeadlineExceeded
```

---

## 6. Deployment Reconciliation

Implemented in:

```text
internal/controller/modelservice_controller.go
```

The operator creates or updates one Deployment for each `ModelService`.

The Deployment uses the same name and namespace as its owning `ModelService`.

### 6.1 Replicas

The Deployment replica count is derived from:

```yaml
spec:
  replicas: 2
```

### 6.2 Container image

The Pod container image is derived from:

```yaml
spec:
  image: nginx:1.29-alpine
```

### 6.3 Container port

The operator defines one named container port:

```yaml
name: http
containerPort: 80
protocol: TCP
```

### 6.4 Health probes

The operator configures HTTP probes against:

```text
path: /
port: http
```

#### Readiness probe

Current settings:

```text
initial delay: 2 seconds
period: 5 seconds
timeout: 2 seconds
failure threshold: 3
```

#### Liveness probe

Current settings:

```text
initial delay: 10 seconds
period: 10 seconds
timeout: 2 seconds
failure threshold: 3
```

### 6.5 Stable labels

```yaml
app.kubernetes.io/name: modelservice
app.kubernetes.io/instance: <ModelService name>
app.kubernetes.io/managed-by: ai-platform-operator
```

### 6.6 Owner reference

The Deployment is controlled by the `ModelService` through `controllerutil.SetControllerReference(...)`.

---

## 7. Service Reconciliation

The operator creates or updates a `ClusterIP` Service with the same name and namespace as the `ModelService`.

### Service port

```yaml
ports:
  - name: http
    port: 80
    targetPort: http
    protocol: TCP
```

### Internal endpoint

```text
http://<name>.<namespace>.svc.cluster.local:<port>
```

Example:

```text
http://fraud-model.ai-platform.svc.cluster.local:80
```

---

## 8. PersistentVolumeClaim Reconciliation

When storage is enabled, the controller creates or updates a PVC.

```yaml
accessModes:
  - ReadWriteOnce
resources:
  requests:
    storage: 1Gi
```

When storage is disabled or absent, the controller deletes only a PVC that is controlled by the same `ModelService`.

### Current retention behavior

```text
ModelService deleted
    ↓
Owned PVC garbage-collected
    ↓
Persistent data may be deleted
```

A configurable retention policy has not yet been implemented.

---

## 9. PVC Mount in the Deployment

When storage is enabled, the operator adds:

### Volume

```yaml
volumes:
  - name: model-storage
    persistentVolumeClaim:
      claimName: fraud-model
```

### Volume mount

```yaml
volumeMounts:
  - name: model-storage
    mountPath: /models
```

---

## 10. Storage Binding Result

The local StorageClass used is `standard`.

The PVC initially showed:

```text
Status: Pending
Reason: WaitForFirstConsumer
Message: waiting for first consumer to be created before binding
```

After the Deployment mounted the PVC, the claim became:

```text
PVC: Bound
Capacity: 1Gi
Access mode: RWO
StorageClass: standard
```

Observed working state:

```text
persistentvolumeclaim/fraud-model   Bound
pod/fraud-model-...                 1/1 Running
pod/fraud-model-...                 1/1 Running
```

Verified Deployment configuration:

```json
[{"mountPath":"/models","name":"model-storage"}]
```

Verified volume configuration:

```json
[{"name":"model-storage","persistentVolumeClaim":{"claimName":"fraud-model"}}]
```

---

## 11. Reconciliation Flow

```text
1. Read ModelService
2. Reconcile Deployment
3. Reconcile Service
4. Reconcile PersistentVolumeClaim
5. Update ModelService status
6. Return
```

If the parent is deleted, the controller returns successfully and Kubernetes garbage collection handles owned children.

---

## 12. CreateOrUpdate and Drift Correction

The controller uses `controllerutil.CreateOrUpdate(...)` for:

- Deployment;
- Service;
- PVC.

This makes reconciliation idempotent.

```text
Resource missing  → create it
Resource exists   → update it toward desired state
Manual drift      → correct it on the next reconciliation
```

---

## 13. Controller Watches

`SetupWithManager` watches:

```go
For(&platformv1alpha1.ModelService{}).
    Owns(&appsv1.Deployment{}).
    Owns(&corev1.Service{}).
    Owns(&corev1.PersistentVolumeClaim{})
```

This allows changes to the parent or owned child resources to trigger reconciliation.

---

## 14. RBAC Permissions

The controller currently has generated permissions for:

- `ModelService` resources;
- `ModelService/status`;
- `ModelService/finalizers`;
- Deployments;
- Services;
- PersistentVolumeClaims.

Finalizer permissions exist, but custom finalizer logic has not yet been implemented.

---

## 15. Scheme Registration Fix

The API registration was corrected for the Kubebuilder scaffold by registering `ModelService` and `ModelServiceList` with `runtime.NewSchemeBuilder(...)` and `scheme.AddKnownTypes(...)`.

This resolved the initial scheme registration problem.

---

## 16. Generated CRD and API Discovery

Installed CRD:

```text
modelservices.platform.anselem.dev
```

API discovery confirmed:

```text
RESOURCE        SHORTNAMES   APIVERSION                         NAMESPACED   KIND
modelservices   ms           platform.anselem.dev/v1alpha1      true         ModelService
```

---

## 17. Working Sample Resource

File:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: nginx:1.29-alpine
  replicas: 2
  port: 80
  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

### Image and port compatibility lesson

NGINX listens on port `80`. Using `port: 8080` caused readiness and liveness failures. After changing the custom resource to `port: 80` and reapplying it, the Pods became healthy.

---

## 18. Automated Tests

Test file:

```text
internal/controller/modelservice_controller_test.go
```

Testing stack:

- Ginkgo;
- Gomega;
- controller-runtime `envtest`.

### What envtest runs

- Kubernetes API server;
- etcd.

### What envtest does not run

- scheduler;
- kubelet;
- Deployment controller;
- real Pods;
- storage provisioner.

Therefore, the expected status in automated tests is `Provisioning`, not `Ready`.

### Tested Deployment behavior

- Deployment creation;
- requested replica count;
- one model container;
- image propagation;
- container port propagation;
- PVC volume mount;
- PVC-backed Pod volume;
- owner reference.

### Tested Service behavior

- Service creation;
- `ClusterIP` type;
- `http` port name;
- Service port;
- named target port;
- selector labels;
- owner reference.

### Tested PVC behavior

- PVC creation;
- `ReadWriteOnce` access mode;
- `1Gi` request;
- owner reference.

### Tested status behavior

- phase is `Provisioning` in envtest;
- ready replicas are `0`;
- endpoint is generated correctly;
- observed generation is current;
- one `Available` condition exists;
- reason is `DeploymentNotReady`;
- the condition message reports ready replicas.

### Cleanup

Each test cleans up:

- ModelService;
- Deployment;
- Service;
- PVC.

---

## 19. Build and Test Results

The following succeeded:

```bash
go fmt ./...
go vet ./...
go build -o bin/manager cmd/main.go
make test
```

Observed controller test coverage:

```text
68.9% of statements
```

Observed package result:

```text
ok github.com/anselem-okeke/ai-platform-operator/internal/controller
```

---

## 20. Live Cluster Verification

The implementation was tested in the kind cluster.

Verified:

- Deployment exists;
- Service exists;
- PVC is `Bound`;
- Pods are `Running` and `Ready`;
- volume is mounted at `/models`;
- Deployment references the correct PVC;
- the controller reconciles the working stack.

Useful verification command:

```bash
kubectl get ms,deploy,service,pvc,pods -n ai-platform
```

---

## 21. Implementation Versus Tests

| Component | Responsibility |
|---|---|
| `modelservice_controller.go` | Implements real behavior |
| `modelservice_controller_test.go` | Verifies controller behavior |
| Kubernetes Deployment controller | Creates and rolls out Pods |
| Scheduler | Places Pods on nodes |
| Kubelet | Runs containers and probes |
| Service controller / EndpointSlice logic | Routes Service traffic |
| Storage provisioner | Binds the PVC |
| Garbage collector | Deletes owned children after parent deletion |

The tests do not implement production functionality. They only prove that the controller creates the expected Kubernetes objects.

---

## 22. Current Known Limitations

### Not yet implemented: workload security

- non-root execution;
- Pod security context;
- container security context;
- read-only root filesystem;
- capability dropping;
- seccomp profile;
- restricted service account.

### Not yet implemented: resource management

- CPU requests and limits;
- memory requests and limits.

### Not yet implemented: availability controls

- startup probe;
- PodDisruptionBudget;
- anti-affinity;
- topology spread constraints;
- configurable rollout strategy;
- graceful termination.

### Not yet implemented: network security

- NetworkPolicy;
- Ingress or Gateway API;
- TLS;
- authentication;
- authorization.

### Not yet implemented: advanced storage lifecycle

- `Retain` versus `Delete` policy;
- finalizer-based cleanup;
- PVC resize rules;
- immutable-field handling.

### Not yet implemented: observability

- custom Prometheus metrics;
- ServiceMonitor;
- Kubernetes Events;
- SLOs;
- alerts;
- dashboards.

### Not yet implemented: delivery and AI integrations

- operator image deployment;
- Helm;
- Argo CD;
- Kargo;
- MLflow;
- KServe;
- object storage;
- GPU support;
- autoscaling.

---

## 23. Current Roadmap Position

### Phase 1 — Core operator foundation

**Status: complete**

```text
[✓] Kubebuilder scaffolding
[✓] ModelService API
[✓] CRD generation and installation
[✓] validation and defaults
[✓] Deployment reconciliation
[✓] readiness and liveness probes
[✓] Service reconciliation
[✓] PVC reconciliation
[✓] storage mount
[✓] status reconciliation
[✓] owner references
[✓] child watches
[✓] controller tests
[✓] live kind verification
```

### Phase 2 — Production-grade workload configuration

**Status: next**

```text
[ ] CPU and memory requests
[ ] CPU and memory limits
[ ] Pod security context
[ ] container security context
[ ] non-root execution
[ ] read-only root filesystem
[ ] capability dropping
[ ] seccomp
[ ] startup probe
[ ] graceful termination
[ ] PodDisruptionBudget
```

---

## 24. Useful Commands

Run from:

```text
/mnt/data/ai-platform-operator
```

```bash
make generate
make manifests
make build
make test
make install
make run
```

Apply the sample:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Inspect the complete stack:

```bash
kubectl get ms,deploy,service,pvc,pods \
  -n ai-platform
```

Inspect status:

```bash
kubectl get ms fraud-model \
  -n ai-platform \
  -o yaml
```

Test Service routing:

```bash
kubectl run curl-test \
  --rm -it \
  --restart=Never \
  --image=curlimages/curl \
  -n ai-platform \
  -- curl -sS http://fraud-model
```

---

## 25. Definition of Done for the Completed Foundation

The foundation is complete because:

- the CRD installs successfully;
- Kubernetes discovers `ModelService`;
- the sample custom resource applies successfully;
- the operator creates a Deployment;
- Pods run with a compatible image and port;
- the Service is created and routes internally;
- the PVC is created and becomes `Bound`;
- storage mounts at the configured path;
- status fields are populated;
- child resources have owner references;
- child changes can trigger reconciliation;
- generated code and manifests succeed;
- `go vet` succeeds;
- the manager builds;
- controller tests pass;
- the kind-cluster verification succeeds.

---

## 26. Baseline Summary

The project is now a functioning Kubernetes operator, not only a Kubebuilder scaffold.

It currently supports:

```text
Create
├── Deployment
├── Service
├── PersistentVolumeClaim
└── status

Update
├── image
├── replicas
├── port
├── mount path
└── storage request

Observe
├── phase
├── ready replicas
├── endpoint
├── observed generation
└── conditions

Self-heal
└── restore owned resources toward ModelService.spec

Delete
└── Kubernetes garbage-collects owned children
```

This document is the implementation baseline before beginning production-hardening work.
