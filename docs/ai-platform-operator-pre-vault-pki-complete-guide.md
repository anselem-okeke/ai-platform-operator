# AI Platform Operator — Complete Pre-Vault PKI Implementation Guide

## Gateway API, Envoy Gateway, MetalLB, TLS termination, HTTP-to-HTTPS redirect, and cert-manager development CA automation

**Project:** `ai-platform-operator`  
**Scope:** Everything implemented and validated before the Vault PKI integration  
**Validated cluster context:** `kind-ai-platform-policy`  
**Application:** `ModelService/fraud-model`  
**Hostname:** `fraud-model.local`  
**Gateway:** `gateway-system/shared-gateway`  
**Backend namespace:** `ai-platform`  
**Gateway data-plane namespace:** `envoy-gateway-system`

---

# 1. Purpose

This guide documents the complete platform implementation that existed immediately before Vault PKI was introduced.

It covers:

1. Creating the local Kubernetes cluster.
2. Installing Calico for real `NetworkPolicy` enforcement.
3. Installing Kubernetes Gateway API CRDs.
4. Installing Envoy Gateway.
5. Installing and configuring MetalLB.
6. Extending the Go/Kubebuilder operator to create `HTTPRoute`.
7. Connecting the route to a shared Gateway.
8. Updating the generated `NetworkPolicy` for Envoy traffic.
9. Creating HTTP and HTTPS Gateway listeners.
10. Creating an HTTP-to-HTTPS redirect.
11. Proving TLS first with a manually generated development certificate.
12. Installing cert-manager.
13. Creating a development root CA with cert-manager.
14. Automating the `fraud-model.local` certificate.
15. Validating certificate issuance, trust, reissuance, renewal, and private-key rotation.
16. Validating external routing, hostname isolation, redirects, and Gateway continuity.
17. Providing recovery and troubleshooting procedures.

This document deliberately stops before Vault PKI.

---

# 2. Final pre-Vault architecture

```text
Client
  │
  ├── HTTP :80
  │      │
  │      ▼
  │  HTTPRoute/fraud-model-http-redirect
  │      │
  │      └── 301 → https://fraud-model.local/
  │
  └── HTTPS :443
         │
         ▼
Gateway/shared-gateway
namespace: gateway-system
         │
         │ TLS termination
         │ Secret: fraud-model-local-tls
         ▼
Envoy Gateway data plane
namespace: envoy-gateway-system
         │
         │ permitted by ModelService NetworkPolicy
         ▼
HTTPRoute/fraud-model
namespace: ai-platform
         │
         ▼
Service/fraud-model:8080
         │
         ▼
Deployment/fraud-model Pods
```

Certificate automation before Vault:

```text
ClusterIssuer/development-selfsigned-bootstrap
        │
        ▼
Certificate/development-root-ca
        │
        ▼
Secret/development-root-ca
        │
        ▼
Issuer/development-ca
        │
        ▼
Certificate/fraud-model-local
        │
        ▼
Secret/fraud-model-local-tls
        │
        ▼
Gateway HTTPS listener
```

---

# 3. Responsibility boundaries

## 3.1 Shared platform infrastructure

The platform administrator owns:

- kind cluster infrastructure;
- Calico;
- Gateway API CRDs;
- Envoy Gateway;
- MetalLB;
- `GatewayClass`;
- shared `Gateway`;
- HTTP-to-HTTPS redirect route;
- cert-manager;
- root CA and issuer lifecycle;
- trust distribution;
- certificate monitoring and rotation procedures.

## 3.2 ModelService operator

The Go operator owns:

- `Deployment`;
- `Service`;
- `ServiceAccount`;
- `PersistentVolumeClaim`;
- `PodDisruptionBudget`;
- `NetworkPolicy`;
- application-specific `HTTPRoute`;
- route lifecycle and drift correction;
- status reporting.

## 3.3 Provider-neutral certificate contract

The Gateway only depends on a normal Kubernetes TLS Secret:

```yaml
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
```

The Secret name used throughout the implementation is:

```text
fraud-model-local-tls
```

This is why the certificate provider could later be changed from the development CA to Vault PKI without changing the Gateway, hostname, route, backend Service, or operator architecture.

---

# 4. Validated component versions and values

```text
Kubernetes:             v1.36.1
kind cluster:           kind-ai-platform-policy
Gateway API CRDs:       v1.5.1
Envoy Gateway:          v1.8.3
MetalLB:                v0.16.1
Calico:                 v3.32.1
cert-manager:           v1.21.0
Pod CIDR:               10.244.0.0/16
Service CIDR:           10.96.0.0/16
API server host port:   6444
MetalLB pool:           172.19.255.200-172.19.255.250
Gateway address:        172.19.255.200
Gateway hostname:       fraud-model.local
Backend port:           8080
```

Repository location:

```text
/mnt/data/ai-platform-operator
```

---

# 5. Prerequisites

The host should provide:

- Linux;
- Docker;
- kind;
- `kubectl`;
- Helm;
- Go;
- Make;
- OpenSSL;
- curl;
- Git;
- a cloned or initialized `ai-platform-operator` repository.

Verify:

```bash
docker version
kind version
kubectl version --client
helm version
go version
make --version
openssl version
curl --version
git --version
```

Enter the repository:

```bash
cd /mnt/data/ai-platform-operator
```

Create local working directories:

```bash
mkdir -p \
  .local/tls \
  .local/cluster-backup

chmod 700 .local/tls
```

Keep `.local/` outside Git:

```bash
grep -qxF '.local/' .gitignore || \
  printf '\n.local/\n' >> .gitignore
```

Never commit:

- CA private keys;
- server private keys;
- generated ServiceAccount tokens;
- decoded Kubernetes Secrets;
- local kubeconfig credentials.

---

# 6. Create the kind cluster with Calico networking

## 6.1 Why the Pod CIDR was changed

An earlier Pod network overlapped with the physical LAN range `192.168.0.0/16`.

That caused Pod traffic intended for LAN systems to be treated as Pod-network traffic.

The corrected design uses:

```text
Pod network:     10.244.0.0/16
Physical LAN:    192.168.0.0/16
```

These ranges do not overlap.

## 6.2 Cluster configuration

Create or verify:

```text
kind-calico-config.yaml
```

Example:

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

name: ai-platform-policy

networking:
  disableDefaultCNI: true
  podSubnet: 10.244.0.0/16
  serviceSubnet: 10.96.0.0/16
  apiServerAddress: "0.0.0.0"
  apiServerPort: 6444

nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30001
        hostPort: 30002
        protocol: TCP

  - role: worker
  - role: worker
```

Create the cluster:

```bash
kind create cluster \
  --config kind-calico-config.yaml
```

Verify the context:

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

Verify nodes:

```bash
kubectl get nodes -o wide
```

Expected structure:

```text
ai-platform-policy-control-plane   Ready   control-plane
ai-platform-policy-worker          Ready   <none>
ai-platform-policy-worker2         Ready   <none>
```

Before a CNI is installed, nodes or system Pods may not be fully ready. Continue with Calico.

---

# 7. Install Calico

Install the pinned Calico manifest:

```bash
kubectl apply \
  -f https://raw.githubusercontent.com/projectcalico/calico/v3.32.1/manifests/calico.yaml
```

Wait for Calico components:

```bash
kubectl rollout status daemonset/calico-node \
  -n kube-system \
  --timeout=300s

kubectl rollout status deployment/calico-kube-controllers \
  -n kube-system \
  --timeout=300s
```

Verify:

```bash
kubectl get pods -n kube-system -o wide
kubectl get nodes
```

All nodes should become:

```text
Ready
```

Confirm Pod addresses use the new CIDR:

```bash
kubectl get pods -A -o wide
```

Expected Pod IP prefix:

```text
10.244.x.x
```

---

# 8. Install Kubernetes Gateway API CRDs

Install Gateway API v1.5.1 standard CRDs:

```bash
kubectl apply \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml
```

Verify required CRDs:

```bash
kubectl get crd | grep gateway.networking.k8s.io
```

Important CRDs include:

```text
gatewayclasses.gateway.networking.k8s.io
gateways.gateway.networking.k8s.io
httproutes.gateway.networking.k8s.io
referencegrants.gateway.networking.k8s.io
```

Verify the API resources:

```bash
kubectl api-resources |
grep -E 'GatewayClass|Gateway|HTTPRoute'
```

---

# 9. Install Envoy Gateway

## 9.1 Pull the chart

```bash
helm pull \
  oci://docker.io/envoyproxy/gateway-helm \
  --version v1.8.3 \
  --destination .local
```

## 9.2 Render Envoy-specific CRDs

Render the chart CRDs:

```bash
helm show crds \
  oci://docker.io/envoyproxy/gateway-helm \
  --version v1.8.3 \
  > .local/envoy-gateway-crds-v1.8.3.yaml
```

Some Helm/OCI clients may write informational `Pulled:` or `Digest:` lines into redirected output. Remove those lines:

```bash
sed \
  '/^Pulled:/d; /^Digest:/d' \
  .local/envoy-gateway-crds-v1.8.3.yaml \
  > .local/envoy-gateway-crds-v1.8.3-clean.yaml
```

Apply the cleaned CRDs:

```bash
kubectl apply \
  -f .local/envoy-gateway-crds-v1.8.3-clean.yaml
```

## 9.3 Install Envoy Gateway

```bash
helm upgrade --install eg \
  oci://docker.io/envoyproxy/gateway-helm \
  --version v1.8.3 \
  --namespace envoy-gateway-system \
  --create-namespace \
  --set crds.enabled=false \
  --skip-crds \
  --wait \
  --timeout 5m
```

Verify:

```bash
helm list -n envoy-gateway-system
kubectl get pods -n envoy-gateway-system
kubectl get deployments -n envoy-gateway-system
```

Expected controller state:

```text
Running
Available
```

---

# 10. Install MetalLB

Install MetalLB v0.16.1:

```bash
kubectl apply \
  -f https://raw.githubusercontent.com/metallb/metallb/v0.16.1/config/manifests/metallb-native.yaml
```

Wait:

```bash
kubectl rollout status deployment/controller \
  -n metallb-system \
  --timeout=300s

kubectl rollout status daemonset/speaker \
  -n metallb-system \
  --timeout=300s
```

Create:

```text
config/platform/metallb-pool.yaml
```

```yaml
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: ai-platform-pool
  namespace: metallb-system
spec:
  addresses:
    - 172.19.255.200-172.19.255.250
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: ai-platform-l2
  namespace: metallb-system
spec:
  ipAddressPools:
    - ai-platform-pool
```

Apply:

```bash
kubectl apply \
  -f config/platform/metallb-pool.yaml
```

Verify:

```bash
kubectl get ipaddresspool,l2advertisement \
  -n metallb-system
```

---

# 11. Create platform namespaces

```bash
kubectl create namespace gateway-system \
  --dry-run=client \
  -o yaml |
kubectl apply -f -

kubectl create namespace ai-platform \
  --dry-run=client \
  -o yaml |
kubectl apply -f -
```

Allow application routes to attach to the shared Gateway:

```bash
kubectl label namespace ai-platform \
  shared-gateway-access=true \
  --overwrite
```

Verify:

```bash
kubectl get namespace ai-platform \
  --show-labels
```

Expected label:

```text
shared-gateway-access=true
```

---

# 12. Extend the Go operator with Gateway API support

This section describes the operator implementation itself.

## 12.1 Add the Gateway API Go dependency

```bash
cd /mnt/data/ai-platform-operator

go get sigs.k8s.io/gateway-api@v1.5.1
go mod tidy
```

Verify:

```bash
grep 'sigs.k8s.io/gateway-api' go.mod
```

Expected:

```text
sigs.k8s.io/gateway-api v1.5.1
```

## 12.2 Register Gateway API types in the manager scheme

File:

```text
cmd/main.go
```

Import:

```go
gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
```

Register:

```go
utilruntime.Must(clientgoscheme.AddToScheme(scheme))
utilruntime.Must(platformv1alpha1.AddToScheme(scheme))
utilruntime.Must(gatewayv1.AddToScheme(scheme))
```

This enables the controller-runtime client to manage `HTTPRoute` objects.

## 12.3 Register Gateway API types in envtest

File:

```text
internal/controller/suite_test.go
```

Import:

```go
gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
```

Register with the test scheme before the test client is initialized:

```go
err = platformv1alpha1.AddToScheme(scheme.Scheme)
Expect(err).NotTo(HaveOccurred())

err = gatewayv1.AddToScheme(scheme.Scheme)
Expect(err).NotTo(HaveOccurred())
```

Do not use `k8sClient.Scheme()` before `k8sClient` exists.

## 12.4 Add exposure fields to the API

File:

```text
api/v1alpha1/modelservice_types.go
```

```go
// ModelServiceExposure defines optional external HTTP exposure through
// Kubernetes Gateway API.
type ModelServiceExposure struct {
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Hostname string `json:"hostname,omitempty"`

	// +kubebuilder:default="/"
	// +kubebuilder:validation:Pattern=`^/.*`
	PathPrefix string `json:"pathPrefix,omitempty"`

	// +kubebuilder:default="shared-gateway"
	// +kubebuilder:validation:MinLength=1
	GatewayName string `json:"gatewayName,omitempty"`

	// +kubebuilder:default="gateway-system"
	// +kubebuilder:validation:MinLength=1
	GatewayNamespace string `json:"gatewayNamespace,omitempty"`

	// +kubebuilder:default="http"
	// +kubebuilder:validation:MinLength=1
	GatewaySectionName string `json:"gatewaySectionName,omitempty"`

	// +kubebuilder:default="envoy-gateway-system"
	// +kubebuilder:validation:MinLength=1
	GatewayDataPlaneNamespace string `json:"gatewayDataPlaneNamespace,omitempty"`
}
```

Add to `ModelServiceSpec`:

```go
// Exposure contains optional Gateway API HTTP exposure configuration.
// +optional
Exposure *ModelServiceExposure `json:"exposure,omitempty"`
```

Regenerate artifacts:

```bash
gofmt -w api/v1alpha1/modelservice_types.go

make generate
make manifests
make build
```

## 12.5 Add HTTPRoute RBAC

Controller markers:

```go
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get
```

Regenerate:

```bash
make manifests
```

Verify:

```bash
grep -nA 20 "httproutes" config/rbac/role.yaml
```

The controller reads route status but does not write it. Envoy Gateway owns route status.

## 12.6 Implement HTTPRoute reconciliation

The reconciler behavior is:

```text
exposure.enabled=true
    → create or update the owned HTTPRoute

exposure.enabled=false
    → delete the owned HTTPRoute

unowned route with same name
    → refuse destructive management
```

The generated route structure is:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  parentRefs:
    - name: shared-gateway
      namespace: gateway-system
      sectionName: http
  hostnames:
    - fraud-model.local
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - kind: Service
          name: fraud-model
          port: 8080
          weight: 1
```

Use an owner reference:

```go
controllerutil.SetControllerReference(
	modelService,
	route,
	r.Scheme,
)
```

This provides:

- ownership;
- garbage collection;
- safe drift correction;
- protection against deleting unrelated routes.

## 12.7 Watch owned HTTPRoutes

Extend `SetupWithManager`:

```go
return ctrl.NewControllerManagedBy(mgr).
	For(&platformv1alpha1.ModelService{}).
	Owns(&corev1.ServiceAccount{}).
	Owns(&appsv1.Deployment{}).
	Owns(&corev1.Service{}).
	Owns(&corev1.PersistentVolumeClaim{}).
	Owns(&policyv1.PodDisruptionBudget{}).
	Owns(&networkingv1.NetworkPolicy{}).
	Owns(&gatewayv1.HTTPRoute{}).
	Named("modelservice").
	Complete(r)
```

## 12.8 Update NetworkPolicy reconciliation

Envoy proxy Pods run in:

```text
envoy-gateway-system
```

The generated `NetworkPolicy` must allow that namespace when exposure is enabled.

Expected peer:

```yaml
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: envoy-gateway-system
```

Only the ModelService TCP port should be allowed:

```yaml
ports:
  - protocol: TCP
    port: 8080
```

Do not allow unrestricted ingress from every namespace.

---

# 13. Add Gateway API CRD support to envtest

Registering Go types is not enough; envtest also needs the actual CRD.

Create:

```bash
mkdir -p config/crd/gateway-api
```

Copy the standard `HTTPRoute` CRD from the Go module cache:

```bash
cp \
  /home/ansible/go/pkg/mod/sigs.k8s.io/gateway-api@v1.5.1/config/crd/standard/gateway.networking.k8s.io_httproutes.yaml \
  config/crd/gateway-api/
```

Configure envtest:

```go
testEnv = &envtest.Environment{
	CRDDirectoryPaths: []string{
		filepath.Join("..", "..", "config", "crd", "bases"),
		filepath.Join("..", "..", "config", "crd", "gateway-api"),
	},
	ErrorIfCRDPathMissing: true,
}
```

Difference:

```text
AddToScheme
    → teaches the Go client the object type

CRDDirectoryPaths
    → teaches the envtest API server how to store and validate it
```

---

# 14. Build and test the operator

Format:

```bash
gofmt -w \
  cmd/main.go \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go \
  internal/controller/suite_test.go
```

Run:

```bash
go mod tidy
make generate
make manifests
make build
make test
```

Validated result included:

```text
go build -o bin/manager cmd/main.go
ok github.com/anselem-okeke/ai-platform-operator/internal/controller
coverage: 79.2% of statements
```

Tests covered:

- specification propagation;
- HTTPRoute creation;
- HTTPRoute updates;
- HTTPRoute deletion when exposure is disabled;
- route ownership;
- NetworkPolicy Envoy namespace allowance;
- Service port propagation;
- Gateway listener propagation;
- ServiceAccount drift correction;
- managed-resource ownership.

---

# 15. Install and run the operator in the cluster

Confirm context:

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

Install or update the CRD:

```bash
cd /mnt/data/ai-platform-operator
make install
```

Run the controller locally:

```bash
make run
```

Keep this terminal running.

In another terminal, apply the sample:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Validated sample workload:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: nginxinc/nginx-unprivileged:1.31-alpine
  replicas: 2
  port: 8080

  networkPolicy:
    enabled: true
    allowSameNamespaceIngress: true
    allowDNSEgress: true

  exposure:
    enabled: true
    hostname: fraud-model.local
    pathPrefix: /
    gatewayName: shared-gateway
    gatewayNamespace: gateway-system
    gatewaySectionName: http
    gatewayDataPlaneNamespace: envoy-gateway-system
```

Verify:

```bash
kubectl get modelservice -n ai-platform
```

Expected structure:

```text
NAME          PHASE   READY   IMAGE                                     ENDPOINT
fraud-model   Ready   2       nginxinc/nginx-unprivileged:1.31-alpine   http://fraud-model.ai-platform.svc.cluster.local:8080
```

Verify managed resources:

```bash
kubectl get \
  deployment,service,serviceaccount,pvc,pdb,networkpolicy \
  -n ai-platform
```

---

# 16. Create the shared HTTP Gateway

Create:

```text
config/platform/shared-gateway.yaml
```

Initial HTTP-only form:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: envoy
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: shared-gateway
  namespace: gateway-system
spec:
  gatewayClassName: envoy

  listeners:
    - name: http
      protocol: HTTP
      port: 80
      hostname: fraud-model.local

      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"
```

Apply:

```bash
kubectl apply \
  -f config/platform/shared-gateway.yaml
```

Wait for programming:

```bash
kubectl wait \
  --for=condition=Programmed \
  gateway/shared-gateway \
  -n gateway-system \
  --timeout=180s
```

Verify:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide
```

Expected:

```text
NAME             CLASS   ADDRESS          PROGRAMMED
shared-gateway   envoy   172.19.255.200   True
```

---

# 17. Validate the operator-managed HTTPRoute

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

Expected hostname:

```text
["fraud-model.local"]
```

Check status:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Accepted=True reason=Accepted
ResolvedRefs=True reason=ResolvedRefs
```

Inspect:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

Confirm:

- Gateway name is `shared-gateway`;
- Gateway namespace is `gateway-system`;
- listener is `http`;
- hostname is `fraud-model.local`;
- backend is `Service/fraud-model`;
- backend port is `8080`;
- the route has a controller owner reference to the `ModelService`.

---

# 18. Validate external HTTP traffic

Get the Gateway IP:

```bash
GATEWAY_IP=$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)

echo "$GATEWAY_IP"
```

Expected:

```text
172.19.255.200
```

Correct hostname:

```bash
curl -v \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Expected backend content:

```text
Welcome to nginx!
```

Incorrect hostname:

```bash
curl -v \
  -H 'Host: wrong-host.local' \
  --connect-timeout 5 \
  http://"$GATEWAY_IP"/
```

Expected:

```text
HTTP/1.1 404 Not Found
```

This confirms hostname isolation.

---

# 19. Validate exposure lifecycle

Disable exposure:

```bash
kubectl patch modelservice fraud-model \
  -n ai-platform \
  --type merge \
  -p '{"spec":{"exposure":{"enabled":false}}}'
```

Verify route deletion:

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

Expected:

```text
NotFound
```

Re-enable:

```bash
kubectl patch modelservice fraud-model \
  -n ai-platform \
  --type merge \
  -p '{"spec":{"exposure":{"enabled":true}}}'
```

Verify recreation:

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

---

# 20. Manual TLS proof before cert-manager

The manual certificate step proved the Gateway TLS design independently of cert-manager.

## 20.1 Create a local CA

```bash
openssl genrsa \
  -out .local/tls/local-ca.key \
  4096

chmod 600 .local/tls/local-ca.key
```

```bash
openssl req \
  -x509 \
  -new \
  -sha256 \
  -days 3650 \
  -key .local/tls/local-ca.key \
  -out .local/tls/local-ca.crt \
  -subj "/C=DE/O=AI Platform Development/CN=AI Platform Local Development CA"
```

Inspect:

```bash
openssl x509 \
  -in .local/tls/local-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

## 20.2 Create the server certificate configuration

Create:

```text
.local/tls/fraud-model-openssl.cnf
```

```ini
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = distinguished_name
req_extensions = request_extensions

[distinguished_name]
C = DE
O = AI Platform Development
CN = fraud-model.local

[request_extensions]
subjectAltName = @subject_alt_names
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth

[subject_alt_names]
DNS.1 = fraud-model.local
```

## 20.3 Generate the server key and CSR

```bash
openssl genrsa \
  -out .local/tls/fraud-model.local.key \
  2048

chmod 600 .local/tls/fraud-model.local.key
```

```bash
openssl req \
  -new \
  -key .local/tls/fraud-model.local.key \
  -out .local/tls/fraud-model.local.csr \
  -config .local/tls/fraud-model-openssl.cnf
```

Inspect SAN:

```bash
openssl req \
  -in .local/tls/fraud-model.local.csr \
  -noout \
  -subject \
  -text |
grep -A2 "Subject Alternative Name"
```

Expected:

```text
DNS:fraud-model.local
```

## 20.4 Sign the certificate

```bash
openssl x509 \
  -req \
  -sha256 \
  -days 825 \
  -in .local/tls/fraud-model.local.csr \
  -CA .local/tls/local-ca.crt \
  -CAkey .local/tls/local-ca.key \
  -CAcreateserial \
  -out .local/tls/fraud-model.local.crt \
  -extensions request_extensions \
  -extfile .local/tls/fraud-model-openssl.cnf
```

Inspect:

```bash
openssl x509 \
  -in .local/tls/fraud-model.local.crt \
  -noout \
  -subject \
  -issuer \
  -dates \
  -ext subjectAltName
```

Verify chain:

```bash
openssl verify \
  -CAfile .local/tls/local-ca.crt \
  .local/tls/fraud-model.local.crt
```

Expected:

```text
.local/tls/fraud-model.local.crt: OK
```

## 20.5 Confirm key and certificate match

```bash
CERT_PUBLIC_KEY_HASH=$(
  openssl x509 \
    -in .local/tls/fraud-model.local.crt \
    -pubkey \
    -noout |
  openssl sha256
)

KEY_PUBLIC_KEY_HASH=$(
  openssl pkey \
    -in .local/tls/fraud-model.local.key \
    -pubout |
  openssl sha256
)

printf 'certificate: %s\nkey:         %s\n' \
  "$CERT_PUBLIC_KEY_HASH" \
  "$KEY_PUBLIC_KEY_HASH"
```

The hashes must match.

## 20.6 Create the manual TLS Secret

```bash
kubectl create secret tls fraud-model-local-tls \
  -n gateway-system \
  --cert=.local/tls/fraud-model.local.crt \
  --key=.local/tls/fraud-model.local.key \
  --dry-run=client \
  -o yaml |
kubectl apply -f -
```

Verify:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

Expected type:

```text
kubernetes.io/tls
```

Never print the private key.

---

# 21. Add HTTPS to the shared Gateway

Update:

```text
config/platform/shared-gateway.yaml
```

Final pre-Vault form:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: envoy
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: shared-gateway
  namespace: gateway-system
spec:
  gatewayClassName: envoy

  listeners:
    - name: http
      protocol: HTTP
      port: 80
      hostname: fraud-model.local
      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"

    - name: https
      protocol: HTTPS
      port: 443
      hostname: fraud-model.local
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: fraud-model-local-tls
      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"
```

Apply:

```bash
kubectl apply \
  -f config/platform/shared-gateway.yaml
```

Verify listener conditions:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.listeners[?(@.name=="https")].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Programmed=True reason=Programmed
Accepted=True reason=Accepted
ResolvedRefs=True reason=ResolvedRefs
```

---

# 22. Attach the ModelService route to HTTPS

Update the sample:

```yaml
spec:
  exposure:
    enabled: true
    hostname: fraud-model.local
    pathPrefix: /
    gatewayName: shared-gateway
    gatewayNamespace: gateway-system
    gatewaySectionName: https
    gatewayDataPlaneNamespace: envoy-gateway-system
```

Apply:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Verify:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='gateway={.spec.parentRefs[0].name}{"\n"}namespace={.spec.parentRefs[0].namespace}{"\n"}listener={.spec.parentRefs[0].sectionName}{"\n"}hostname={.spec.hostnames[0]}{"\n"}'
```

Expected:

```text
gateway=shared-gateway
namespace=gateway-system
listener=https
hostname=fraud-model.local
```

---

# 23. Create HTTP-to-HTTPS redirect

Create:

```text
config/platform/fraud-model-http-redirect.yaml
```

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: fraud-model-http-redirect
  namespace: ai-platform
spec:
  parentRefs:
    - name: shared-gateway
      namespace: gateway-system
      sectionName: http

  hostnames:
    - fraud-model.local

  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            statusCode: 301
```

Apply:

```bash
kubectl apply \
  -f config/platform/fraud-model-http-redirect.yaml
```

Verify:

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform \
  -o jsonpath='listener={.spec.parentRefs[0].sectionName}{"\n"}hostname={.spec.hostnames[0]}{"\n"}redirectScheme={.spec.rules[0].filters[0].requestRedirect.scheme}{"\n"}statusCode={.spec.rules[0].filters[0].requestRedirect.statusCode}{"\n"}'
```

Expected:

```text
listener=http
hostname=fraud-model.local
redirectScheme=https
statusCode=301
```

---

# 24. Validate manual TLS

Set the address:

```bash
GATEWAY_IP=$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)
```

Trusted HTTPS:

```bash
curl \
  --cacert .local/tls/local-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Expected:

```text
Welcome to nginx!
```

Direct TLS verification:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -CAfile .local/tls/local-ca.crt \
  -verify_return_error \
  </dev/null 2>/dev/null |
grep "Verify return code"
```

Expected:

```text
Verify return code: 0 (ok)
```

Redirect:

```bash
curl -I \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Expected:

```text
HTTP/1.1 301 Moved Permanently
location: https://fraud-model.local/
```

Follow redirect:

```bash
curl -L \
  --cacert .local/tls/local-ca.crt \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Wrong SNI hostname:

```bash
curl \
  --cacert .local/tls/local-ca.crt \
  --resolve wrong-host.local:443:"$GATEWAY_IP" \
  https://wrong-host.local/
```

The request should fail or be rejected.

Untrusted CA:

```bash
curl \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Expected:

```text
SSL certificate problem: unable to get local issuer certificate
```

Do not use `curl -k` as proof of correct certificate trust.

---

# 25. Install cert-manager

Install the validated pinned version:

```bash
helm upgrade --install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --version v1.21.0 \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait \
  --timeout 5m
```

Wait for all components:

```bash
kubectl rollout status deployment/cert-manager \
  -n cert-manager \
  --timeout=300s

kubectl rollout status deployment/cert-manager-webhook \
  -n cert-manager \
  --timeout=300s

kubectl rollout status deployment/cert-manager-cainjector \
  -n cert-manager \
  --timeout=300s
```

Verify:

```bash
kubectl get pods -n cert-manager
kubectl get crd | grep cert-manager.io
```

Important CRDs:

```text
certificates.cert-manager.io
certificaterequests.cert-manager.io
issuers.cert-manager.io
clusterissuers.cert-manager.io
```

Optional webhook validation:

```bash
kubectl apply --dry-run=server -f - <<'EOF'
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: webhook-test
  namespace: gateway-system
spec:
  selfSigned: {}
EOF
```

---

# 26. Bootstrap the cert-manager development CA

Create:

```text
config/platform/cert-manager-development-ca.yaml
```

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: development-selfsigned-bootstrap
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: development-root-ca
  namespace: gateway-system
spec:
  isCA: true
  commonName: AI Platform Development Root CA

  subject:
    organizations:
      - AI Platform Development

  secretName: development-root-ca

  duration: 87600h
  renewBefore: 8760h

  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always

  issuerRef:
    name: development-selfsigned-bootstrap
    kind: ClusterIssuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: development-ca
  namespace: gateway-system
spec:
  ca:
    secretName: development-root-ca
```

Apply:

```bash
kubectl apply \
  -f config/platform/cert-manager-development-ca.yaml
```

Verify:

```bash
kubectl get clusterissuer development-selfsigned-bootstrap
kubectl get certificate development-root-ca \
  -n gateway-system
kubectl get issuer development-ca \
  -n gateway-system
```

Expected:

```text
development-selfsigned-bootstrap   Ready=True
development-root-ca                Ready=True
development-ca                     Ready=True
```

A temporary warning that `development-root-ca` does not exist can appear while resources reconcile. It is resolved when the Certificate creates the Secret and the Issuer reaches `Ready=True`.

---

# 27. Verify and export the public development CA

Inspect the Secret safely:

```bash
kubectl get secret development-root-ca \
  -n gateway-system \
  -o go-template='type={{.type}}{{"\n"}}keys={{range $key, $value := .data}}{{$key}} {{end}}{{"\n"}}'
```

Expected:

```text
type=kubernetes.io/tls
keys=ca.crt tls.crt tls.key
```

Do not print `tls.key`.

Export only the public CA:

```bash
kubectl get secret development-root-ca \
  -n gateway-system \
  -o jsonpath='{.data.ca\.crt}' |
base64 --decode \
  > .local/tls/cert-manager-development-ca.crt

chmod 644 .local/tls/cert-manager-development-ca.crt
```

Inspect:

```bash
openssl x509 \
  -in .local/tls/cert-manager-development-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

Expected identity:

```text
O = AI Platform Development
CN = AI Platform Development Root CA
```

---

# 28. Create the cert-manager server Certificate

Create:

```text
config/platform/fraud-model-certificate.yaml
```

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: fraud-model-local
  namespace: gateway-system
spec:
  secretName: fraud-model-local-tls

  commonName: fraud-model.local

  dnsNames:
    - fraud-model.local

  duration: 720h
  renewBefore: 168h

  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always

  usages:
    - digital signature
    - key encipherment
    - server auth

  issuerRef:
    name: development-ca
    kind: Issuer
    group: cert-manager.io
```

This creates:

- a 30-day leaf certificate;
- renewal seven days before expiry;
- a new private key for every issuance.

---

# 29. Migrate from the manual Secret to cert-manager

Delete the manually created Secret:

```bash
kubectl delete secret fraud-model-local-tls \
  -n gateway-system
```

Immediately apply the Certificate:

```bash
kubectl apply \
  -f config/platform/fraud-model-certificate.yaml
```

Watch:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -w
```

Expected:

```text
fraud-model-local   True   fraud-model-local-tls
```

Verify the generated Secret:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

Expected:

```text
TYPE                DATA
kubernetes.io/tls   3
```

The third key is normally the CA certificate.

Because the Secret name stayed unchanged, the Gateway did not need to change.

---

# 30. Validate cert-manager ownership

```bash
kubectl describe certificate fraud-model-local \
  -n gateway-system
```

Expected:

```text
Ready=True
Reason=Ready
Message=Certificate is up to date and has not expired
```

Inspect safe annotations:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o yaml |
grep -E \
  'cert-manager.io|controller.cert-manager.io|name: fraud-model-local-tls'
```

Expected metadata:

```text
cert-manager.io/certificate-name: fraud-model-local
cert-manager.io/common-name: fraud-model.local
cert-manager.io/issuer-kind: Issuer
cert-manager.io/issuer-name: development-ca
```

Confirm the relation:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='certificate={.metadata.name}{"\n"}secret={.spec.secretName}{"\n"}ready={.status.conditions[?(@.type=="Ready")].status}{"\n"}'
```

Expected:

```text
certificate=fraud-model-local
secret=fraud-model-local-tls
ready=True
```

---

# 31. Inspect and verify the issued certificate

Export:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.tls\.crt}' |
base64 --decode \
  > .local/tls/cert-manager-fraud-model.crt
```

Inspect:

```bash
openssl x509 \
  -in .local/tls/cert-manager-fraud-model.crt \
  -noout \
  -subject \
  -issuer \
  -serial \
  -dates \
  -ext subjectAltName
```

Expected:

```text
subject=CN = fraud-model.local
issuer=O = AI Platform Development, CN = AI Platform Development Root CA
DNS:fraud-model.local
```

Verify:

```bash
openssl verify \
  -CAfile .local/tls/cert-manager-development-ca.crt \
  .local/tls/cert-manager-fraud-model.crt
```

Expected:

```text
.local/tls/cert-manager-fraud-model.crt: OK
```

---

# 32. End-to-end HTTPS and redirect validation

Get the Gateway IP:

```bash
GATEWAY_IP=$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)
```

Trusted HTTPS:

```bash
curl \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Expected:

```text
Welcome to nginx!
```

TLS chain:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -CAfile .local/tls/cert-manager-development-ca.crt \
  -verify_return_error \
  </dev/null 2>/dev/null |
grep "Verify return code"
```

Expected:

```text
Verify return code: 0 (ok)
```

HTTP redirect:

```bash
curl -I \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Expected:

```text
HTTP/1.1 301 Moved Permanently
location: https://fraud-model.local/
```

Follow redirect:

```bash
curl -L \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Expected:

```text
Welcome to nginx!
```

---

# 33. Validate reissuance, renewal, and private-key rotation

## 33.1 Inspect lifecycle data

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='notBefore={.status.notBefore}{"\n"}notAfter={.status.notAfter}{"\n"}renewalTime={.status.renewalTime}{"\n"}revision={.status.revision}{"\n"}'
```

## 33.2 Record the current serial

```bash
OLD_SERIAL=$(
  kubectl get secret fraud-model-local-tls \
    -n gateway-system \
    -o jsonpath='{.data.tls\.crt}' |
  base64 --decode |
  openssl x509 -noout -serial |
  cut -d= -f2
)

echo "old serial: $OLD_SERIAL"
```

## 33.3 Record the current public-key fingerprint

```bash
OLD_KEY_FP=$(
  kubectl get secret fraud-model-local-tls \
    -n gateway-system \
    -o jsonpath='{.data.tls\.key}' |
  base64 --decode |
  openssl pkey -pubout 2>/dev/null |
  openssl sha256 |
  awk '{print $2}'
)

echo "old key fingerprint: $OLD_KEY_FP"
```

## 33.4 Trigger renewal

Preferred:

```bash
cmctl renew fraud-model-local \
  -n gateway-system
```

Alternative development-only reissuance test:

```bash
kubectl delete secret fraud-model-local-tls \
  -n gateway-system
```

Watch:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -w
```

Expected transition:

```text
READY=False
READY=True
```

## 33.5 Read the new serial and key fingerprint

```bash
NEW_SERIAL=$(
  kubectl get secret fraud-model-local-tls \
    -n gateway-system \
    -o jsonpath='{.data.tls\.crt}' |
  base64 --decode |
  openssl x509 -noout -serial |
  cut -d= -f2
)

NEW_KEY_FP=$(
  kubectl get secret fraud-model-local-tls \
    -n gateway-system \
    -o jsonpath='{.data.tls\.key}' |
  base64 --decode |
  openssl pkey -pubout 2>/dev/null |
  openssl sha256 |
  awk '{print $2}'
)

printf 'old serial: %s\nnew serial: %s\n' \
  "$OLD_SERIAL" \
  "$NEW_SERIAL"

printf 'old key: %s\nnew key: %s\n' \
  "$OLD_KEY_FP" \
  "$NEW_KEY_FP"
```

Both should change because:

```yaml
rotationPolicy: Always
```

Retest HTTPS afterward.

---

# 34. Final pre-Vault validation

```bash
printf '\nGateway:\n'
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide

printf '\nGateway listeners:\n'
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.listeners[*]}listener={.name}{"\n"}{range .conditions[*]}  {.type}={.status} reason={.reason}{"\n"}{end}{end}'

printf '\nCertificate resources:\n'
kubectl get \
  clusterissuer,issuer,certificate,certificaterequest \
  -A

printf '\nTLS Secret:\n'
kubectl get secret fraud-model-local-tls \
  -n gateway-system

printf '\nRoutes:\n'
kubectl get httproute \
  -n ai-platform

printf '\nEnvoy Service:\n'
kubectl get service \
  -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=shared-gateway

printf '\nModelService:\n'
kubectl get modelservice fraud-model \
  -n ai-platform
```

Expected state:

```text
Gateway/shared-gateway:
  Programmed=True
  address=172.19.255.200

Issuer/development-ca:
  Ready=True

Certificate/fraud-model-local:
  Ready=True

Secret/fraud-model-local-tls:
  type=kubernetes.io/tls

HTTPRoute/fraud-model:
  Accepted=True
  ResolvedRefs=True
  listener=https

HTTPRoute/fraud-model-http-redirect:
  listener=http
  redirect=https
  status=301

ModelService/fraud-model:
  Ready
  replicas=2
```

---

# 35. Recovery and reinstall order

After cluster recreation, restore in this order:

```text
1. kind cluster
2. Calico
3. Gateway API standard CRDs
4. Envoy Gateway-specific CRDs
5. Envoy Gateway controller
6. MetalLB
7. MetalLB address pool and L2 advertisement
8. namespaces and route-attachment labels
9. shared Gateway
10. cert-manager
11. development root CA and Issuer
12. fraud-model Certificate
13. operator CRD
14. operator controller
15. ModelService sample
16. HTTP-to-HTTPS redirect
17. end-to-end validation
```

Commands:

```bash
kind create cluster \
  --config kind-calico-config.yaml

kubectl apply \
  -f https://raw.githubusercontent.com/projectcalico/calico/v3.32.1/manifests/calico.yaml

kubectl apply \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml

kubectl apply \
  -f .local/envoy-gateway-crds-v1.8.3-clean.yaml

helm upgrade --install eg \
  oci://docker.io/envoyproxy/gateway-helm \
  --version v1.8.3 \
  -n envoy-gateway-system \
  --create-namespace \
  --set crds.enabled=false \
  --skip-crds \
  --wait \
  --timeout 5m

kubectl apply \
  -f https://raw.githubusercontent.com/metallb/metallb/v0.16.1/config/manifests/metallb-native.yaml

kubectl apply \
  -f config/platform/metallb-pool.yaml

kubectl apply \
  -f config/platform/shared-gateway.yaml

helm upgrade --install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --version v1.21.0 \
  -n cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait \
  --timeout 5m

kubectl apply \
  -f config/platform/cert-manager-development-ca.yaml

kubectl apply \
  -f config/platform/fraud-model-certificate.yaml

make install
```

Run the controller:

```bash
make run
```

In another terminal:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml

kubectl apply \
  -f config/platform/fraud-model-http-redirect.yaml
```

---

# 36. Troubleshooting

## 36.1 Nodes remain NotReady

Check Calico:

```bash
kubectl get pods -n kube-system
kubectl describe pod -n kube-system -l k8s-app=calico-node
```

Confirm the cluster was created with:

```yaml
disableDefaultCNI: true
podSubnet: 10.244.0.0/16
```

## 36.2 `no matches for kind "HTTPRoute"`

Gateway API CRDs are missing:

```bash
kubectl get crd httproutes.gateway.networking.k8s.io
```

Reapply:

```bash
kubectl apply \
  -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml
```

For envtest, confirm:

```text
config/crd/gateway-api/gateway.networking.k8s.io_httproutes.yaml
```

and confirm the extra `CRDDirectoryPaths` entry.

## 36.3 Gateway is not Programmed

Check:

```bash
kubectl describe gateway shared-gateway \
  -n gateway-system

kubectl get pods -n envoy-gateway-system
kubectl logs -n envoy-gateway-system deployment/envoy-gateway
```

Also verify MetalLB:

```bash
kubectl get pods -n metallb-system
kubectl get ipaddresspool,l2advertisement \
  -n metallb-system
```

## 36.4 HTTPRoute is not Accepted

Inspect:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

Confirm:

- Gateway name;
- Gateway namespace;
- listener section;
- route namespace label;
- allowed route policy;
- hostname.

## 36.5 `ResolvedRefs=False`

Confirm:

```bash
kubectl get service fraud-model \
  -n ai-platform

kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

The backend Service must exist in the same namespace and expose the referenced port.

## 36.6 Route exists but traffic is blocked

Inspect:

```bash
kubectl get networkpolicy fraud-model \
  -n ai-platform \
  -o yaml
```

Confirm:

```yaml
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: envoy-gateway-system
```

and port `8080`.

## 36.7 HTTPS listener has `ResolvedRefs=False`

Check:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

The Secret must exist in the same namespace as the Gateway and have type:

```text
kubernetes.io/tls
```

## 36.8 cert-manager kinds are unknown

```bash
kubectl get crd | grep cert-manager.io
```

Reinstall cert-manager CRDs if necessary.

## 36.9 Development Issuer temporarily reports missing Secret

If the root Certificate is still being issued, the `development-ca` Issuer may briefly report:

```text
secrets "development-root-ca" not found
```

Wait:

```bash
kubectl wait \
  --for=condition=Ready \
  certificate/development-root-ca \
  -n gateway-system \
  --timeout=180s

kubectl wait \
  --for=condition=Ready \
  issuer/development-ca \
  -n gateway-system \
  --timeout=180s
```

## 36.10 Certificate is Ready but curl rejects it

Use the correct CA and hostname:

```bash
curl \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Do not request the Gateway by IP while expecting hostname validation.

## 36.11 Informational Helm OCI lines corrupt rendered YAML

Clean:

```bash
sed \
  '/^Pulled:/d; /^Digest:/d' \
  input.yaml \
  > clean.yaml
```

Apply the clean file only.

---

# 37. Security properties achieved

Before Vault PKI, the implementation already provided:

- encrypted client-to-Gateway traffic;
- hostname validation;
- certificate-chain validation;
- HTTP-to-HTTPS redirection;
- TLS termination at Envoy Gateway;
- internal-only backend Service;
- namespace-scoped Gateway route attachment;
- explicit Gateway data-plane NetworkPolicy allowance;
- route ownership and drift correction;
- least-required HTTPRoute RBAC;
- no private keys committed to Git;
- automated certificate reissuance;
- automated certificate renewal scheduling;
- automatic private-key rotation;
- provider-neutral certificate consumption by the Gateway.

---

# 38. Files created or updated

Core operator implementation:

```text
go.mod
go.sum
cmd/main.go
api/v1alpha1/modelservice_types.go
api/v1alpha1/zz_generated.deepcopy.go
internal/controller/modelservice_controller.go
internal/controller/modelservice_controller_test.go
internal/controller/suite_test.go
config/crd/bases/platform.anselem.dev_modelservices.yaml
config/crd/gateway-api/gateway.networking.k8s.io_httproutes.yaml
config/rbac/role.yaml
config/samples/platform_v1alpha1_modelservice.yaml
```

Platform resources:

```text
kind-calico-config.yaml
config/platform/metallb-pool.yaml
config/platform/shared-gateway.yaml
config/platform/fraud-model-http-redirect.yaml
config/platform/cert-manager-development-ca.yaml
config/platform/fraud-model-certificate.yaml
```

Local, non-committed material:

```text
.local/tls/
.local/cluster-backup/
.local/envoy-gateway-crds-v1.8.3.yaml
.local/envoy-gateway-crds-v1.8.3-clean.yaml
```

---

# 39. Completion checklist

```text
[✓] kind cluster created
[✓] Non-overlapping Pod CIDR configured
[✓] Calico installed
[✓] Nodes Ready
[✓] NetworkPolicy enforcement available
[✓] Gateway API CRDs installed
[✓] Envoy Gateway installed
[✓] MetalLB installed
[✓] Address pool configured
[✓] Shared Gateway assigned 172.19.255.200
[✓] Operator Gateway API dependency added
[✓] Gateway API scheme registered
[✓] Exposure API added to ModelService
[✓] HTTPRoute RBAC generated
[✓] HTTPRoute reconciliation implemented
[✓] HTTPRoute ownership implemented
[✓] HTTPRoute watch configured
[✓] NetworkPolicy allows Envoy namespace
[✓] Envtest includes HTTPRoute CRD
[✓] Controller tests pass
[✓] Controller coverage reached 79.2%
[✓] ModelService becomes Ready
[✓] HTTPRoute Accepted=True
[✓] HTTPRoute ResolvedRefs=True
[✓] Correct hostname reaches backend
[✓] Incorrect hostname rejected
[✓] Exposure disable deletes route
[✓] Exposure re-enable recreates route
[✓] Manual development CA created
[✓] Manual server certificate SAN verified
[✓] Manual TLS Secret created
[✓] Shared Gateway HTTP listener configured
[✓] Shared Gateway HTTPS listener configured
[✓] HTTPS listener references TLS Secret
[✓] HTTP-to-HTTPS redirect created
[✓] HTTP returns 301
[✓] Trusted HTTPS succeeds
[✓] OpenSSL verify code is 0
[✓] Untrusted CA is rejected
[✓] cert-manager installed
[✓] Development root CA automated
[✓] development-ca Issuer Ready
[✓] fraud-model Certificate Ready
[✓] TLS Secret generated automatically
[✓] Certificate verifies against development root
[✓] Renewal/reissuance validated
[✓] Certificate serial changes
[✓] Private key rotation configured
[✓] Gateway remains Programmed during certificate changes
```

---

# 40. Boundary before Vault PKI

At the end of this guide, the certificate source is:

```text
Certificate/fraud-model-local
        │
        ▼
Issuer/development-ca
        │
        ▼
Secret/development-root-ca
```

The next phase replaces only the issuer backend:

```text
Before:
Certificate → development-ca → development root CA

After:
Certificate → vault-issuer → Vault PKI
```

The following remain unchanged:

- `ModelService`;
- operator controller;
- `HTTPRoute`;
- hostname;
- shared Gateway;
- HTTPS listener;
- TLS Secret name;
- backend Service;
- HTTP redirect.

That separation is the key architectural reason the later Vault PKI integration was possible without coupling Vault directly to the operator.
