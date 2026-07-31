# AI Platform Operator Setup Guide

This guide explains the complete setup of the `ai-platform-operator` project, including:

- installing Go, kubectl, kind, Helm, and Make;
- creating a local Kubernetes cluster with kind;
- initializing a Kubebuilder project;
- creating the `ModelService` API and controller;
- understanding every generated folder and file;
- understanding `make generate`, `make manifests`, `make build`, and `make test`;
- handling the `/mnt/data` symbolic-link limitation safely.

---

# 1. What We Are Building

We are building a Kubernetes Operator called:

```text
ai-platform-operator
```

The operator introduces a new Kubernetes resource called:

```text
ModelService
```

A future user will be able to create a resource similar to:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-detector
spec:
  image: example/model-server:v1
  replicas: 2
```

The operator will watch this resource and create the Kubernetes objects needed to run the model-serving application.

The planned reconciliation flow is:

```text
ModelService custom resource
        |
        v
ModelService controller
        |
        +--> Deployment
        +--> Service
        +--> configuration
        +--> status updates
```

---

# 2. Required Development Tools

The project uses the following tools:

| Tool | Purpose |
|---|---|
| Go | Compiles the operator and manages Go dependencies |
| Docker | Runs kind Kubernetes nodes as containers and builds images |
| kubectl | Communicates with the Kubernetes API server |
| kind | Creates a local Kubernetes cluster using Docker containers |
| Helm | Packages and installs Kubernetes applications |
| Git | Tracks source-code changes |
| Make | Runs the project build commands from the Makefile |
| Kubebuilder | Creates the operator project structure and controller scaffolding |
| controller-gen | Generates Kubernetes DeepCopy code, CRDs, and RBAC manifests |
| Kustomize | Combines Kubernetes YAML files for deployment |
| setup-envtest | Provides Kubernetes API-server and etcd binaries for controller tests |

---

# 3. Step 1: Install Go

The Ubuntu package repository may provide an older Go version. The official Go binary installation places Go under:

```text
/usr/local/go
```

Run:

```bash
cd /tmp

curl -LO https://go.dev/dl/go1.26.4.linux-amd64.tar.gz

sudo rm -rf /usr/local/go

sudo tar -C /usr/local -xzf go1.26.4.linux-amd64.tar.gz
```

Add Go and Go-installed programs to your `PATH`:

```bash
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

Verify:

```bash
go version
```

Expected output:

```text
go version go1.26.4 linux/amd64
```

## Why `$HOME/go/bin` is needed

When you install a Go command using:

```bash
go install <module>@<version>
```

Go normally places the executable in:

```text
$HOME/go/bin
```

Adding this directory to `PATH` allows commands such as `kind` and `kubebuilder` to run from any directory.

---

# 4. Step 2: Install kubectl

`kubectl` is the command-line tool used to communicate with the Kubernetes API server.

Run:

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

Expected:

```text
kubectl: OK
```

Install it:

```bash
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

Verify:

```bash
kubectl version --client
```

## Why kubectl is important

Later, kubectl will be used to:

```bash
kubectl get nodes
kubectl get pods
kubectl apply -f <file>
kubectl get modelservices
kubectl describe modelservice <name>
```

It is the main command-line interface between you and Kubernetes.

---

# 5. Step 3: Install kind

kind means:

```text
Kubernetes IN Docker
```

It creates Kubernetes nodes as Docker containers.

Install it using Go:

```bash
go install sigs.k8s.io/kind@v0.32.0
```

Verify:

```bash
kind version
```

If the command is not found:

```bash
source ~/.bashrc
which kind
kind version
```

## Why kind is useful

It provides a local Kubernetes environment without requiring a cloud provider.

The architecture is:

```text
Linux server
    |
    v
Docker
    |
    +--> control-plane container
    +--> worker container
    |
    v
Local Kubernetes cluster
```

---

# 6. Step 4: Install Helm

Helm is the Kubernetes package manager.

Run:

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

## Why Helm is important

Later, Helm can package and install the operator and its dependent resources.

A Helm chart can contain:

```text
operator Deployment
ServiceAccount
RBAC
CRDs
Services
ConfigMaps
Values
```

---

# 7. Step 5: Install Make

Kubebuilder generates a `Makefile`. The Makefile contains commands for generating, compiling, testing, installing, and deploying the operator.

Install Make:

```bash
sudo apt update
sudo apt install -y make
```

Verify:

```bash
make --version
```

## Why Make is important

`make` does not generate CRDs or compile Go by itself. It reads instructions from the project `Makefile` and runs the correct underlying tools.

For example:

```bash
make generate
```

may internally run `controller-gen`.

```bash
make build
```

may internally run generation checks and `go build`.

---

# 8. Step 6: Verify the Main Tools

Run:

```bash
go version
docker version --format '{{.Client.Version}}'
kubectl version --client
kind version
helm version
git --version
make --version
```

At this point, the machine has the basic development environment required for the operator project.

---

# 9. Step 7: Create the kind Cluster

Create the project directory:

```bash
mkdir -p ~/projects/ai-platform-operator
cd ~/projects/ai-platform-operator
```

Create `kind-cluster.yaml`:

```bash
cat > kind-cluster.yaml <<'EOF_KIND'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

name: ai-platform

nodes:
  - role: control-plane
  - role: worker
EOF_KIND
```

Create the cluster:

```bash
kind create cluster --config kind-cluster.yaml
```

kind performs the following operations:

```text
Pull Kubernetes node image
        |
        v
Create Docker containers
        |
        +--> ai-platform-control-plane
        +--> ai-platform-worker
        |
        v
Start Kubernetes control-plane components
        |
        v
Write kubeconfig context
```

Verify:

```bash
kubectl cluster-info --context kind-ai-platform
kubectl get nodes -o wide
```

Expected structure:

```text
NAME                        STATUS   ROLES
ai-platform-control-plane   Ready    control-plane
ai-platform-worker          Ready    <none>
```

---

# 10. Installing Kubebuilder

Kubebuilder is the project scaffolding tool used to create the operator repository.

A Kubebuilder installation normally provides the command:

```bash
kubebuilder version
```

Kubebuilder creates:

- the Go project structure;
- the controller-runtime manager;
- the Dockerfile;
- Kubernetes deployment configuration;
- RBAC configuration;
- testing structure;
- Makefile targets;
- CRD and controller scaffolding.

---

# 11. Initializing the Kubebuilder Project

A project is normally initialized with a command similar to:

```bash
kubebuilder init \
  --domain anselem.dev \
  --repo <your-go-module>
```

This creates the base project structure.

The domain:

```text
anselem.dev
```

later becomes part of the Kubernetes API group:

```text
platform.anselem.dev
```

The base project created by `kubebuilder init` includes folders such as:

```text
cmd/
config/
hack/
internal/
test/
```

and files such as:

```text
Dockerfile
Makefile
PROJECT
go.mod
go.sum
README.md
```

---

# 12. Creating the ModelService API and Controller

The custom resource and controller were created with:

```bash
kubebuilder create api \
  --group platform \
  --version v1alpha1 \
  --kind ModelService
```

Kubebuilder then asked:

```text
Create Resource [y/n]
Create Controller [y/n]
```

Answering `y` to both created:

```text
api/v1alpha1/modelservice_types.go
api/v1alpha1/groupversion_info.go
internal/controller/modelservice_controller.go
internal/controller/modelservice_controller_test.go
internal/controller/suite_test.go
config/samples/platform_v1alpha1_modelservice.yaml
```

It also updated:

```text
cmd/main.go
PROJECT
config/rbac/
```

The generated API identity is:

```text
Group:   platform.anselem.dev
Version: v1alpha1
Kind:    ModelService
```

Therefore the resource uses:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
```

---

# 13. Why the First Kubebuilder Command Failed

The API and controller files were created successfully, but Kubebuilder then attempted to run:

```bash
make generate
```

The machine initially did not have `make` installed, so the final post-scaffolding step failed.

This meant:

```text
API files created          successful
Controller files created   successful
PROJECT updated            successful or mostly successful
make generate              failed because Make was missing
```

The project did not need to be recreated. Only the missing build step had to be completed.

---

# 14. The `/mnt/data` Symbolic-Link Problem

The project currently lives at:

```text
/mnt/data/ai-platform-operator
```

The Kubebuilder Makefile normally stores helper tools under:

```text
/mnt/data/ai-platform-operator/bin
```

It downloaded a versioned binary such as:

```text
bin/controller-gen-v0.21.0
```

Then it attempted to create a symbolic link:

```text
bin/controller-gen -> controller-gen-v0.21.0
```

The filesystem backing `/mnt/data` returned:

```text
ln: failed to create symbolic link: Input/output error
```

This is a filesystem limitation. It is not a Go-code or Kubebuilder-code failure.

Some shared or mounted filesystems do not support Linux symbolic links in the same way as a native Linux filesystem.

---

# 15. Keeping the Project in `/mnt/data`

The project can remain in:

```text
/mnt/data/ai-platform-operator
```

Only Kubebuilder helper tools are redirected to a native Linux directory:

```text
/home/ansible/.local/kubebuilder/bin
```

Run:

```bash
cd /mnt/data/ai-platform-operator

export LOCALBIN="$HOME/.local/kubebuilder/bin"
mkdir -p "$LOCALBIN"
```

Then use normal Make commands:

```bash
make generate
make manifests
go mod tidy
make build
make test
```

The layout becomes:

```text
Project source and generated project files:
/mnt/data/ai-platform-operator

Kubebuilder helper executables:
/home/ansible/.local/kubebuilder/bin
```

Nothing in the repository is moved or deleted.

## Make the setting persistent

Add the variable to `.bashrc`:

```bash
echo 'export LOCALBIN="$HOME/.local/kubebuilder/bin"' >> ~/.bashrc
source ~/.bashrc
```

After that, you can enter the project and run normal commands:

```bash
cd /mnt/data/ai-platform-operator
make generate
make manifests
make build
make test
```

---

# 16. Understanding `make generate`

Run:

```bash
make generate
```

This generates supporting Go code for Kubernetes API types.

Main input:

```text
api/v1alpha1/modelservice_types.go
```

Main output:

```text
api/v1alpha1/zz_generated.deepcopy.go
```

The generated file contains methods such as:

```go
func (in *ModelService) DeepCopy() *ModelService
```

and:

```go
func (in *ModelService) DeepCopyObject() runtime.Object
```

## Why DeepCopy methods are required

The Kubernetes controller-runtime uses shared caches. Controllers should work with copies of cached objects instead of mutating shared cached objects directly.

The flow is:

```text
Cached ModelService
        |
        v
DeepCopy
        |
        v
Independent object copy
        |
        v
Controller safely modifies the copy
```

Run `make generate` after changing API structs, especially after adding:

- fields;
- nested structs;
- slices;
- maps;
- pointers;
- additional API types.

Do not manually edit:

```text
zz_generated.deepcopy.go
```

It will be regenerated.

---

# 17. Understanding `make manifests`

Run:

```bash
make manifests
```

This generates Kubernetes YAML from Go API definitions and Kubebuilder markers.

Main output:

```text
config/crd/bases/platform.anselem.dev_modelservices.yaml
```

This is the Custom Resource Definition, or CRD.

The CRD teaches Kubernetes about:

```text
ModelService
```

Without the CRD, Kubernetes only knows its built-in resources such as:

```text
Pod
Deployment
Service
ConfigMap
Secret
```

After the CRD is installed, Kubernetes understands commands such as:

```bash
kubectl get modelservices
kubectl describe modelservice <name>
```

`make manifests` also updates RBAC YAML from markers in the controller source.

Example marker:

```go
// +kubebuilder:rbac:groups=platform.anselem.dev,resources=modelservices,verbs=get;list;watch;create;update;patch;delete
```

Possible generated output:

```text
config/rbac/role.yaml
```

Run `make manifests` after changing:

- API fields;
- validation markers;
- printer columns;
- status subresource markers;
- RBAC markers;
- resource scope;
- API metadata.

---

# 18. Understanding `make build`

Run:

```bash
make build
```

This compiles the Go source code into the operator executable.

Inputs include:

```text
cmd/main.go
api/v1alpha1/*.go
internal/controller/*.go
go.mod
go.sum
```

Output:

```text
bin/manager
```

The build flow is:

```text
Go source code
        +
Generated DeepCopy code
        +
Controller-runtime dependencies
        |
        v
Go compiler
        |
        v
bin/manager
```

The `manager` executable is the operator process.

When started, it:

- connects to Kubernetes;
- registers the ModelService API;
- starts the ModelService controller;
- watches resources;
- runs reconciliation;
- exposes health and metrics endpoints.

A successful build proves that the code compiles. It does not prove that the controller logic is correct.

---

# 19. Understanding `make test`

Run:

```bash
make test
```

This runs Go tests for the API and controller.

Relevant files include:

```text
internal/controller/modelservice_controller_test.go
internal/controller/suite_test.go
```

Kubebuilder controller tests often use `envtest`.

`envtest` starts temporary Kubernetes control-plane components:

```text
etcd
kube-apiserver
```

It does not start a full worker-node cluster, but it provides a real Kubernetes API for controller testing.

The flow is:

```text
make test
    |
    v
setup-envtest obtains test assets
    |
    v
Temporary API server and etcd start
    |
    v
CRDs are installed
    |
    v
Controller tests run
    |
    v
Temporary environment stops
```

Tests can verify that:

- a ModelService can be created;
- the controller creates a Deployment;
- the requested image is used;
- replica changes are reconciled;
- status is updated;
- resources are deleted correctly;
- duplicate resources are not created.

A build answers:

```text
Can the code compile?
```

A test answers:

```text
Does the code behave correctly?
```

---

# 20. Understanding `go mod tidy`

Run:

```bash
go mod tidy
```

This synchronizes Go dependencies.

It:

- adds dependencies imported by the code;
- removes unused dependencies;
- updates `go.mod`;
- updates `go.sum`.

It does not generate CRDs or compile the operator.

Use it after adding or removing Go imports or dependencies.

---

# 21. How the Current Folder Structure Was Created

Current structure:

```text
.
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
├── img
├── internal
│   └── controller
├── kind-cluster.yaml
├── Makefile
├── PROJECT
├── README.md
└── test
```

The creation sources are summarized below.

| Path or file | Created mainly by | Purpose |
|---|---|---|
| `cmd/main.go` | `kubebuilder init` | Operator program entry point |
| `Dockerfile` | `kubebuilder init` | Builds the operator container image |
| `Makefile` | `kubebuilder init` | Provides build, test, install, and deploy commands |
| `PROJECT` | `kubebuilder init`, updated by `create api` | Stores Kubebuilder project metadata |
| `go.mod` | `kubebuilder init` | Defines the Go module and dependencies |
| `go.sum` | Go dependency commands | Stores dependency checksums |
| `hack/boilerplate.go.txt` | `kubebuilder init` | Header template for generated Go files |
| `config/manager` | `kubebuilder init` | Operator Deployment configuration |
| `config/default` | `kubebuilder init` | Main Kustomize deployment entry point |
| `config/network-policy` | `kubebuilder init` | Controls access to operator metrics |
| `config/prometheus` | `kubebuilder init` | Prometheus ServiceMonitor configuration |
| `config/rbac` | init, API scaffolding, manifests | Service account and permissions |
| `test/e2e` | `kubebuilder init` | End-to-end test scaffolding |
| `test/utils` | `kubebuilder init` | Shared test helpers |
| `api/v1alpha1` | `kubebuilder create api` | Defines the ModelService Kubernetes API |
| `internal/controller` | `kubebuilder create api` | Contains ModelService reconciliation logic and tests |
| `config/samples` | `kubebuilder create api` | Example ModelService YAML |
| `zz_generated.deepcopy.go` | `make generate` | Generated Kubernetes DeepCopy methods |
| CRD file in `config/crd/bases` | `make manifests` | Registers ModelService with Kubernetes |
| generated RBAC rules | `make manifests` | Grants the operator Kubernetes permissions |
| `bin/manager` | `make build` | Compiled operator executable |
| `bin/controller-gen-v0.21.0` | Makefile tool installation | Versioned controller-gen executable |
| `cover.out` | test coverage command | Stores Go test coverage data |
| `kind-cluster.yaml` | created manually | Defines the local kind cluster |
| `img/ai-platform-arc.png` | created manually | Architecture documentation image |
| `AGENTS.md` | created manually or by project tooling | Contributor or agent instructions |

---

# 22. Important Folders Explained

## `api/v1alpha1`

Contains the Go definitions for the Kubernetes API.

```text
api/v1alpha1/
├── groupversion_info.go
├── modelservice_types.go
└── zz_generated.deepcopy.go
```

### `groupversion_info.go`

Registers:

```text
platform.anselem.dev/v1alpha1
```

with the Kubernetes runtime scheme.

### `modelservice_types.go`

Defines:

```go
ModelServiceSpec
ModelServiceStatus
ModelService
ModelServiceList
```

This is one of the main files you will edit.

### `zz_generated.deepcopy.go`

Generated by `make generate`. Do not edit it manually.

---

## `internal/controller`

Contains controller logic:

```text
internal/controller/
├── modelservice_controller.go
├── modelservice_controller_test.go
└── suite_test.go
```

### `modelservice_controller.go`

Contains the `Reconcile` method.

This is where the operator behaviour is implemented.

Planned logic:

```text
Read ModelService
        |
        v
Create or update Deployment
        |
        v
Create or update Service
        |
        v
Observe readiness
        |
        v
Update ModelService status
```

### Test files

The test files verify controller behaviour using Go tests and envtest.

---

## `cmd`

Contains:

```text
cmd/main.go
```

This starts the controller-runtime manager and registers the API and controller.

It is the main entry point compiled into:

```text
bin/manager
```

---

## `config/crd`

Contains CRD generation configuration and the generated CRD.

```text
config/crd/
├── bases/
│   └── platform.anselem.dev_modelservices.yaml
├── kustomization.yaml
└── kustomizeconfig.yaml
```

The generated file under `bases` tells Kubernetes what a `ModelService` is.

---

## `config/rbac`

Contains permissions.

Important examples:

```text
service_account.yaml
role.yaml
role_binding.yaml
leader_election_role.yaml
metrics_reader_role.yaml
modelservice_admin_role.yaml
modelservice_editor_role.yaml
modelservice_viewer_role.yaml
```

RBAC answers:

```text
Who can perform which action on which Kubernetes resource?
```

The operator will later need permissions for resources it manages, such as Deployments and Services.

---

## `config/manager`

Contains the Kubernetes Deployment configuration for the operator manager.

```text
config/manager/manager.yaml
```

This is used when the operator is deployed inside Kubernetes.

---

## `config/default`

This is the main Kustomize entry point.

It combines:

```text
CRD
RBAC
ServiceAccount
Manager Deployment
Metrics Service
NetworkPolicy
```

Commands such as `make deploy` normally build from this directory.

---

## `config/samples`

Contains an example custom resource:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

This file is used to test the operator after installing the CRD and running the controller.

---

## `config/prometheus`

Contains optional Prometheus Operator integration.

It can define a `ServiceMonitor` that tells Prometheus how to scrape the operator metrics endpoint.

---

## `config/network-policy`

Contains network policies controlling which traffic may reach operator metrics endpoints.

---

## `bin`

Contains executable files.

```text
bin/
├── controller-gen-v0.21.0
└── manager
```

### `controller-gen-v0.21.0`

A development helper used to generate code and manifests.

### `manager`

The compiled operator executable.

These are not source files.

---

## `test/e2e`

Contains end-to-end tests.

These test the complete deployed behaviour rather than only isolated functions.

---

# 23. Source Files Versus Generated Files

## Files you normally edit

```text
api/v1alpha1/modelservice_types.go
internal/controller/modelservice_controller.go
internal/controller/modelservice_controller_test.go
config/samples/platform_v1alpha1_modelservice.yaml
README.md
```

## Files you normally do not edit manually

```text
api/v1alpha1/zz_generated.deepcopy.go
config/crd/bases/platform.anselem.dev_modelservices.yaml
bin/manager
cover.out
```

## Files you may customize carefully later

```text
config/manager/manager.yaml
config/default/kustomization.yaml
config/rbac/*.yaml
config/prometheus/*.yaml
config/network-policy/*.yaml
Dockerfile
Makefile
```

---

# 24. Recommended Development Command Order

After changing API types or controller code, use:

```bash
cd /mnt/data/ai-platform-operator

export LOCALBIN="$HOME/.local/kubebuilder/bin"
mkdir -p "$LOCALBIN"

make generate
make manifests
go mod tidy
make build
make test
```

The sequence means:

```text
make generate
    Generate supporting Go code

make manifests
    Generate CRD and RBAC YAML

go mod tidy
    Synchronize Go dependencies

make build
    Compile the operator executable

make test
    Verify expected behaviour
```

---

# 25. Verify the Generated Results

Check helper tools:

```bash
ls -lah "$LOCALBIN"
```

Check API code:

```bash
ls -lah api/v1alpha1
```

Expected important file:

```text
zz_generated.deepcopy.go
```

Check the generated CRD:

```bash
ls -lah config/crd/bases
```

Expected:

```text
platform.anselem.dev_modelservices.yaml
```

Check the compiled operator:

```bash
ls -lah bin
file bin/manager
```

Check tests:

```bash
make test
```

Check repository changes:

```bash
git status --short
```

---

# 26. What Happens Next

The scaffolding and environment are now ready.

The next development milestone is:

```text
1. Define ModelServiceSpec
2. Define ModelServiceStatus
3. Regenerate code
4. Regenerate CRD and RBAC
5. Implement Deployment reconciliation
6. Install the CRD into kind
7. Run the operator locally
8. Apply a sample ModelService
9. Verify that the operator creates a Deployment
10. Update replicas and test reconciliation
11. Delete the ModelService and verify cleanup
```

The first important source files to edit will be:

```text
api/v1alpha1/modelservice_types.go
internal/controller/modelservice_controller.go
config/samples/platform_v1alpha1_modelservice.yaml
```

---

# 27. Complete Mental Model

The full process is:

```text
You write ModelService Go types
        |
        v
make generate
        |
        +--> DeepCopy Go methods
        |
        v
make manifests
        |
        +--> CRD YAML
        +--> RBAC YAML
        |
        v
You write reconciliation logic
        |
        v
make build
        |
        +--> bin/manager
        |
        v
make test
        |
        +--> pass or fail
        |
        v
make install
        |
        +--> install CRD into kind
        |
        v
make run
        |
        +--> run operator locally
        |
        v
kubectl apply sample ModelService
        |
        v
Controller reconciles Kubernetes resources
```

The most important principle is:

```text
Go types define the desired API.
The CRD teaches Kubernetes about that API.
The controller implements the behaviour.
The manager executable runs the controller.
Tests verify that the behaviour is correct.
```

