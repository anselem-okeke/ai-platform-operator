# OIDC/JWT Security for the AI Platform Operator

## 1. Purpose

This document explains the final authentication, authorization, transport-security, and Kubernetes-security architecture implemented for the AI Platform Operator.

The goal of the implementation is to protect a model-serving workload with:

- Keycloak-based OpenID Connect authentication;
- JWT access tokens for human and machine identities;
- Vault-issued TLS certificates;
- Envoy Gateway JWT validation;
- role-based HTTP authorization;
- least-privilege Kubernetes RBAC;
- disabled Kubernetes API token mounting for model workloads;
- automated end-to-end validation.

The protected example workload is:

```text
ModelService: fraud-model
Namespace:    ai-platform
Hostname:     fraud-model.local
```

The identity provider is:

```text
Keycloak hostname: auth.ai-platform.local
Realm:             ai-platform
Audience:          ai-platform-gateway
```

---

## 2. Final Request Architecture

```text
Client
  ↓
Keycloak authorization or token endpoint
  ↓
JWT access token
  ↓
Vault-issued HTTPS certificate
  ↓
Envoy Gateway
  ↓
JWT signature validation
  ↓
Issuer validation
  ↓
Audience validation
  ↓
Realm-role validation
  ↓
HTTP-method authorization
  ↓
HTTPRoute
  ↓
Service
  ↓
fraud-model Pods
```

A request is allowed to reach the backend only after all required security checks pass.

---

## 3. Main Components

### 3.1 Client

The client can be either:

- a human user authenticating through Authorization Code with PKCE;
- a machine identity authenticating through the OAuth 2.0 client-credentials flow.

Examples:

```text
viewer-user
admin-user
ai-platform-service
```

The client sends the resulting access token as:

```http
Authorization: Bearer <access-token>
```

---

### 3.2 Keycloak

Keycloak is the OpenID Connect identity provider.

It performs:

- user authentication;
- machine authentication;
- client management;
- role assignment;
- token issuance;
- token signing;
- publication of OIDC discovery metadata;
- publication of JSON Web Keys through JWKS.

Keycloak is deployed in:

```text
Namespace: keycloak
```

External address:

```text
https://auth.ai-platform.local
```

Realm:

```text
ai-platform
```

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Keycloak runs behind Envoy Gateway. TLS terminates at Envoy, while Keycloak receives HTTP internally on port `8080`.

---

### 3.3 Vault PKI

Vault acts as the certificate authority for the platform.

It issues certificates used by:

- `auth.ai-platform.local`;
- `fraud-model.local`.

The certificate flow is:

```text
cert-manager
  ↓ Kubernetes authentication
Vault Kubernetes auth mount
  ↓
Vault PKI role
  ↓
cert-manager Issuer
  ↓
Certificate resource
  ↓
TLS Secret
  ↓
Envoy Gateway HTTPS listener
```

Vault is external to the Kubernetes cluster:

```text
https://vault.platform.local:8200
```

The certificates are trusted through the platform root CA:

```text
AI Platform ModelService Root CA
```

---

### 3.4 Envoy Gateway

Envoy Gateway is the external security boundary.

It performs:

- TLS termination;
- Server Name Indication selection;
- HTTP-to-HTTPS redirection;
- JWT extraction;
- JWT signature validation;
- issuer validation;
- audience validation;
- claim-based role authorization;
- HTTP-method authorization;
- request forwarding to the correct `HTTPRoute` backend.

The shared Gateway is:

```text
Namespace: gateway-system
Name:      shared-gateway
```

The Gateway exposes separate HTTPS listeners for:

```text
auth.ai-platform.local
fraud-model.local
```

Multiple HTTPS listeners share port `443`, while Envoy selects the correct listener and certificate through TLS SNI.

---

### 3.5 Gateway API HTTPRoutes

The ModelService operator generates the backend route:

```text
HTTPRoute: fraud-model
Namespace: ai-platform
Listener:  fraud-model-https
Hostname:  fraud-model.local
```

A separate route performs HTTP-to-HTTPS redirection:

```text
HTTPRoute: fraud-model-http-redirect
Listener:  http
Status:    301
```

The redirect route is intentionally not protected by JWT authentication. Its only responsibility is to redirect clients to HTTPS.

The workload route is HTTPS-only and is the target of the `SecurityPolicy`.

---

### 3.6 SecurityPolicy

Envoy Gateway uses a `SecurityPolicy` attached directly to the ModelService `HTTPRoute`.

```text
SecurityPolicy: fraud-model-jwt-authentication
Namespace:      ai-platform
Target:         HTTPRoute/fraud-model
```

The policy validates:

```text
Issuer:
https://auth.ai-platform.local/realms/ai-platform

Audience:
ai-platform-gateway

JWKS:
http://keycloak.keycloak.svc.cluster.local:8080/
realms/ai-platform/protocol/openid-connect/certs
```

The issuer and JWKS locations are intentionally different:

```text
Issuer
  External identity written into the token.

JWKS URI
  Internal cluster address used by Envoy to retrieve public signing keys.
```

The policy rejects missing, malformed, expired, incorrectly signed, incorrectly issued, or incorrectly scoped tokens.

---

## 4. Authentication Architecture

Authentication answers:

```text
Who is calling?
Is the presented token valid?
Was the token issued by the trusted realm?
Was the token intended for this resource server?
Has the token expired?
```

The complete authentication path is:

```text
Client
  ↓
Keycloak
  ↓
Signed JWT access token
  ↓
Envoy Gateway SecurityPolicy
  ↓
JWKS signature validation
  ↓
Issuer validation
  ↓
Audience validation
  ↓
Authenticated request
```

### 4.1 Human Authentication

Human users authenticate through:

```text
Authorization Code Flow + PKCE S256
```

Client:

```text
ai-platform-cli
```

Characteristics:

```text
publicClient=true
standardFlowEnabled=true
directAccessGrantsEnabled=false
PKCE=S256
```

The password grant is deliberately disabled.

The PKCE flow is:

```text
Browser
  ↓
Keycloak authorization endpoint
  ↓
User login
  ↓
Authorization code
  ↓
Loopback callback
  ↓
Code verifier exchange
  ↓
User access token
```

---

### 4.2 Machine Authentication

Machine clients authenticate through:

```text
OAuth 2.0 Client Credentials
```

Client:

```text
ai-platform-service
```

Service-account identity:

```text
service-account-ai-platform-service
```

Expected token claims include:

```text
azp = ai-platform-service
preferred_username = service-account-ai-platform-service
aud contains ai-platform-gateway
realm_access.roles contains model-deployer
```

---

## 5. Authorization Architecture

Authentication proves identity. Authorization determines what the identity may do.

Authorization is based on Keycloak realm roles stored in:

```text
realm_access.roles
```

The role hierarchy is:

```text
platform-admin
  └── model-deployer
        └── model-viewer
```

Composite roles ensure higher roles inherit lower permissions.

### 5.1 Role Meanings

```text
model-viewer
  Read-only model access.

model-deployer
  Read access plus deployment-changing requests.

platform-admin
  Full supported HTTP access to the protected model route.
```

### 5.2 HTTP Authorization Matrix

| Role | GET | HEAD | POST | PUT | PATCH | DELETE |
|---|---:|---:|---:|---:|---:|---:|
| `model-viewer` | Allow | Allow | Deny | Deny | Deny | Deny |
| `model-deployer` | Allow | Allow | Allow | Allow | Allow | Deny |
| `platform-admin` | Allow | Allow | Allow | Allow | Allow | Allow |

The policy uses:

```text
defaultAction: Deny
```

Any authenticated request not explicitly allowed is denied.

---

## 6. Identity Model

### 6.1 Realm Roles

```text
model-viewer
model-deployer
platform-admin
```

### 6.2 Human Users

```text
viewer-user
  → model-viewer

deployer-user
  → model-deployer

admin-user
  → platform-admin
```

### 6.3 Machine Identity

```text
service-account-ai-platform-service
  → model-deployer
```

### 6.4 Keycloak Administration Boundary

The `platform-admin` role is an AI Platform application role.

It is not a Keycloak administrator role.

It must not grant built-in `realm-management` privileges such as:

```text
realm-admin
manage-users
manage-clients
manage-realm
```

This separates application authorization from identity-provider administration.

---

## 7. Expected HTTP Status Behavior

The system intentionally distinguishes authentication failures, authorization failures, and backend behavior.

### 7.1 Authentication Failure

```text
401 Unauthorized
```

Examples:

- no access token;
- malformed token;
- expired token;
- invalid signature;
- wrong issuer;
- wrong audience.

### 7.2 Authorization Failure

```text
403 Forbidden
```

This means:

- the token is valid;
- the caller is authenticated;
- the caller's role or HTTP method is not permitted.

### 7.3 Backend Method Failure

```text
405 Method Not Allowed
```

This can be a successful authorization result.

For example, Envoy may allow `POST`, but the NGINX test backend may not implement it. In that case:

```text
Gateway authorization succeeded
Backend returned 405
```

### 7.4 Expected Test Matrix

```text
Missing token       GET     → 401
Invalid token       GET     → 401
model-viewer        GET     → 200
model-viewer        POST    → 403
model-viewer        DELETE  → 403
model-deployer      GET     → 200
model-deployer      POST    → backend response, commonly 405
model-deployer      DELETE  → 403
platform-admin      DELETE  → backend response, commonly 405
```

---

## 8. Transport Security Architecture

Bearer tokens must not be sent over plaintext HTTP.

The implemented transport path is:

```text
http://fraud-model.local
  ↓ 301 redirect
https://fraud-model.local
  ↓
Vault-issued certificate
  ↓
Envoy Gateway TLS termination
  ↓
Protected HTTPRoute
```

The served certificate must contain:

```text
CN  = fraud-model.local
SAN = DNS:fraud-model.local
```

The Keycloak certificate must contain:

```text
CN  = auth.ai-platform.local
SAN = DNS:auth.ai-platform.local
```

Validation is performed with the platform CA and without using insecure TLS options such as:

```text
-k
--insecure
```

---

## 9. Kubernetes Security Boundary

OIDC roles and Kubernetes RBAC solve different problems.

```text
Keycloak roles
  Control access through Envoy Gateway.

Kubernetes RBAC
  Controls what Pods and controllers may do through the Kubernetes API.
```

The human users and the service client do not receive Kubernetes credentials.

They are authorized at the gateway layer only.

---

## 10. ModelService Workload Security

The `ModelService` custom resource exposes the security setting:

```yaml
security:
  automountServiceAccountToken: false
```

The API field is represented in Go as:

```go
AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
```

The CRD defaults it to:

```text
false
```

The controller propagates the value to both:

```text
ServiceAccount.automountServiceAccountToken
Deployment.spec.template.spec.automountServiceAccountToken
```

The propagation chain is:

```text
ModelService spec
  ↓
Operator reconciliation
  ↓
Dedicated ServiceAccount
  ↓
Deployment Pod template
  ↓
No Kubernetes API token mounted
```

The workload does not need Kubernetes API access, so its Pods should not receive Kubernetes credentials.

---

## 11. Workload RBAC Expectations

The workload identity is:

```text
system:serviceaccount:ai-platform:fraud-model
```

It should not be able to:

```text
list Pods
list Services
read Secrets
read ConfigMaps
create Pods
create Deployments
read ModelService resources
request ServiceAccount tokens
```

The dedicated ServiceAccount exists for workload identity isolation, not for Kubernetes API access.

---

## 12. Operator RBAC Expectations

The operator legitimately requires Kubernetes API access because it reconciles child resources.

It is allowed to:

```text
get/list/watch/update/patch ModelService resources
update ModelService status
manage Deployments
manage Services
manage ServiceAccounts
manage PersistentVolumeClaims
manage PodDisruptionBudgets
manage NetworkPolicies
manage HTTPRoutes
create and patch Events
```

It should not be able to:

```text
create ModelService resources
delete ModelService resources
read Secrets
list Nodes
create ClusterRoles
create RoleBindings
request arbitrary ServiceAccount tokens
use pods/exec
use pods/portforward
```

The validated parent-resource permissions are:

```text
create ModelService: no
delete ModelService: no
update ModelService/status: yes
create ServiceAccount token: no
```

---

## 13. Operator Reconciliation Boundary

The operator restores resources that it owns, but it cannot restore itself or recreate the parent custom resource.

### 13.1 Resources Restored by the Operator

When the operator is running and the `ModelService` exists, it can recreate:

```text
Deployment
Service
HTTPRoute
PersistentVolumeClaim
PodDisruptionBudget
NetworkPolicy
ServiceAccount
```

### 13.2 Resources Not Restored by the Operator

The operator cannot automatically recreate:

```text
the operator Deployment itself
the operator namespace
the ModelService parent resource
the ai-platform namespace
a recreated kind cluster
```

### 13.3 Correct Mental Model

```text
Deleting a generated Deployment:
  operator restores it

Deleting the ModelService:
  operator does not recreate it

Deleting the operator:
  nothing reconciles

Recreating the kind cluster:
  all in-cluster state is gone unless reapplied
```

### 13.4 Recommended Two-Layer Reconciliation

```text
GitOps or bootstrap layer
  └── restores namespaces, operator and ModelService CRs

ModelService operator
  └── restores workload child resources
```

---

## 14. Network Security

The model workload uses a generated `NetworkPolicy`.

The intended traffic path is:

```text
Envoy Gateway data plane
  ↓
fraud-model Service
  ↓
fraud-model Pods
```

The workload should not be broadly reachable from unrelated namespaces or Pods.

Keycloak also uses NetworkPolicies to restrict:

- access to Keycloak port `8080`;
- access to the Keycloak management port;
- PostgreSQL access to Keycloak only.

The internal JWKS endpoint is reachable by Envoy because the data-plane Pods match the required source labels.

---

## 15. Source-of-Truth Files

The implementation is distributed across several areas of the repository.

### 15.1 Keycloak Platform

```text
config/platform/keycloak/
infrastructure/keycloak/
```

### 15.2 Gateway and Authentication

```text
config/platform/shared-gateway.yaml
config/platform/authentication/
```

### 15.3 ModelService API and Controller

```text
api/v1alpha1/modelservice_types.go
internal/controller/modelservice_controller.go
config/crd/bases/platform.anselem.dev_modelservices.yaml
config/rbac/role.yaml
config/samples/platform_v1alpha1_modelservice.yaml
```

### 15.4 Validation Scripts

```text
validate-keycloak-installation.sh
validate-keycloak-https.sh
configure-keycloak-realm-clients.sh
configure-keycloak-roles-users.sh
get-machine-token.sh
decode-jwt.sh
validate-machine-token.sh
validate-jwt-signature.py
pkce-login.py
validate-gateway-jwt-authentication.sh
validate-gateway-role-authorization.sh
validate-gateway-role-matrix.sh
validate-kubernetes-permissions.sh
validate-oidc-end-to-end.sh
```

---

## 16. Secret-Management Boundary

Real secret values are never committed to Git.

Local sensitive material includes:

```text
config/platform/keycloak/.secrets/
.local/keycloak/
*.jwt
token-response JSON files
client-secret files
test-user password files
private keys
```

Safe-to-commit content includes:

```text
Kubernetes manifests
Kustomizations
Vault policy definitions
configuration scripts
validation scripts
non-secret realm configuration
example environment files
CRD changes
RBAC changes
controller code
```

The implementation uses declarative secret definitions while keeping real values in Git-ignored local files.

---

## 17. End-to-End Security Flow

The complete successful machine request looks like this:

```text
1. ai-platform-service authenticates to Keycloak.
2. Keycloak issues a short-lived access token.
3. The token contains:
     iss = trusted ai-platform realm
     aud = ai-platform-gateway
     azp = ai-platform-service
     roles = model-deployer, model-viewer
4. The client connects to https://fraud-model.local.
5. Envoy serves the Vault-issued fraud-model.local certificate.
6. Envoy extracts the bearer token.
7. Envoy retrieves the Keycloak signing key through JWKS.
8. Envoy validates signature, issuer, audience and expiry.
9. Envoy reads realm_access.roles.
10. Envoy checks the HTTP method against the authorization policy.
11. The HTTPRoute forwards the request to Service/fraud-model.
12. The Service forwards the request to fraud-model Pods.
13. The workload serves the response without holding Kubernetes API credentials.
```

---

## 18. Security Guarantees Achieved

The completed design provides the following guarantees:

```text
[✓] Keycloak is exposed only through trusted HTTPS
[✓] JWTs are signed and validated through JWKS
[✓] Issuer validation is enforced
[✓] Audience validation is enforced
[✓] Human and machine identities are separated
[✓] Direct password grant is disabled
[✓] PKCE is used for public-client login
[✓] Missing and invalid tokens are rejected
[✓] Role-based HTTP authorization is enforced
[✓] Default authorization behavior is deny
[✓] HTTP redirects to HTTPS
[✓] Vault-issued certificates are used
[✓] Workload ServiceAccount token mounting is disabled
[✓] Workload Kubernetes API permissions are denied
[✓] Operator permissions are restricted
[✓] Sensitive token and password files are excluded from Git
[✓] Authentication and authorization are validated automatically
[✓] End-to-end request flow is reproducible
```

---

## 19. Current Limitations

The current backend is an NGINX test workload. It demonstrates the security path but does not expose a real model API.

Therefore:

```text
POST, PUT, PATCH or DELETE may return 405
```

This does not necessarily indicate an authorization failure. It usually means the request passed Envoy authorization and reached a backend that does not implement the method.

The current platform also does not expose a customer-facing AI Platform REST API. Authentication is applied directly at the ModelService gateway boundary.

The future platform architecture can evolve to:

```text
Client
  ↓
Keycloak
  ↓
Envoy Gateway
  ↓
AI Platform REST API
  ↓
Kubernetes API
  ↓
ModelService Operator
```

In that future design, the REST API becomes the controlled service interface, while the operator continues to manage Kubernetes lifecycle reconciliation.

---

## 20. Summary

The final system separates security responsibilities cleanly:

```text
Keycloak
  Authenticates users and machines and issues signed JWTs.

Vault
  Issues trusted TLS certificates.

cert-manager
  Automates certificate requests and renewal.

Envoy Gateway
  Terminates TLS and enforces JWT authentication and role authorization.

Gateway API
  Routes authenticated requests to the correct workload.

ModelService Operator
  Reconciles workload resources and applies secure defaults.

Kubernetes RBAC
  Restricts controller and workload API permissions.

NetworkPolicy
  Restricts network reachability.

Git
  Stores declarative configuration without real secret values.
```

The result is a reproducible, layered security architecture for exposing model-serving workloads through OIDC, JWT, HTTPS, and least-privilege Kubernetes controls.
