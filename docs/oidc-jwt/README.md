# OIDC, JWT, Keycloak, Vault PKI, and Envoy Gateway Documentation

## Purpose

This documentation set describes the complete implementation of authentication, authorization, HTTPS exposure, certificate management, Kubernetes security hardening, validation, Git safety, and recovery for the AI Platform `ModelService`.

It is written so that another engineer can:

- understand the final architecture;
- reproduce the implementation step by step;
- validate each layer independently;
- troubleshoot failures;
- rebuild the environment after a cluster reset;
- review the security boundaries and design decisions.

---

## Architecture

![img](/img/jwt-auth.png)

[//]: # (```text)

[//]: # (Client)

[//]: # (  ↓)

[//]: # (Keycloak authorization or token endpoint)

[//]: # (  ↓)

[//]: # (JWT access token)

[//]: # (  ↓)

[//]: # (Vault-issued HTTPS certificate)

[//]: # (  ↓)

[//]: # (Envoy Gateway)

[//]: # (  ↓)

[//]: # (JWT signature, issuer, audience, and expiry validation)

[//]: # (  ↓)

[//]: # (Role and HTTP-method authorization)

[//]: # (  ↓)

[//]: # (HTTPRoute)

[//]: # (  ↓)

[//]: # (Service)

[//]: # (  ↓)

[//]: # (fraud-model Pods)

[//]: # (```)

The implementation uses:

```text
Kubernetes
Kind
AI Platform ModelService Operator
Envoy Gateway
Gateway API
Keycloak
PostgreSQL
Vault PKI
cert-manager
MetalLB
NetworkPolicy
OIDC
OAuth 2.0
JWT
Authorization Code with PKCE
Client Credentials
Kubernetes RBAC
```

---

## Documentation Structure

| Order | Document | Purpose |
|---:|---|---|
| 00 | [`00-overview-and-architecture.md`](00-overview-and-architecture.md) | Explains the complete architecture, trust boundaries, request paths, and component responsibilities. |
| 01 | [`01-prerequisites-and-environment.md`](01-prerequisites-and-environment.md) | Defines the required cluster components, software, hostnames, namespaces, versions, and pre-flight checks. |
| 02 | [`02-keycloak-installation.md`](02-keycloak-installation.md) | Installs Keycloak, PostgreSQL, storage, Services, Secrets, NetworkPolicies, and supporting manifests. |
| 03 | [`03-vault-pki-and-keycloak-https.md`](03-vault-pki-and-keycloak-https.md) | Configures Vault PKI, cert-manager, Keycloak HTTPS, the Gateway listener, certificate issuance, and HTTP redirection. |
| 04 | [`04-keycloak-realm-and-clients.md`](04-keycloak-realm-and-clients.md) | Creates the `ai-platform` realm, resource-server client, public PKCE client, confidential service client, and audience mappers. |
| 05 | [`05-roles-users-and-service-accounts.md`](05-roles-users-and-service-accounts.md) | Creates realm roles, composite-role inheritance, human users, test credentials, and the machine service-account identity. |
| 06 | [`06-token-flows-and-jwt-validation.md`](06-token-flows-and-jwt-validation.md) | Documents machine and human token flows, JWT claims, decoding, JWKS retrieval, PKCE, and signature validation. |
| 07 | [`07-fraud-model-https-exposure.md`](07-fraud-model-https-exposure.md) | Restores the operator and `ModelService`, exposes `fraud-model.local` over HTTPS, validates SNI, and adds HTTP-to-HTTPS redirection. |
| 08 | [`08-envoy-jwt-authentication.md`](08-envoy-jwt-authentication.md) | Attaches Envoy Gateway JWT authentication to the fraud-model route and validates issuer, audience, signature, and expiry. |
| 09 | [`09-role-based-authorization.md`](09-role-based-authorization.md) | Adds role- and HTTP-method-based authorization and validates the complete `401`, `403`, and backend `405` behavior matrix. |
| 10 | [`10-kubernetes-security-hardening.md`](10-kubernetes-security-hardening.md) | Hardens ServiceAccount token mounting, Pod security, operator RBAC, workload permissions, CRD schema, and controller behavior. |
| 11 | [`11-end-to-end-validation.md`](11-end-to-end-validation.md) | Runs the complete validation path across Keycloak, TLS, Gateway, JWT, authorization, backend, and Kubernetes security. |
| 12 | [`12-git-safety-and-secret-management.md`](12-git-safety-and-secret-management.md) | Defines which files are safe to commit, which must remain local, and how to scan staged content for secrets. |
| 13 | [`13-recovery-and-troubleshooting.md`](13-recovery-and-troubleshooting.md) | Provides scenario-based diagnostics and recovery procedures for Keycloak, Vault, cert-manager, Envoy, JWT, routes, RBAC, and workloads. |
| 14 | [`14-complete-command-reference.md`](14-complete-command-reference.md) | Provides a condensed command-only operational reference for installation, validation, troubleshooting, and Git operations. |

---

## Recommended Reading and Reproduction Order

For a complete understanding and fresh environment, follow:

```text
01-prerequisites-and-environment.md
  ↓
02-keycloak-installation.md
  ↓
03-vault-pki-and-keycloak-https.md
  ↓
04-keycloak-realm-and-clients.md
  ↓
05-roles-users-and-service-accounts.md
  ↓
06-token-flows-and-jwt-validation.md
  ↓
07-fraud-model-https-exposure.md
  ↓
08-envoy-jwt-authentication.md
  ↓
09-role-based-authorization.md
  ↓
10-kubernetes-security-hardening.md
  ↓
11-end-to-end-validation.md
```

Then review:

```text
12-git-safety-and-secret-management.md
13-recovery-and-troubleshooting.md
14-complete-command-reference.md
```

---

## Installation Documents

The actual implementation and installation steps are mainly in:

```text
02-keycloak-installation.md
03-vault-pki-and-keycloak-https.md
04-keycloak-realm-and-clients.md
05-roles-users-and-service-accounts.md
06-token-flows-and-jwt-validation.md
07-fraud-model-https-exposure.md
08-envoy-jwt-authentication.md
09-role-based-authorization.md
10-kubernetes-security-hardening.md
```

The remaining documents provide architecture, prerequisites, validation, Git safety, recovery, and quick-reference commands.

---

## Environment Summary

```text
Repository:
  /mnt/data/ai-platform-operator

Kubernetes context:
  kind-ai-platform-policy

Kind cluster:
  ai-platform-policy

Kubernetes version:
  v1.36.1

Application namespace:
  ai-platform

Keycloak namespace:
  keycloak

Gateway namespace:
  gateway-system

Envoy data-plane namespace:
  envoy-gateway-system

Operator namespace:
  ai-platform-operator-system

Gateway:
  gateway-system/shared-gateway

Gateway address:
  172.19.255.200

Keycloak hostname:
  auth.ai-platform.local

Model hostname:
  fraud-model.local

OIDC realm:
  ai-platform

OIDC issuer:
  https://auth.ai-platform.local/realms/ai-platform

Expected JWT audience:
  ai-platform-gateway

Vault:
  https://vault.platform.local:8200

Vault Kubernetes auth mount:
  kubernetes-kind/

Vault ModelService PKI mount:
  pki_modelservice/
```

---

## Identity Model

### Human identities

```text
viewer-user
  └── model-viewer

deployer-user
  └── model-deployer
      └── model-viewer

admin-user
  └── platform-admin
      └── model-deployer
          └── model-viewer
```

### Machine identity

```text
service-account-ai-platform-service
  └── model-deployer
      └── model-viewer
```

---

## OIDC Clients

```text
ai-platform-gateway
  bearer-only resource server and expected audience

ai-platform-cli
  public client
  Authorization Code
  PKCE S256
  direct password grant disabled

ai-platform-service
  confidential client
  service account enabled
  Client Credentials grant
```

---

## Authorization Matrix

| Identity | GET | HEAD | POST | PUT | PATCH | DELETE |
|---|---:|---:|---:|---:|---:|---:|
| `model-viewer` | Allow | Allow | Deny | Deny | Deny | Deny |
| `model-deployer` | Allow | Allow | Allow | Allow | Allow | Deny |
| `platform-admin` | Allow | Allow | Allow | Allow | Allow | Allow |

Expected behavior:

```text
No token:
  401

Invalid or expired token:
  401

Valid token without sufficient role:
  403

Valid and authorized method unsupported by nginx:
  405

Valid and authorized GET:
  200
```

---

## Important Security Boundaries

### Keycloak

Keycloak authenticates identities and issues tokens.

It does not directly configure Kubernetes RBAC.

### Envoy Gateway

Envoy validates:

```text
JWT signature
issuer
audience
expiry
application roles
HTTP methods
```

### Kubernetes RBAC

Kubernetes controls what ServiceAccounts can do against the Kubernetes API.

### ModelService Operator

The operator manages child resources derived from the parent `ModelService`.

It does not create the parent CR itself and does not restore an entire deleted cluster.

### Vault and cert-manager

Vault is the certificate authority and source of issued certificates.

cert-manager requests and stores certificates as Kubernetes TLS Secrets.

---

## Secret Handling Rules

Never commit:

```text
config/platform/keycloak/.secrets/
.local/keycloak/
*.jwt
token response JSON files
client secrets
test-user passwords
bootstrap administrator passwords
PostgreSQL passwords
private keys
```

Safe to commit:

```text
manifests
scripts
CRD changes
controller code
RBAC
placeholder environment examples
documentation
validation automation
```

See:

```text
12-git-safety-and-secret-management.md
```

---

## Validation Entry Point

The final combined validation is:

```bash
cd /mnt/data/ai-platform-operator

infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

Expected ending:

```text
PASS: End-to-end OIDC/JWT request path validated.
```

For the complete validation sequence, see:

```text
11-end-to-end-validation.md
```

---

## Quick Troubleshooting Entry Point

Use:

```text
13-recovery-and-troubleshooting.md
```

Start by confirming:

```bash
kubectl config current-context
kind get clusters
kubectl get pods -A
kubectl get gateway,httproute -A
kubectl get securitypolicy -A
kubectl get certificate,issuer -A
```

Then identify whether the failure occurs at:

```text
Keycloak
certificate issuance
TLS
Gateway listener
HTTPRoute
JWT authentication
authorization
backend
operator reconciliation
Kubernetes RBAC
```

---

## Documentation Conventions

Each implementation document follows a consistent structure where applicable:

```text
Purpose
Architecture
Environment
Prerequisites
Files created or modified
Configuration
Commands
Expected results
Validation
Troubleshooting
Completion criteria
```

Commands are intended to be run from:

```text
/mnt/data/ai-platform-operator
```

unless stated otherwise.

---

## Completion Status

```text
[✓] Architecture documented
[✓] Prerequisites documented
[✓] Keycloak installation documented
[✓] Vault PKI and HTTPS documented
[✓] Realm and clients documented
[✓] Roles, users, and service account documented
[✓] Token flows and JWT validation documented
[✓] Fraud model HTTPS exposure documented
[✓] Envoy JWT authentication documented
[✓] Role-based authorization documented
[✓] Kubernetes security hardening documented
[✓] End-to-end validation documented
[✓] Git safety documented
[✓] Recovery and troubleshooting documented
[✓] Complete command reference documented
```

---

## Documentation Set

```text
docs/oidc-jwt/
├── README.md
├── 00-overview-and-architecture.md
├── 01-prerequisites-and-environment.md
├── 02-keycloak-installation.md
├── 03-vault-pki-and-keycloak-https.md
├── 04-keycloak-realm-and-clients.md
├── 05-roles-users-and-service-accounts.md
├── 06-token-flows-and-jwt-validation.md
├── 07-fraud-model-https-exposure.md
├── 08-envoy-jwt-authentication.md
├── 09-role-based-authorization.md
├── 10-kubernetes-security-hardening.md
├── 11-end-to-end-validation.md
├── 12-git-safety-and-secret-management.md
├── 13-recovery-and-troubleshooting.md
└── 14-complete-command-reference.md
```
