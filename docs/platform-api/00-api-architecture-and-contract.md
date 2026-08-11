# AI Platform REST API Architecture and Contract

## Purpose

This document defines the initial architecture, responsibility boundaries, API surface, request and response contracts, authorization model, namespace policy, validation rules, health semantics, and Kubernetes access requirements for the AI Platform REST API.

The REST API is the customer-facing control-plane interface for creating and managing `ModelService` resources without requiring users or CI/CD systems to interact directly with the Kubernetes API.

The implementation language is Go.

## API Architecture

```text
User / CI/CD client
  ↓
Keycloak
  ↓ JWT access token
Envoy Gateway
  ↓
JWT authentication
  ↓
Coarse role and HTTP-method authorization
  ↓
AI Platform REST API
  ↓
Request validation
  ↓
API-level authorization
  ↓
Kubernetes API
  ↓
ModelService Custom Resource
  ↓
AI Platform Operator
  ↓
Deployment
Service
HTTPRoute
PersistentVolumeClaim
PodDisruptionBudget
NetworkPolicy
ServiceAccount
  ↓
Model-serving Pods
```

## Responsibility Boundary

### AI Platform REST API

The API is responsible for:

- receiving HTTP requests;
- validating request bodies;
- validating JWT-derived identity and roles;
- enforcing API-level authorization;
- applying platform defaults;
- creating, reading, updating, patching, and deleting `ModelService` resources;
- reading `ModelService` status;
- returning stable API responses and errors.

### AI Platform Operator

The operator is responsible for:

- watching `ModelService` resources;
- reconciling desired state;
- creating Deployments, Services, HTTPRoutes, PVCs, PDBs, NetworkPolicies, and ServiceAccounts;
- updating `ModelService` status;
- restoring deleted child resources.

The intended boundary is:

```text
REST API
  ↓ manages ModelService resources only

Operator
  ↓ manages child Kubernetes resources
```

The REST API must not become a second Kubernetes operator.

## Why the REST API Is Needed

Without the REST API, users need:

```text
kubectl
Kubernetes credentials
knowledge of the ModelService CRD
direct access to the Kubernetes API
knowledge of platform-managed fields
```

With the REST API, users need:

```text
an HTTPS endpoint
a valid Keycloak token
a documented JSON request
```

## Trust Boundaries

### Client → Envoy Gateway

```text
HTTPS
JWT bearer token
```

Envoy performs edge authentication and coarse authorization.

### Envoy Gateway → AI Platform REST API

The API still performs its own authorization checks. This gives defense in depth:

```text
Envoy Gateway
  ↓ edge authentication and coarse authorization
AI Platform REST API
  ↓ resource and business authorization
```

### REST API → Kubernetes API

The API runs with a dedicated ServiceAccount and receives permission only to manage `ModelService` resources.

It should not directly manage Deployments, Services, Secrets, ServiceAccounts, RBAC, Nodes, HTTPRoutes, PVCs, or NetworkPolicies.

## Namespace Policy

The first API version uses one configured namespace:

```text
MODEL_SERVICE_NAMESPACE=ai-platform
```

Clients cannot choose arbitrary Kubernetes namespaces.

The public API therefore uses:

```text
/api/v1/model-services
```

instead of:

```text
/api/v1/namespaces/{namespace}/model-services
```

Internally:

```text
API request
  ↓
configured namespace = ai-platform
  ↓
metadata.namespace = ai-platform
```

## API Base Path

```text
/api/v1
```

## API Endpoints

| Method | Endpoint | Purpose | Minimum Role |
|---|---|---|---|
| `GET` | `/healthz` | Process liveness | Public |
| `GET` | `/readyz` | API readiness | Public/Internal |
| `GET` | `/api/v1/model-services` | List ModelServices | `model-viewer` |
| `GET` | `/api/v1/model-services/{name}` | Get one ModelService | `model-viewer` |
| `GET` | `/api/v1/model-services/{name}/status` | Get runtime status | `model-viewer` |
| `POST` | `/api/v1/model-services` | Create ModelService | `model-deployer` |
| `PUT` | `/api/v1/model-services/{name}` | Replace mutable configuration | `model-deployer` |
| `PATCH` | `/api/v1/model-services/{name}` | Partially update configuration | `model-deployer` |
| `DELETE` | `/api/v1/model-services/{name}` | Delete ModelService | `platform-admin` |

## Role Model

```text
platform-admin
  └── model-deployer
        └── model-viewer
```

### `model-viewer`

Allowed:

```text
GET /api/v1/model-services
GET /api/v1/model-services/{name}
GET /api/v1/model-services/{name}/status
```

### `model-deployer`

Allowed:

```text
all model-viewer operations
POST /api/v1/model-services
PUT /api/v1/model-services/{name}
PATCH /api/v1/model-services/{name}
```

### `platform-admin`

Allowed:

```text
all model-deployer operations
DELETE /api/v1/model-services/{name}
```

## Authentication Contract

Protected endpoints require:

```http
Authorization: Bearer <JWT>
```

Expected token properties:

```text
issuer:
  https://auth.ai-platform.local/realms/ai-platform

audience:
  ai-platform-gateway

signature:
  valid Keycloak signing key

expiry:
  token must not be expired
```

Roles are read from:

```text
realm_access.roles
```

## Create ModelService

### Request

```http
POST /api/v1/model-services
Content-Type: application/json
Authorization: Bearer <JWT>
```

```json
{
  "name": "fraud-model",
  "image": "ghcr.io/example/fraud-model:v1.0.0",
  "replicas": 2,
  "port": 8080,
  "exposure": {
    "enabled": true,
    "hostname": "fraud-model.local",
    "pathPrefix": "/"
  },
  "storage": {
    "enabled": true,
    "size": "1Gi",
    "mountPath": "/models"
  }
}
```

### Client-Controlled Fields

```text
name
image
replicas
port
exposure.enabled
exposure.hostname
exposure.pathPrefix
storage.enabled
storage.size
storage.mountPath
```

### Platform-Controlled Fields

Clients must not control:

```text
apiVersion
kind
metadata.namespace
metadata.ownerReferences
metadata.finalizers
status
gatewayName
gatewayNamespace
gatewaySectionName
gatewayDataPlaneNamespace
ServiceAccount name
RBAC resources
NetworkPolicy internals
runAsNonRoot
privileged settings
Linux capabilities
automountServiceAccountToken
```

## Platform Defaults

A client request can be translated internally to a secure `ModelService`:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: ghcr.io/example/fraud-model:v1.0.0
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
    readOnlyRootFilesystem: true
    automountServiceAccountToken: false
```

The REST API is therefore a controlled platform abstraction rather than a raw Kubernetes proxy.

## Create Response

```http
HTTP/1.1 201 Created
Content-Type: application/json
```

```json
{
  "apiVersion": "v1",
  "kind": "ModelService",
  "name": "fraud-model",
  "namespace": "ai-platform",
  "generation": 1,
  "state": "Pending",
  "links": {
    "self": "/api/v1/model-services/fraud-model",
    "status": "/api/v1/model-services/fraud-model/status"
  }
}
```

## List ModelServices

```http
GET /api/v1/model-services
Authorization: Bearer <JWT>
```

```json
{
  "items": [
    {
      "name": "fraud-model",
      "image": "ghcr.io/example/fraud-model:v1.0.0",
      "replicas": 2,
      "state": "Ready",
      "hostname": "fraud-model.local"
    }
  ],
  "count": 1
}
```

## Get ModelService

```http
GET /api/v1/model-services/fraud-model
Authorization: Bearer <JWT>
```

```json
{
  "apiVersion": "v1",
  "kind": "ModelService",
  "name": "fraud-model",
  "image": "ghcr.io/example/fraud-model:v1.0.0",
  "replicas": 2,
  "port": 8080,
  "exposure": {
    "enabled": true,
    "hostname": "fraud-model.local",
    "pathPrefix": "/"
  },
  "storage": {
    "enabled": true,
    "size": "1Gi",
    "mountPath": "/models"
  },
  "state": "Ready",
  "generation": 1
}
```

## ModelService Status

```http
GET /api/v1/model-services/fraud-model/status
Authorization: Bearer <JWT>
```

```json
{
  "name": "fraud-model",
  "state": "Ready",
  "observedGeneration": 1,
  "desiredReplicas": 2,
  "readyReplicas": 2,
  "endpoint": "https://fraud-model.local",
  "conditions": [
    {
      "type": "Ready",
      "status": "True",
      "reason": "DeploymentReady",
      "message": "ModelService is available"
    }
  ]
}
```

## PUT Semantics

`PUT` replaces the mutable API representation.

```http
PUT /api/v1/model-services/fraud-model
Content-Type: application/json
Authorization: Bearer <JWT>
```

```json
{
  "image": "ghcr.io/example/fraud-model:v1.1.0",
  "replicas": 3,
  "port": 8080,
  "exposure": {
    "enabled": true,
    "hostname": "fraud-model.local",
    "pathPrefix": "/"
  },
  "storage": {
    "enabled": true,
    "size": "2Gi",
    "mountPath": "/models"
  }
}
```

The URL name is authoritative.

## PATCH Semantics

The initial version uses JSON Merge Patch:

```text
Content-Type: application/merge-patch+json
```

```http
PATCH /api/v1/model-services/fraud-model
```

```json
{
  "replicas": 4,
  "image": "ghcr.io/example/fraud-model:v1.2.0"
}
```

Arbitrary Kubernetes JSON Patch is not exposed initially.

## DELETE Semantics

```http
DELETE /api/v1/model-services/fraud-model
Authorization: Bearer <JWT>
```

Minimum role:

```text
platform-admin
```

Successful response:

```http
HTTP/1.1 202 Accepted
```

```json
{
  "name": "fraud-model",
  "state": "Deleting",
  "message": "ModelService deletion accepted"
}
```

## Validation Rules

### Name

```text
required
valid Kubernetes DNS-1123 name
maximum 63 characters
```

### Image

```text
required
non-empty
sensible maximum length
```

### Replicas

```text
minimum: 1
maximum: 10
```

Configurable with:

```text
MAX_MODEL_REPLICAS=10
```

### Port

```text
1-65535
```

### Hostname

Required when:

```text
exposure.enabled=true
```

It must be a valid DNS hostname.

### Path Prefix

```text
must begin with /
default /
```

### Storage Size

Must be a valid Kubernetes quantity such as:

```text
1Gi
5Gi
500Mi
```

### Storage Mount Path

Must be an absolute path.

Example:

```text
/models
```

## Error Contract

### Generic Error

```json
{
  "error": {
    "code": "MODEL_SERVICE_NOT_FOUND",
    "message": "ModelService "fraud-model" was not found",
    "requestId": "a3105308-976c-4a8d-b637-130bc46c5752"
  }
}
```

### Validation Error

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "Request validation failed",
    "requestId": "a3105308-976c-4a8d-b637-130bc46c5752",
    "details": [
      {
        "field": "replicas",
        "message": "must be between 1 and 10"
      }
    ]
  }
}
```

## HTTP Status Contract

| Situation | HTTP Status |
|---|---:|
| Successful GET | `200 OK` |
| Successful create | `201 Created` |
| Successful update | `200 OK` |
| Successful deletion request | `202 Accepted` |
| Invalid JSON | `400 Bad Request` |
| Validation failure | `400 Bad Request` |
| Missing or invalid JWT | `401 Unauthorized` |
| Insufficient role | `403 Forbidden` |
| Resource not found | `404 Not Found` |
| Resource already exists | `409 Conflict` |
| Request too large | `413 Content Too Large` |
| Unsupported media type | `415 Unsupported Media Type` |
| Unexpected API failure | `500 Internal Server Error` |
| Kubernetes dependency unavailable | `503 Service Unavailable` |

## Health Endpoints

### `/healthz`

Purpose:

```text
process liveness
```

It must not depend on Kubernetes, Keycloak, Vault, or Envoy Gateway.

```json
{
  "status": "ok"
}
```

### `/readyz`

Purpose:

```text
determine whether the API can serve platform requests
```

Initial readiness checks:

```text
Kubernetes client initialized
Kubernetes API reachable
ModelService CRD available
```

Success:

```json
{
  "status": "ready"
}
```

Failure:

```http
503 Service Unavailable
```

```json
{
  "status": "not-ready"
}
```

## Kubernetes Client

The API uses:

```text
sigs.k8s.io/controller-runtime/pkg/client
```

This allows the REST API to reuse the existing Go `ModelService` types from:

```text
api/v1alpha1
```

## Kubernetes ServiceAccount

The REST API will use a dedicated identity:

```text
ServiceAccount/ai-platform-api
```

## Kubernetes RBAC

The API requires only:

```yaml
apiGroups:
  - platform.anselem.dev

resources:
  - modelservices

verbs:
  - get
  - list
  - create
  - update
  - patch
  - delete
```

It should not receive access to:

```text
Secrets
Nodes
Deployments
Pods
Services
ServiceAccounts
Roles
RoleBindings
ClusterRoles
ClusterRoleBindings
HTTPRoutes
PVCs
NetworkPolicies
```

## Go Project Structure

Recommended structure:

```text
cmd/
├── main.go
└── platform-api/
    └── main.go

internal/
├── controller/
│   └── modelservice_controller.go
│
└── api/
    ├── server.go
    ├── routes.go
    ├── handlers/
    │   ├── health.go
    │   ├── readiness.go
    │   ├── list_modelservices.go
    │   ├── get_modelservice.go
    │   ├── get_modelservice_status.go
    │   ├── create_modelservice.go
    │   ├── update_modelservice.go
    │   ├── patch_modelservice.go
    │   └── delete_modelservice.go
    ├── middleware/
    │   ├── authentication.go
    │   ├── authorization.go
    │   ├── logging.go
    │   └── request_id.go
    ├── request/
    │   └── modelservice.go
    ├── response/
    │   ├── modelservice.go
    │   └── error.go
    └── validation/
        └── modelservice.go
```

The structure may evolve as the API grows.

## Initial Configuration

```text
HTTP_ADDRESS=:8080
MODEL_SERVICE_NAMESPACE=ai-platform
MAX_MODEL_REPLICAS=10
OIDC_ISSUER=https://auth.ai-platform.local/realms/ai-platform
OIDC_AUDIENCE=ai-platform-gateway
LOG_LEVEL=info
```

Platform-managed Gateway settings may include:

```text
MODEL_GATEWAY_NAME=shared-gateway
MODEL_GATEWAY_NAMESPACE=gateway-system
MODEL_GATEWAY_SECTION_NAME=fraud-model-https
MODEL_GATEWAY_DATAPLANE_NAMESPACE=envoy-gateway-system
```

## Request Processing Flow

```text
1. Client sends HTTPS request
2. Envoy validates JWT
3. Envoy performs coarse authorization
4. API receives request
5. Request ID is created or propagated
6. API validates identity and role
7. JSON body is decoded
8. Request schema is validated
9. Platform policies are validated
10. API creates or modifies a ModelService
11. Kubernetes API persists the ModelService
12. Operator observes the change
13. Operator reconciles child resources
14. API returns the accepted resource state
```

## Design Principles

```text
least privilege
secure defaults
deny by default
do not expose raw Kubernetes internals unnecessarily
do not accept arbitrary Kubernetes manifests
keep operator and API responsibilities separate
keep the API contract stable even if CRD internals evolve
return consistent errors
support idempotent operations where practical
log security-relevant actions
never log access tokens or client secrets
```

## Initial Non-Goals

```text
multi-cluster support
arbitrary namespace selection
arbitrary Kubernetes YAML submission
raw Deployment management
raw HTTPRoute management
raw Secret management
model registry integration
object storage management
GitOps promotion
multi-tenancy
quota management
autoscaling policy APIs
web UI
```

## Completion Criteria

```text
[✓] API architecture defined
[✓] REST API and operator responsibilities separated
[✓] trust boundaries defined
[✓] namespace policy defined
[✓] API base path defined
[✓] endpoint list defined
[✓] role-to-operation mapping defined
[✓] authentication contract defined
[✓] create request and response defined
[✓] list response defined
[✓] get response defined
[✓] status response defined
[✓] PUT semantics defined
[✓] PATCH semantics defined
[✓] DELETE semantics defined
[✓] validation rules defined
[✓] error response contract defined
[✓] HTTP status contract defined
[✓] health semantics defined
[✓] readiness semantics defined
[✓] Kubernetes client boundary defined
[✓] least-privilege API RBAC defined
[✓] initial Go project structure defined
```

## Next Implementation Step

```text
[ ] Go API project structure created
[ ] Health and readiness endpoints added
[ ] Configuration loading added
[ ] Structured logging added
```
