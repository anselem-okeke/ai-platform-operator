# External Exposure for `ModelService` with Kubernetes Gateway API

![img](/img/httproute-modelservice2.png)

## Implementation and Validation Documentation

**Project:** `ai-platform-operator`  
**Milestone:** Optional external exposure with Gateway API  
**Primary resource:** `ModelService`  
**Generated route:** `gateway.networking.k8s.io/v1` `HTTPRoute`  
**Validated controller coverage:** **79.2%**

---

## 1. Objective

The goal of this milestone was to extend the `ModelService` operator so that a model-serving workload can optionally be exposed through Kubernetes Gateway API.

The required behavior is:

```text
spec.exposure.enabled: true
        ↓
Create or update an HTTPRoute
        ↓
Attach the HTTPRoute to the shared Gateway
        ↓
Forward matching traffic to the generated Service
        ↓
Allow Envoy Gateway data-plane traffic through NetworkPolicy
```

When exposure is disabled:

```text
spec.exposure.enabled: false
        ↓
Delete the owned HTTPRoute
        ↓
Keep the remaining ModelService resources running
```

The resulting managed-resource hierarchy is:

```text
ModelService
├── Deployment
├── Service
├── ServiceAccount
├── PersistentVolumeClaim
├── PodDisruptionBudget
├── NetworkPolicy
└── HTTPRoute
```

---

## 2. Environment Architecture

Two Gateway-related namespaces exist in the cluster:

| Component | Namespace |
|---|---|
| Shared `Gateway` object | `gateway-system` |
| Envoy proxy Pods | `envoy-gateway-system` |
| `ModelService` and generated `HTTPRoute` | `ai-platform` |

These namespaces serve different purposes:

- The `HTTPRoute` attaches to `Gateway/shared-gateway` in `gateway-system`.
- Requests are actually forwarded by Envoy proxy Pods running in `envoy-gateway-system`.
- The generated `NetworkPolicy` must therefore allow ingress from `envoy-gateway-system`.
- The route remains in the same namespace as the backend `Service`, which is `ai-platform`.

The validated request path is:

```text
Client
  │
  │ Host: fraud-model.local
  ▼
Gateway/shared-gateway
namespace: gateway-system
  │
  ▼
Envoy proxy Pods
namespace: envoy-gateway-system
  │
  │ allowed by NetworkPolicy
  ▼
HTTPRoute/fraud-model
namespace: ai-platform
  │
  ▼
Service/fraud-model:8080
  │
  ▼
Deployment Pods
```

---

## 3. Files Updated

The milestone changed or generated the following files:

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

---

## 4. Gateway API Dependency

Gateway API v1 types were added to the Go project.

```bash
cd /mnt/data/ai-platform-operator

go get sigs.k8s.io/gateway-api@v1.5.1
go mod tidy
```

Verification:

```bash
grep 'sigs.k8s.io/gateway-api' go.mod
```

Expected dependency:

```text
sigs.k8s.io/gateway-api v1.5.1
```

---

## 5. Gateway API Scheme Registration

### 5.1 Manager scheme

File:

```text
cmd/main.go
```

Import:

```go
gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
```

Registration:

```go
utilruntime.Must(clientgoscheme.AddToScheme(scheme))
utilruntime.Must(platformv1alpha1.AddToScheme(scheme))
utilruntime.Must(gatewayv1.Install(scheme))
```

This allows the running controller manager to create, read, update, watch, and delete Gateway API objects through the controller-runtime client.

> `AddToScheme` may appear struck through in some IDEs because a newer alias such as `Install` is preferred. It remains functional. The project built and tested successfully with `AddToScheme`.

### 5.2 Test scheme

File:

```text
internal/controller/suite_test.go
```

Import:

```go
gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
```

Registration:

```go
err = platformv1alpha1.AddToScheme(scheme.Scheme)
Expect(err).NotTo(HaveOccurred())

err = gatewayv1.Install(scheme.Scheme)
Expect(err).NotTo(HaveOccurred())
```

The important detail is that the test uses:

```go
scheme.Scheme
```

and not:

```go
k8sClient.Scheme()
```

at this point in `BeforeSuite`, because `k8sClient` has not yet been initialized.

---

## 6. Exposure API Added to `ModelService`

File:

```text
api/v1alpha1/modelservice_types.go
```

The following API structure was added:

```go
// ModelServiceExposure defines optional external HTTP exposure through
// Kubernetes Gateway API.
type ModelServiceExposure struct {
	// Enabled determines whether the operator creates an HTTPRoute.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Hostname is the DNS hostname matched by the HTTPRoute.
	// Example: fraud-model.local
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Hostname string `json:"hostname,omitempty"`

	// PathPrefix is the HTTP path prefix forwarded to the ModelService.
	// +kubebuilder:default="/"
	// +kubebuilder:validation:Pattern=`^/.*`
	PathPrefix string `json:"pathPrefix,omitempty"`

	// GatewayName is the name of the shared Gateway.
	// +kubebuilder:default="shared-gateway"
	// +kubebuilder:validation:MinLength=1
	GatewayName string `json:"gatewayName,omitempty"`

	// GatewayNamespace is the namespace containing the shared Gateway.
	// +kubebuilder:default="gateway-system"
	// +kubebuilder:validation:MinLength=1
	GatewayNamespace string `json:"gatewayNamespace,omitempty"`

	// GatewaySectionName identifies the Gateway listener to which the
	// HTTPRoute should attach.
	// +kubebuilder:default="http"
	// +kubebuilder:validation:MinLength=1
	GatewaySectionName string `json:"gatewaySectionName,omitempty"`

	// GatewayDataPlaneNamespace is the namespace containing the Gateway
	// proxy Pods. The operator allows ingress from this namespace through
	// the generated NetworkPolicy.
	// +kubebuilder:default="envoy-gateway-system"
	// +kubebuilder:validation:MinLength=1
	GatewayDataPlaneNamespace string `json:"gatewayDataPlaneNamespace,omitempty"`
}
```

The following field was added to `ModelServiceSpec`:

```go
// Exposure contains optional Gateway API HTTP exposure configuration.
// When disabled or omitted, no HTTPRoute is created.
// +optional
Exposure *ModelServiceExposure `json:"exposure,omitempty"`
```

### Generated CRD schema

Artifacts were regenerated with:

```bash
gofmt -w api/v1alpha1/modelservice_types.go
make generate
make manifests
make build
```

The generated CRD includes defaults and validation for:

- `enabled`
- `hostname`
- `pathPrefix`
- `gatewayName`
- `gatewayNamespace`
- `gatewaySectionName`
- `gatewayDataPlaneNamespace`

Example generated schema behavior:

```yaml
exposure:
  properties:
    enabled:
      default: false
      type: boolean
    gatewayDataPlaneNamespace:
      default: envoy-gateway-system
      minLength: 1
      type: string
    gatewayName:
      default: shared-gateway
      minLength: 1
      type: string
    gatewayNamespace:
      default: gateway-system
      minLength: 1
      type: string
    gatewaySectionName:
      default: http
      minLength: 1
      type: string
    hostname:
      minLength: 1
      maxLength: 253
      type: string
    pathPrefix:
      default: /
      pattern: ^/.*
      type: string
```

---

## 7. Resolved Exposure Configuration

A resolver was added to normalize omitted fields and apply controller defaults.

```go
type resolvedExposureConfiguration struct {
	Enabled                   bool
	Hostname                  string
	PathPrefix                string
	GatewayName               string
	GatewayNamespace          string
	GatewaySectionName        string
	GatewayDataPlaneNamespace string
}
```

Default behavior:

```go
configuration := resolvedExposureConfiguration{
	Enabled:                   false,
	PathPrefix:                "/",
	GatewayName:               "shared-gateway",
	GatewayNamespace:          "gateway-system",
	GatewaySectionName:        "http",
	GatewayDataPlaneNamespace: "envoy-gateway-system",
}
```

The resolver:

- returns disabled defaults when `spec.exposure` is omitted;
- preserves explicit user values;
- fills omitted optional values with safe defaults;
- keeps the Gateway object namespace separate from the Envoy data-plane namespace.

---

## 8. HTTPRoute RBAC

The controller received explicit permissions for owned HTTPRoutes.

Kubebuilder markers:

```go
//
// Permissions for owned HTTPRoutes.
//
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get
```

Generated RBAC verification:

```bash
grep -nA 15 "httproutes" config/rbac/role.yaml
```

Validated permissions:

```yaml
- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - httproutes
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch

- apiGroups:
  - gateway.networking.k8s.io
  resources:
  - httproutes/status
  verbs:
  - get
```

The operator reads route status but does not write it. Envoy Gateway writes the `HTTPRoute` status conditions.

---

## 9. HTTPRoute Reconciliation

The main reconciler now calls:

```go
if err := r.reconcileHTTPRoute(
	ctx,
	modelService,
	labels,
); err != nil {
	return ctrl.Result{}, err
}
```

This call occurs after NetworkPolicy reconciliation and before status reconciliation.

### 9.1 Disabled behavior

When exposure is disabled:

1. The controller tries to retrieve the route.
2. If it does not exist, reconciliation succeeds without action.
3. If retrieval fails for another reason, the error is returned.
4. If a route exists but is not controlled by the `ModelService`, deletion is refused.
5. If the route is owned by the `ModelService`, it is deleted.

This protects unrelated or manually created HTTPRoutes from accidental deletion.

### 9.2 Enabled behavior

When exposure is enabled:

1. `hostname` must be non-empty.
2. Gateway API typed values are prepared.
3. `controllerutil.CreateOrUpdate` creates or updates the route.
4. Labels are synchronized.
5. Parent Gateway reference is synchronized.
6. Hostname is synchronized.
7. Path-prefix match is synchronized.
8. Backend `Service` name and port are synchronized.
9. A controller owner reference is established.

The generated route has the following effective structure:

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

### 9.3 Ownership

The operator uses:

```go
controllerutil.SetControllerReference(
	modelService,
	route,
	r.Scheme,
)
```

This makes the `ModelService` the controller owner of the generated `HTTPRoute`.

Benefits:

- Kubernetes ownership is explicit.
- The controller can distinguish managed routes from unrelated routes.
- Garbage collection can remove dependents when the owner is deleted.
- Reconciliation can safely manage drift.

---

## 10. NetworkPolicy Integration

Before this milestone, the generated NetworkPolicy allowed same-namespace traffic only.

Because Envoy proxy Pods run in a different namespace, exposure-enabled services require an additional ingress peer.

The updated ingress logic builds a peer list:

```go
peers := []networkingv1.NetworkPolicyPeer{}
```

Same-namespace ingress:

```go
networkingv1.NetworkPolicyPeer{
	PodSelector: &metav1.LabelSelector{},
}
```

Envoy data-plane namespace ingress when exposure is enabled:

```go
networkingv1.NetworkPolicyPeer{
	NamespaceSelector: &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"kubernetes.io/metadata.name":
				exposure.GatewayDataPlaneNamespace,
		},
	},
}
```

The rule allows only TCP traffic to the `ModelService` port.

Effective behavior:

```text
Allowed:
- Pods in the ModelService namespace, when configured
- Pods in envoy-gateway-system, when exposure is enabled

Not allowed:
- Unrestricted ingress from all namespaces
- Arbitrary ports
```

Validated production selector:

```yaml
namespaceSelector:
  matchLabels:
    kubernetes.io/metadata.name: envoy-gateway-system
```

Validated backend port:

```yaml
protocol: TCP
port: 8080
```

---

## 11. Watching Owned HTTPRoutes

`SetupWithManager` was extended with:

```go
Owns(&gatewayv1.HTTPRoute{}).
```

The controller builder now includes the route among its owned resources:

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

Changes or deletions of an owned `HTTPRoute` can therefore trigger reconciliation of its parent `ModelService`.

---

## 12. Sample `ModelService`

File:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

Exposure configuration:

```yaml
spec:
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

In the validated cluster, the generated backend Service exposed port `8080`, and the HTTPRoute propagated that port correctly.

---

## 13. Envtest Support for Gateway API

Registering Gateway API Go types in the test scheme was not enough by itself.

The envtest API server also needed the actual `HTTPRoute` CRD installed.

The standard Gateway API CRD was located at:

```text
/home/ansible/go/pkg/mod/sigs.k8s.io/gateway-api@v1.5.1/config/crd/standard/gateway.networking.k8s.io_httproutes.yaml
```

It was copied into the project:

```bash
mkdir -p config/crd/gateway-api

cp \
  /home/ansible/go/pkg/mod/sigs.k8s.io/gateway-api@v1.5.1/config/crd/standard/gateway.networking.k8s.io_httproutes.yaml \
  config/crd/gateway-api/
```

Result:

```text
config/crd/gateway-api/gateway.networking.k8s.io_httproutes.yaml
```

The test environment was configured with both CRD directories:

```go
testEnv = &envtest.Environment{
	CRDDirectoryPaths: []string{
		filepath.Join("..", "..", "config", "crd", "bases"),
		filepath.Join("..", "..", "config", "crd", "gateway-api"),
	},
	ErrorIfCRDPathMissing: true,
}
```

This distinction is important:

```text
AddToScheme
    → teaches the Go client about HTTPRoute

CRDDirectoryPaths
    → teaches the envtest API server how to store and validate HTTPRoute
```

---

## 14. Controller Test Coverage

The controller test suite validates five main scenarios.

### 14.1 Specification propagation test

The test performs an initial reconciliation, updates the `ModelService`, reconciles again, and verifies that changes propagate to managed resources.

HTTPRoute update assertions include:

- parent Gateway name: `shared-gateway`;
- parent Gateway namespace: `gateway-system`;
- listener section: `public-http` after update;
- hostname: `updated-model.example.test`;
- path prefix: `/v2/predict`;
- backend Service name: the `ModelService` name;
- backend port: `8081` after test update;
- controller owner reference.

The same test verifies the NetworkPolicy contains two ingress peers:

1. same-namespace Pods;
2. `envoy-gateway-system`.

### 14.2 NetworkPolicy delete test

The test:

1. reconciles the resource;
2. confirms the NetworkPolicy exists;
3. sets `spec.networkPolicy.enabled=false`;
4. updates the `ModelService`;
5. reconciles again;
6. confirms the owned NetworkPolicy is deleted.

### 14.3 HTTPRoute delete test

The test:

1. performs initial reconciliation;
2. confirms the HTTPRoute exists;
3. retrieves the current `ModelService`;
4. verifies `spec.exposure` is present;
5. sets `spec.exposure.enabled=false`;
6. updates the resource;
7. reconciles again;
8. confirms the HTTPRoute returns `NotFound`.

### 14.4 Ownership test

The ownership test confirms controller ownership for managed resources, including:

- ServiceAccount;
- Deployment;
- Service;
- PersistentVolumeClaim;
- PodDisruptionBudget;
- NetworkPolicy;
- HTTPRoute.

HTTPRoute ownership is checked with:

```go
Expect(
	metav1.IsControlledBy(
		httpRoute,
		modelService,
	),
).To(BeTrue())
```

### 14.5 ServiceAccount drift test

The test deliberately changes the managed ServiceAccount’s secure token-mount configuration, reconciles again, and verifies that the controller restores the desired value.

This confirms that adding HTTPRoute support did not break existing drift-remediation behavior.

---

## 15. Build and Test Validation

Commands:

```bash
cd /mnt/data/ai-platform-operator

gofmt -w \
  cmd/main.go \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go \
  internal/controller/suite_test.go

go mod tidy
make generate
make manifests
make build
make test
```

Observed successful result:

```text
go build -o bin/manager cmd/main.go

ok github.com/anselem-okeke/ai-platform-operator/internal/controller
coverage: 79.2% of statements
```

Final controller coverage:

```text
79.2%
```

---

## 16. Cluster Installation and Execution

### 16.1 Confirm context

```bash
kubectl config current-context
```

Target context:

```text
kind-ai-platform-policy
```

Switch when necessary:

```bash
kubectl config use-context kind-ai-platform-policy
```

### 16.2 Install the updated ModelService CRD

```bash
cd /mnt/data/ai-platform-operator
make install
```

### 16.3 Run the controller

```bash
make run
```

This command remains running while cluster validation is performed from another terminal.

### 16.4 Apply the sample

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Observed:

```text
modelservice.platform.anselem.dev/fraud-model configured
```

---

## 17. Cluster Resource Validation

### 17.1 ModelService

```bash
kubectl get modelservice -n ai-platform
```

Observed state:

```text
NAME          PHASE   READY   IMAGE                                     ENDPOINT
fraud-model   Ready   2       nginxinc/nginx-unprivileged:1.31-alpine   http://fraud-model.ai-platform.svc.cluster.local:8080
```

### 17.2 Managed resources

```bash
kubectl get deployment,service,serviceaccount,pvc,pdb,networkpolicy \
  -n ai-platform
```

Validated resources included:

- `Deployment/fraud-model`
- `Service/fraud-model`
- `ServiceAccount/fraud-model`
- `PersistentVolumeClaim/fraud-model`
- `PodDisruptionBudget/fraud-model`
- `NetworkPolicy/fraud-model`

The Service exposed:

```text
8080/TCP
```

### 17.3 HTTPRoute

```bash
kubectl get httproute -n ai-platform
```

Observed:

```text
NAME          HOSTNAMES
fraud-model   ["fraud-model.local"]
```

---

## 18. HTTPRoute Status Validation

Command:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Observed:

```text
Accepted=True reason=Accepted
ResolvedRefs=True reason=ResolvedRefs
```

Meaning:

### `Accepted=True`

Envoy Gateway accepted the route configuration and attached it to the referenced Gateway listener.

### `ResolvedRefs=True`

All object references in the route were successfully resolved, including the backend `Service`.

---

## 19. Validated HTTPRoute Manifest

The real cluster produced:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  labels:
    app.kubernetes.io/instance: fraud-model
    app.kubernetes.io/managed-by: ai-platform-operator
    app.kubernetes.io/name: modelservice
  name: fraud-model
  namespace: ai-platform
  ownerReferences:
  - apiVersion: platform.anselem.dev/v1alpha1
    blockOwnerDeletion: true
    controller: true
    kind: ModelService
    name: fraud-model
spec:
  hostnames:
  - fraud-model.local
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: shared-gateway
    namespace: gateway-system
    sectionName: http
  rules:
  - backendRefs:
    - group: ""
      kind: Service
      name: fraud-model
      port: 8080
      weight: 1
    matches:
    - path:
        type: PathPrefix
        value: /
status:
  parents:
  - conditions:
    - message: Route is accepted
      reason: Accepted
      status: "True"
      type: Accepted
    - message: Resolved all the Object references for the Route
      reason: ResolvedRefs
      status: "True"
      type: ResolvedRefs
    controllerName: gateway.envoyproxy.io/gatewayclass-controller
```

This manifest confirms:

- hostname propagation;
- path-prefix propagation;
- Service-port propagation;
- Gateway namespace propagation;
- listener propagation;
- owner reference;
- Gateway controller acceptance;
- backend reference resolution.

---

## 20. Gateway Validation

The Gateway address was retrieved with:

```bash
GATEWAY_IP=$(kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{.status.addresses[0].value}')

echo "$GATEWAY_IP"
```

Observed:

```text
172.19.255.200
```

Gateway status:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system
```

Observed:

```text
NAME             CLASS   ADDRESS          PROGRAMMED
shared-gateway   envoy   172.19.255.200   True
```

`PROGRAMMED=True` confirms the Gateway data plane was configured successfully.

---

## 21. External Traffic Validation

### 21.1 Correct hostname

Request:

```bash
curl -v \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

The request successfully reached the backend.

`--resolve` maps the hostname to the Gateway address for this request only:

```text
fraud-model.local → 172.19.255.200
```

It does not modify `/etc/hosts`.

### 21.2 Incorrect hostname

Request:

```bash
curl -v \
  -H 'Host: wrong-host.local' \
  --connect-timeout 5 \
  http://"$GATEWAY_IP"/
```

Observed response:

```text
HTTP/1.1 404 Not Found
```

This proves that the route matches only the configured hostname and that an unrelated `Host` header does not reach the backend.

---

## 22. Exposure Disable and Re-enable Validation

### 22.1 Disable exposure

Command:

```bash
kubectl patch modelservice fraud-model \
  -n ai-platform \
  --type merge \
  -p '{"spec":{"exposure":{"enabled":false}}}'
```

Observed:

```text
modelservice.platform.anselem.dev/fraud-model patched
```

Route verification:

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

Observed:

```text
Error from server (NotFound):
httproutes.gateway.networking.k8s.io "fraud-model" not found
```

This confirms that disabling exposure deletes the owned route.

### 22.2 Re-enable exposure

Command:

```bash
kubectl patch modelservice fraud-model \
  -n ai-platform \
  --type merge \
  -p '{"spec":{"exposure":{"enabled":true}}}'
```

Observed:

```text
modelservice.platform.anselem.dev/fraud-model patched
```

Route verification:

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

Observed:

```text
NAME          HOSTNAMES               AGE
fraud-model   ["fraud-model.local"]   9s
```

This confirms that re-enabling exposure recreates the route.

---

## 23. Final Completion Matrix

| Validation item | Result |
|---|---:|
| HTTPRoute created | ✅ |
| HTTPRoute accepted | ✅ |
| Backend reference resolved | ✅ |
| Hostname propagated | ✅ |
| Path prefix propagated | ✅ |
| Service port propagated | ✅ |
| Gateway namespace propagated | ✅ |
| Gateway listener propagated | ✅ |
| Envoy namespace allowed by NetworkPolicy | ✅ |
| Owner reference configured | ✅ |
| Correct hostname reaches backend | ✅ |
| Incorrect hostname returns 404 | ✅ |
| Disabling exposure deletes HTTPRoute | ✅ |
| Re-enabling exposure recreates HTTPRoute | ✅ |
| Controller tests pass | ✅ |
| Controller coverage | **79.2%** |

---

## 24. Operational Verification Commands

### Check route

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

### Check route conditions

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

### Inspect route

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

### Inspect NetworkPolicy

```bash
kubectl get networkpolicy fraud-model \
  -n ai-platform \
  -o yaml
```

### Check Gateway

```bash
kubectl get gateway shared-gateway \
  -n gateway-system
```

### Test correct hostname

```bash
GATEWAY_IP=$(kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{.status.addresses[0].value}')

curl -v \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

### Test incorrect hostname

```bash
curl -v \
  -H 'Host: wrong-host.local' \
  --connect-timeout 5 \
  http://"$GATEWAY_IP"/
```

### Disable exposure

```bash
kubectl patch modelservice fraud-model \
  -n ai-platform \
  --type merge \
  -p '{"spec":{"exposure":{"enabled":false}}}'
```

### Re-enable exposure

```bash
kubectl patch modelservice fraud-model \
  -n ai-platform \
  --type merge \
  -p '{"spec":{"exposure":{"enabled":true}}}'
```

---

## 25. Troubleshooting Notes

### `AddToScheme` appears struck through

This is typically an IDE deprecation indication, not a compilation failure.

The following registration was validated successfully:

```go
gatewayv1.Install(scheme.Scheme)
```

### Nil pointer during test-suite startup

Incorrect:

```go
gatewayv1.Install(k8sClient.Scheme())
```

when `k8sClient` has not yet been initialized.

Correct:

```go
gatewayv1.Install(scheme.Scheme)
```

### `no matches for kind "HTTPRoute"`

The Go scheme may be registered while the envtest API server still lacks the CRD.

Ensure this file exists:

```text
config/crd/gateway-api/gateway.networking.k8s.io_httproutes.yaml
```

Ensure `suite_test.go` includes:

```go
filepath.Join("..", "..", "config", "crd", "gateway-api")
```

### Route exists but is not accepted

Check:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

Review:

- parent Gateway name;
- parent Gateway namespace;
- listener section name;
- namespace attachment policy;
- Gateway status;
- route conditions.

### `ResolvedRefs=False`

Check that:

- `Service/fraud-model` exists;
- the Service is in the same namespace as the route;
- the backend port matches the Service port;
- the backend kind is `Service`.

### Correct route but traffic is blocked

Inspect the NetworkPolicy:

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

and confirm the allowed port matches the Service port.

---

## 26. Suggested Git Commit

Review changes:

```bash
git status
```

Stage the milestone:

```bash
git add \
  go.mod \
  go.sum \
  cmd/main.go \
  api/v1alpha1/modelservice_types.go \
  api/v1alpha1/zz_generated.deepcopy.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go \
  internal/controller/suite_test.go \
  config/crd/bases \
  config/crd/gateway-api \
  config/rbac/role.yaml \
  config/samples/platform_v1alpha1_modelservice.yaml
```

Commit:

```bash
git commit -m "feat: add optional Gateway API exposure for ModelService"
```

---

## 27. Final Outcome

The `ModelService` operator now supports secure, optional, declarative external exposure through Kubernetes Gateway API.

The implementation:

- creates an `HTTPRoute` only when requested;
- synchronizes hostname, path, listener, Gateway, Service, and port;
- grants only the required RBAC permissions;
- allows only the Envoy data-plane namespace through NetworkPolicy;
- establishes controller ownership;
- rejects unmatched hostnames;
- deletes exposure cleanly when disabled;
- recreates exposure when re-enabled;
- is covered by envtest-based controller tests;
- reached **79.2% controller coverage**;
- was successfully validated end to end in the Kubernetes cluster.
