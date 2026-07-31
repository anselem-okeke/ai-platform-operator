# AI Platform Operator — Reproducible Setup and Progress Guide

This guide documents the complete setup and implementation performed so far for the **AI Platform Operator** project.

Its purpose is to let another engineer reproduce the environment, understand the architecture, scaffold the Kubernetes operator, define the `ModelService` custom resource, install and validate the CRD, and reach the same development checkpoint.

---

## 1. Project goal

We are building a small Kubernetes-based AI platform control plane.

The first working version follows this design:

```text
External user or client
        |
        v
Platform REST API              # added later
        |
        v
Kubernetes API Server
        |
        v
ModelService Custom Resource
        |
        v
Go Kubernetes Operator
        |
        +--> Deployment
        +--> Service
        +--> PersistentVolumeClaim
        +--> Status and conditions
```

Later phases will add:

- Deployment reconciliation
- Service reconciliation
- persistent storage
- lifecycle status
- REST API
- authentication and authorization
- audit logging
- Prometheus metrics
- Helm packaging
- Argo CD GitOps deployment
- KServe integration
- MLflow integration

---

## 2. Architecture

Most platform components will run inside Kubernetes.

```text
External
+-------------------+
| User / Client     |
+---------+---------+
          |
          | HTTPS
          v

Kubernetes cluster
+--------------------------------------------------+
| Ingress / API Gateway                            |
|              |                                   |
|              v                                   |
| Platform REST API                                |
|              |                                   |
|              v                                   |
| Kubernetes API Server                            |
|              |                                   |
|              v                                   |
| ModelService Custom Resource                     |
|              |                                   |
|              v                                   |
| Go Operator                                      |
|      +-----------+-----------+-----------+        |
|      |           |           |           |        |
|      v           v           v           v        |
| Deployment    Service       PVC        Status     |
+--------------------------------------------------+
```

### Kubernetes control plane versus AI platform control plane

These are different concepts.

**Kubernetes control plane**:

- API server
- scheduler
- controller manager
- etcd

**AI platform control plane**:

- REST API
- validation
- authentication and authorization
- lifecycle management
- audit logging
- custom resources
- operator reconciliation

This repository implements the **AI platform control plane**, not Kubernetes itself.

---

# Part I — Tool installation

## 3. Development environment

The environment used for this project currently has:

```text
Go:         go1.26.4 linux/amd64
Docker:     29.6.2
kubectl:    v1.36.2
Kustomize:  v5.8.1
kind:       v0.32.0
Helm:       v4.2.3
Git:        2.43.0
```

The kind context is:

```text
kind-ai-platform
```

The Kubernetes API server is reachable at:

```text
https://0.0.0.0:6443
```

> Binding the API server to `0.0.0.0` is suitable only for an isolated lab or a host protected by firewall rules. Do not expose port 6443 publicly without strong network controls.

---

## 4. Install Go

The Ubuntu package repository offered an older release, so Go was installed from the official archive.

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.26.4.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.4.linux-amd64.tar.gz
```

Add Go to `PATH`:

```bash
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

Verify:

```bash
go version
```

Expected:

```text
go version go1.26.4 linux/amd64
```

---

## 5. Install kubectl

```bash
cd /tmp

curl -LO "https://dl.k8s.io/release/$(curl -L -s \
  https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"

curl -LO "https://dl.k8s.io/release/$(curl -L -s \
  https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl.sha256"
```

Verify the checksum:

```bash
echo "$(cat kubectl.sha256)  kubectl" | sha256sum --check
```

Install:

```bash
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

Verify:

```bash
kubectl version --client
```

---

## 6. Install kind

Because Docker was already installed, kind was used to run Kubernetes nodes as containers.

```bash
go install sigs.k8s.io/kind@v0.32.0
source ~/.bashrc
kind version
```

Expected:

```text
kind v0.32.0 go1.26.4 linux/amd64
```

---

## 7. Install Helm

```bash
curl -fsSL -o /tmp/get_helm.sh \
  https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-4
chmod 700 /tmp/get_helm.sh
/tmp/get_helm.sh
```

Verify:

```bash
helm version
```

---

## 8. Verify all tools

```bash
go version
docker version --format '{{.Client.Version}}'
kubectl version --client
kind version
helm version
git --version
```

Example successful output:

```text
go version go1.26.4 linux/amd64
29.6.2
Client Version: v1.36.2
Kustomize Version: v5.8.1
kind v0.32.0 go1.26.4 linux/amd64
version.BuildInfo{Version:"v4.2.3", ...}
git version 2.43.0
```

---

# Part II — Create the Kubernetes cluster

## 9. Create the project directory

```bash
mkdir -p /mnt/data/ai-platform-operator
cd /mnt/data/ai-platform-operator
```

---

## 10. Create the kind configuration

Create `kind-cluster.yaml`:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

name: ai-platform

nodes:
  - role: control-plane
  - role: worker
```

Create the cluster:

```bash
kind create cluster --config kind-cluster.yaml
```

Verify:

```bash
kubectl cluster-info --context kind-ai-platform
kubectl get nodes -o wide
kubectl config current-context
```

Expected context:

```text
kind-ai-platform
```

Expected nodes:

```text
ai-platform-control-plane   Ready   control-plane
ai-platform-worker          Ready   <none>
```

---

# Part III — Install and initialize Kubebuilder

## 11. Install Kubebuilder

```bash
cd /tmp
curl -L -o kubebuilder \
  "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder
sudo mv kubebuilder /usr/local/bin/kubebuilder
```

Verify:

```bash
kubebuilder version
```

---

## 12. Initialize the repository

```bash
cd /mnt/data/ai-platform-operator
git init

kubebuilder init \
  --domain anselem.dev \
  --repo github.com/anselem-okeke/ai-platform-operator
```

The base scaffold includes:

```text
ai-platform-operator/
├── cmd/
│   └── main.go
├── config/
├── internal/
│   └── controller/
├── Dockerfile
├── Makefile
├── PROJECT
├── go.mod
├── go.sum
└── README.md
```

---

## 13. Create the ModelService API and controller

```bash
kubebuilder create api \
  --group platform \
  --version v1alpha1 \
  --kind ModelService
```

Answer:

```text
Create Resource [y/n]: y
Create Controller [y/n]: y
```

The resulting API is:

```text
platform.anselem.dev/v1alpha1
```

The resource kind is:

```text
ModelService
```

---

# Part IV — Repository structure

## 14. Current structure

The repository currently contains:

```text
ai-platform-operator/
├── AGENTS.md
├── api
│   └── v1alpha1
│       ├── groupversion_info.go
│       ├── modelservice_types.go
│       └── zz_generated.deepcopy.go
├── bin
│   ├── controller-gen-v0.21.0
│   └── manager
├── cmd
│   └── main.go
├── config
│   ├── crd
│   ├── default
│   ├── manager
│   ├── network-policy
│   ├── prometheus
│   ├── rbac
│   └── samples
├── cover.out
├── Dockerfile
├── go.mod
├── go.sum
├── hack
│   └── boilerplate.go.txt
├── img
│   └── ai-platform-arc.png
├── internal
│   └── controller
│       ├── modelservice_controller.go
│       ├── modelservice_controller_test.go
│       └── suite_test.go
├── kind-cluster.yaml
├── Makefile
├── PROJECT
├── README.md
└── test
    ├── e2e
    └── utils
```

### Purpose of the main directories

`api/v1alpha1/` contains the API types and generated deep-copy code.

`internal/controller/` contains reconciliation logic and controller tests.

`config/` contains CRD, RBAC, deployment, metrics, network-policy and sample manifests.

`bin/` contains generated tooling and the compiled manager binary.

Do not edit `zz_generated.deepcopy.go` or generated binaries manually.

---

# Part V — Define the ModelService API

## 15. Desired and observed state

The desired state includes:

- image
- replicas
- port
- optional storage

The observed state includes:

- phase
- ready replicas
- endpoint
- observed generation
- conditions

The implementation belongs in:

```text
api/v1alpha1/modelservice_types.go
```

Core type definitions:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ModelServiceStorage struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// +kubebuilder:default="1Gi"
	// +kubebuilder:validation:Pattern=`^[0-9]+(Mi|Gi|Ti)$`
	Size string `json:"size,omitempty"`

	// +kubebuilder:default="/models"
	// +kubebuilder:validation:Pattern=`^/.*`
	MountPath string `json:"mountPath,omitempty"`
}

type ModelServiceSpec struct {
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Replicas int32 `json:"replicas,omitempty"`

	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	Storage *ModelServiceStorage `json:"storage,omitempty"`
}

type ModelServiceStatus struct {
	Phase              string             `json:"phase,omitempty"`
	ReadyReplicas      int32              `json:"readyReplicas,omitempty"`
	Endpoint           string             `json:"endpoint,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`

	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ms
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ModelService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelServiceSpec   `json:"spec"`
	Status ModelServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ModelServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelService `json:"items"`
}
```

---

## 16. Why `ObservedGeneration` matters

Kubernetes increments:

```text
metadata.generation
```

whenever `spec` changes.

The controller will record the generation it processed in:

```text
status.observedGeneration
```

Example:

```text
metadata.generation:        4
status.observedGeneration:  3
```

This means the status still reflects an older specification.

---

# Part VI — Scheme registration

## 17. Compiler problem encountered

This registration pattern failed:

```go
func init() {
	SchemeBuilder.Register(&ModelService{}, &ModelServiceList{})
}
```

Compiler error:

```text
cannot use &ModelService{} as func(*runtime.Scheme) error
```

The generated scaffold uses `runtime.NewSchemeBuilder`, whose `Register` method accepts functions rather than API objects.

---

## 18. Correct registration

Update `api/v1alpha1/groupversion_info.go`:

```go
var (
	SchemeGroupVersion = schema.GroupVersion{
		Group:   "platform.anselem.dev",
		Version: "v1alpha1",
	}

	GroupVersion = SchemeGroupVersion

	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(
			SchemeGroupVersion,
			&ModelService{},
			&ModelServiceList{},
		)

		metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
		return nil
	})

	AddToScheme = SchemeBuilder.AddToScheme
)
```

Remove the invalid `init()` registration function from `modelservice_types.go`.

---

# Part VII — Sample custom resource

## 19. Sample YAML

File:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

Content:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: nginx:1.29-alpine
  replicas: 2
  port: 8080
  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

For the upcoming NGINX Deployment test, use port `80`, because the standard NGINX image listens on port 80:

```bash
sed -i 's/port: 8080/port: 80/' \
  config/samples/platform_v1alpha1_modelservice.yaml
```

---

# Part VIII — Generate, build and test

## 20. Generate helper code

```bash
make generate
```

This regenerates deep-copy methods.

---

## 21. Generate manifests

```bash
make manifests
```

This regenerates CRD and RBAC YAML.

---

## 22. Synchronize dependencies

```bash
go mod tidy
```

---

## 23. Build

```bash
make build
```

This runs formatting, vetting and compilation. The manager binary is created at:

```text
bin/manager
```

---

# Part IX — Controller test fixes

## 24. Required image validation

The generated test originally created a `ModelService` without `spec.image`.

The API server correctly rejected it:

```text
spec.image: Invalid value: "": spec.image in body should be at least 1 chars long
```

Update the test resource in:

```text
internal/controller/modelservice_controller_test.go
```

Use:

```go
resource := &platformv1alpha1.ModelService{
	ObjectMeta: metav1.ObjectMeta{
		Name:      resourceName,
		Namespace: resourceNamespace,
	},
	Spec: platformv1alpha1.ModelServiceSpec{
		Image:    "nginx:1.29-alpine",
		Replicas: 1,
		Port:     8080,
	},
}
```

### Unresolved `namespace` reference

The test defines:

```go
const (
	resourceName      = "test-resource"
	resourceNamespace = "default"
)
```

Therefore this is wrong:

```go
Namespace: namespace
```

and this is correct:

```go
Namespace: resourceNamespace
```

Run:

```bash
go fmt ./internal/controller/...
make test
```

---

# Part X — Install and validate the CRD

## 25. Install the CRD

```bash
make install
```

Successful output:

```text
customresourcedefinition.apiextensions.k8s.io/modelservices.platform.anselem.dev created
```

Verify:

```bash
kubectl get crd modelservices.platform.anselem.dev
kubectl api-resources | grep -i modelservice
```

Expected API discovery:

```text
modelservices   ms   platform.anselem.dev/v1alpha1   true   ModelService
```

This confirms:

- plural resource: `modelservices`
- short name: `ms`
- API group/version: `platform.anselem.dev/v1alpha1`
- namespaced resource
- kind: `ModelService`

---

## 26. Create the namespace

```bash
kubectl create namespace ai-platform
```

If it already exists:

```bash
kubectl get namespace ai-platform
```

---

## 27. Test schema validation

```bash
cat <<'YAML' | kubectl apply -f -
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: invalid-model
  namespace: ai-platform
spec:
  image: nginx:1.29-alpine
  replicas: 50
  port: 8080
YAML
```

Expected rejection:

```text
spec.replicas: Invalid value: 50
must be less than or equal to 10
```

This validation is enforced by the Kubernetes API server before reconciliation.

---

## 28. Apply the valid sample

```bash
kubectl apply -f config/samples/platform_v1alpha1_modelservice.yaml
```

Inspect:

```bash
kubectl get modelservices -n ai-platform
kubectl get ms -n ai-platform
kubectl get modelservice fraud-model -n ai-platform -o yaml
```

At the current checkpoint, status columns may be empty because status reconciliation is not implemented yet.

---

# Part XI — Current controller state

## 29. Current reconciliation behavior

The generated controller currently watches `ModelService` resources but contains placeholder logic:

```go
func (r *ModelServiceReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}
```

It does not yet create:

- Deployment
- Service
- PVC
- status

---

# Part XII — Next milestone

## 30. Deployment reconciliation

The next implementation step is:

```text
ModelService event
        |
        v
Reconcile()
        |
        +--> read ModelService
        +--> construct desired Deployment
        +--> create Deployment when missing
        +--> update Deployment when spec changes
        +--> set owner reference
        +--> watch owned Deployment changes
```

Required RBAC markers:

```go
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
```

The manager will watch both the parent and owned Deployments:

```go
return ctrl.NewControllerManagedBy(mgr).
	For(&platformv1alpha1.ModelService{}).
	Owns(&appsv1.Deployment{}).
	Named("modelservice").
	Complete(r)
```

The first Deployment implementation will include:

- image
- replicas
- container port
- labels and selector
- readiness probe
- liveness probe
- owner reference

---

# Part XIII — Roadmap

## 31. Development milestones

```text
M1  Operator scaffolded                    complete
M2  ModelService API scaffolded            complete
M3  Real spec and status defined           complete
M4  CRD installed and validated            complete
M5  Deployment reconciliation              next
M6  Service reconciliation
M7  PVC reconciliation
M8  Lifecycle status and conditions
M9  Update and deletion behavior
M10 REST API
M11 Authentication and audit logging
M12 Prometheus metrics
M13 Helm chart
M14 Argo CD deployment
M15 KServe integration
M16 MLflow integration
```

---

## 32. Final target structure

Do not create the whole final tree yet. Add directories only when their implementation begins.

```text
ai-platform-operator/
├── api/
│   └── v1alpha1/
├── cmd/
│   ├── operator/
│   │   └── main.go
│   └── api/
│       └── main.go
├── internal/
│   ├── controller/
│   ├── platform/
│   ├── resources/
│   └── httpapi/
├── config/
│   ├── crd/
│   ├── default/
│   ├── manager/
│   ├── prometheus/
│   ├── rbac/
│   └── samples/
├── charts/
│   └── ai-platform/
├── deploy/
│   ├── dev/
│   ├── staging/
│   └── production/
├── test/
│   ├── integration/
│   └── e2e/
├── Dockerfile.operator
├── Dockerfile.api
├── Makefile
├── PROJECT
├── go.mod
├── go.sum
└── README.md
```

Recommended implementation order:

```text
Kubebuilder scaffold
    |
    v
ModelService CRD
    |
    v
Deployment controller
    |
    v
Service and PVC
    |
    v
Status
    |
    v
REST API
    |
    v
Separate binaries and Dockerfiles
    |
    v
Helm
    |
    v
Argo CD
    |
    v
KServe and MLflow
```

---

# Part XIV — Reproduction checklist

## 33. Main command sequence

```bash
# Verify tools
go version
docker version --format '{{.Client.Version}}'
kubectl version --client
kind version
helm version
git --version

# Enter project
cd /mnt/data/ai-platform-operator

# Create cluster
kind create cluster --config kind-cluster.yaml

# Verify cluster
kubectl cluster-info --context kind-ai-platform
kubectl get nodes -o wide
kubectl config current-context

# Initialize project
git init
kubebuilder init \
  --domain anselem.dev \
  --repo github.com/anselem-okeke/ai-platform-operator

kubebuilder create api \
  --group platform \
  --version v1alpha1 \
  --kind ModelService

# After editing the API types and scheme registration
make generate
make manifests
go mod tidy
make build
make test

# Install CRD
make install

# Verify CRD
kubectl get crd modelservices.platform.anselem.dev
kubectl api-resources | grep -i modelservice

# Create namespace
kubectl create namespace ai-platform

# Apply resource
kubectl apply -f config/samples/platform_v1alpha1_modelservice.yaml

# Inspect
kubectl get ms -n ai-platform
kubectl get modelservice fraud-model -n ai-platform -o yaml
```

---

# Part XV — Troubleshooting summary

## 34. `go` command not found

```bash
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

## 35. `kind` command not found

```bash
source ~/.bashrc
which kind
kind version
```

## 36. SchemeBuilder registration error

Symptom:

```text
cannot use &ModelService{} as func(*runtime.Scheme) error
```

Fix: register types through `scheme.AddKnownTypes(...)` inside the function passed to `runtime.NewSchemeBuilder`.

## 37. Empty image rejected in tests

Symptom:

```text
spec.image: Invalid value: ""
```

Fix: provide a valid `Spec.Image` in the test object.

## 38. Unresolved `namespace`

Use:

```go
Namespace: resourceNamespace
```

not:

```go
Namespace: namespace
```

## 39. NGINX probe failure

The standard NGINX image listens on port 80. Use:

```yaml
port: 80
```

for the initial Deployment reconciliation test.

---

# Conclusion

At the current checkpoint:

- all required tools are installed
- the kind cluster is running
- the Kubebuilder repository is scaffolded
- the `ModelService` API is defined
- validation and defaults are generated
- Go types are correctly registered
- the project builds
- controller tests were corrected for required fields
- the CRD is installed
- Kubernetes recognizes `ModelService`
- the next step is Deployment reconciliation

Current flow:

```text
kubectl apply
      |
      v
Kubernetes API Server
      |
      v
ModelService Custom Resource
      |
      v
Stored in Kubernetes
```

Next flow:

```text
ModelService
      |
      v
Go Operator
      |
      v
Deployment
```
