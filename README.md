# AI Platform Operator

A Kubernetes operator and secure platform foundation for deploying, exposing, and managing model-serving workloads through a declarative `ModelService` custom resource.

The project combines Kubernetes-native lifecycle management with secure HTTPS exposure, Keycloak-based OpenID Connect authentication, JWT validation, role-based authorization, Vault-backed certificate issuance, and least-privilege security controls.

---

## Overview

The AI Platform Operator allows platform users to define a model-serving workload with a single Kubernetes custom resource:

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

  exposure:
    enabled: true
    hostname: fraud-model.local
    pathPrefix: /
    gatewayName: shared-gateway
    gatewayNamespace: gateway-system
    gatewaySectionName: fraud-model-https
    gatewayDataPlaneNamespace: envoy-gateway-system

  security:
    runAsNonRoot: true
    runAsUser: 101
    runAsGroup: 101
    fsGroup: 101
    readOnlyRootFilesystem: true
    automountServiceAccountToken: false

  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

The operator reconciles the desired state into the Kubernetes resources required to run the service.

---

## Architecture

```text
User or CI/CD client
  ↓
Keycloak
  ↓ JWT access token
Envoy Gateway
  ↓
JWT signature, issuer, audience, and expiry validation
  ↓
Role and HTTP-method authorization
  ↓
HTTPRoute
  ↓
Service
  ↓
Model-serving Pods
```

The control-plane reconciliation path is:

```text
ModelService custom resource
  ↓
AI Platform Operator
  ├── Deployment
  ├── Service
  ├── ServiceAccount
  ├── HTTPRoute
  ├── PersistentVolumeClaim
  ├── PodDisruptionBudget
  └── NetworkPolicy
```

Certificate issuance follows:

```text
cert-manager
  ↓
Vault Kubernetes authentication
  ↓
Vault PKI
  ↓
Certificate
  ↓
Kubernetes TLS Secret
  ↓
Envoy Gateway HTTPS listener
```

---

## Features

- Declarative `ModelService` API
- Kubernetes reconciliation and status reporting
- Deployment and replica lifecycle management
- Service and Gateway API exposure
- Hostname-specific HTTPS listeners
- Vault-issued TLS certificates through cert-manager
- Keycloak OpenID Connect authentication
- Machine-to-machine client-credentials flow
- Human Authorization Code flow with PKCE
- JWT signature, issuer, audience, and expiry validation
- Role-based and HTTP-method-based authorization
- Persistent volume support
- PodDisruptionBudget support
- NetworkPolicy generation
- Non-root workload security
- Read-only root filesystem
- ServiceAccount token automount disabled by default
- Least-privilege operator RBAC
- Automated installation and validation scripts
- Recovery and troubleshooting documentation

---

## Technology Stack

| Area | Technology |
|---|---|
| Operator | Go, Kubebuilder, controller-runtime |
| Kubernetes | Kubernetes, Kind |
| Ingress | Envoy Gateway, Gateway API |
| Identity | Keycloak |
| Authentication | OpenID Connect, OAuth 2.0, JWT |
| Authorization | Keycloak realm roles, Envoy SecurityPolicy |
| Certificates | Vault PKI, cert-manager |
| Load balancing | MetalLB |
| Database | PostgreSQL |
| Security | RBAC, NetworkPolicy, Pod security context |
| Testing | Go tests, shell validation scripts, Python JWT validation |

---


---

## Prerequisites

The local development environment used for this project includes:

```text
Kubernetes:
  v1.36.1

Kind cluster:
  ai-platform-policy

Kubernetes context:
  kind-ai-platform-policy

Gateway:
  gateway-system/shared-gateway

Application namespace:
  ai-platform

Keycloak namespace:
  keycloak

Envoy data-plane namespace:
  envoy-gateway-system

Vault:
  https://vault.platform.local:8200
```

Required tools:

```text
go
make
docker
kind
kubectl
jq
curl
openssl
python3
```

Required cluster components:

```text
Gateway API CRDs
Envoy Gateway
MetalLB
cert-manager
Vault PKI
```

---

## Quick Start

### 1. Clone the repository

```bash
git clone <repository-url>
cd ai-platform-operator
```

### 2. Create or select the Kubernetes cluster

```bash
kubectl config use-context kind-ai-platform-policy
```

Verify:

```bash
kubectl config current-context
kubectl get nodes
```

### 3. Generate code and manifests

```bash
make generate
make manifests
```

### 4. Run tests

```bash
make test
```

### 5. Install the CRD

```bash
make install
```

### 6. Deploy the operator

```bash
make deploy
```

Wait for the controller:

```bash
kubectl rollout status \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system \
  --timeout=180s
```

### 7. Create the application namespace

```bash
kubectl create namespace ai-platform \
  --dry-run=client \
  -o yaml \
  | kubectl apply -f -
```

### 8. Apply the sample ModelService

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

### 9. Verify reconciliation

```bash
kubectl get modelservice \
  -n ai-platform
```

```bash
kubectl get \
  deployment,service,serviceaccount,pvc,pdb,networkpolicy,httproute \
  -n ai-platform
```

---

## ModelService Lifecycle

The operator follows a level-triggered reconciliation model.

```text
Desired state
  ↓
ModelService spec
  ↓
Controller reconciliation
  ↓
Observed Kubernetes resources
  ↓
ModelService status
```

The controller is designed to be:

- idempotent;
- retry-safe;
- status-aware;
- owner-reference driven;
- compatible with Kubernetes reconciliation patterns.

When a managed child resource is deleted, the operator recreates it during the next reconciliation cycle.

---

## HTTPS Exposure

The sample workload is exposed at:

```text
https://fraud-model.local
```

The shared Gateway includes a dedicated listener:

```text
fraud-model-https
```

The listener uses the TLS Secret:

```text
gateway-system/fraud-model-local-tls
```

HTTP requests are redirected to HTTPS through:

```text
HTTPRoute/fraud-model-http-redirect
```

Expected behavior:

```text
HTTP:
  301 redirect

HTTPS without JWT:
  401 Unauthorized

HTTPS with valid and authorized JWT:
  backend response
```

---

## Keycloak OIDC Configuration

The Keycloak realm is:

```text
ai-platform
```

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Expected audience:

```text
ai-platform-gateway
```

Configured clients:

### `ai-platform-gateway`

Bearer-only resource server and expected JWT audience.

### `ai-platform-cli`

Public client for human authentication.

```text
Authorization Code:
  enabled

PKCE:
  S256

Direct Access Grant:
  disabled
```

### `ai-platform-service`

Confidential client for machine-to-machine authentication.

```text
Service account:
  enabled

Client Credentials:
  enabled
```

---

## Roles

```text
platform-admin
  └── model-deployer
        └── model-viewer
```

Role behavior:

| Role | Read | Create/Update | Delete |
|---|---:|---:|---:|
| `model-viewer` | Yes | No | No |
| `model-deployer` | Yes | Yes | No |
| `platform-admin` | Yes | Yes | Yes |

Test identities:

```text
viewer-user
deployer-user
admin-user
service-account-ai-platform-service
```

---

## Authentication and Authorization Outcomes

```text
No token:
  401 Unauthorized

Malformed token:
  401 Unauthorized

Expired token:
  401 Unauthorized

Valid token without sufficient role:
  403 Forbidden

Valid and authorized method unsupported by nginx:
  405 Method Not Allowed

Valid and authorized GET:
  200 OK
```

---

## Kubernetes Security

The workload security model includes:

```text
runAsNonRoot: true
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
capabilities.drop:
  - ALL
automountServiceAccountToken: false
```

The generated ServiceAccount and PodSpec both disable automatic Kubernetes API token mounting.

The workload is not granted permission to:

```text
list Pods
read Secrets
create Pods
request ServiceAccount tokens
access ModelService resources
```

The operator is allowed to reconcile its managed resources but is not allowed to:

```text
create ModelService parent resources
delete ModelService parent resources
read Secrets
list Nodes
request ServiceAccount tokens
modify Kubernetes RBAC
```

## Development

### Format Go code

```bash
gofmt -w \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go
```

### Generate code

```bash
make generate
```

### Generate manifests

```bash
make manifests
```

### Run tests

```bash
make test
```

### Run the controller locally

```bash
make run
```

---

## Troubleshooting

### Operator not reconciling

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager \
  --tail=200
```

### Route missing

```bash
kubectl get modelservice fraud-model \
  -n ai-platform
```

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

### Gateway not programmed

```bash
kubectl describe gateway shared-gateway \
  -n gateway-system
```

### Valid token returns `401`

Check:

```text
token expiry
issuer
audience
JWKS reachability
signature
Authorization header
```

### Valid token returns `403`

Check:

```text
realm_access.roles
HTTP method
authorization rule
composite-role inheritance
```

See:

```text
docs/oidc-jwt/13-recovery-and-troubleshooting.md
```

The REST API will be implemented in Go and will manage only `ModelService` resources. The operator will remain responsible for child-resource reconciliation.

---

## License

- This project is licensed under the Apache License 2.0.

- See: [LICENSE](LICENSE)

> Any third-party code, templates, or dependencies remain subject to their respective licenses and attribution requirements.
