# Role-Based Authorization

## Purpose

This document explains how valid Keycloak tokens are authorized according to AI Platform roles and HTTP methods.

Authentication answers:

```text
Is this token valid?
```

Authorization answers:

```text
Is this identity allowed to perform this operation?
```

---

## Role Hierarchy

```text
platform-admin
  └── model-deployer
        └── model-viewer
```

Effective roles:

```text
model-viewer:
  model-viewer

model-deployer:
  model-deployer
  model-viewer

platform-admin:
  platform-admin
  model-deployer
  model-viewer
```

The claim used by Envoy is:

```text
realm_access.roles
```

---

## Authorization Matrix

| Role | GET | HEAD | POST | PUT | PATCH | DELETE |
|---|---:|---:|---:|---:|---:|---:|
| `model-viewer` | Allow | Allow | Deny | Deny | Deny | Deny |
| `model-deployer` | Allow | Allow | Allow | Allow | Allow | Deny |
| `platform-admin` | Allow | Allow | Allow | Allow | Allow | Allow |

The policy uses a deny-by-default model.

---

## Expected Status Codes

```text
401 Unauthorized
  token is missing, malformed, expired, or invalid

403 Forbidden
  token is valid, but the role is not authorized

405 Method Not Allowed
  Envoy authorized the request, but nginx does not implement the method

200 OK
  request was authorized and processed successfully
```

---

## SecurityPolicy Authorization Rules

The authorization configuration is attached to the same policy that performs JWT authentication.

Conceptual structure:

```yaml
spec:
  targetRefs:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      name: fraud-model

  jwt:
    providers:
      - name: ai-platform-keycloak
        issuer: https://auth.ai-platform.local/realms/ai-platform
        audiences:
          - ai-platform-gateway
        remoteJWKS:
          uri: http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs

  authorization:
    defaultAction: Deny
    rules:
      - name: viewer-read
        action: Allow
        principal:
          jwt:
            provider: ai-platform-keycloak
            claims:
              - name: realm_access.roles
                valueType: StringArray
                values:
                  - model-viewer
        operation:
          methods:
            - GET
            - HEAD

      - name: deployer-write
        action: Allow
        principal:
          jwt:
            provider: ai-platform-keycloak
            claims:
              - name: realm_access.roles
                valueType: StringArray
                values:
                  - model-deployer
        operation:
          methods:
            - GET
            - HEAD
            - POST
            - PUT
            - PATCH

      - name: admin-all
        action: Allow
        principal:
          jwt:
            provider: ai-platform-keycloak
            claims:
              - name: realm_access.roles
                valueType: StringArray
                values:
                  - platform-admin
```

Use the exact schema supported by the installed Envoy Gateway CRD.

---

## Apply the Policy

```bash
kubectl apply \
  -f config/platform/authentication/fraud-model-jwt-securitypolicy.yaml
```

Check status:

```bash
kubectl get securitypolicy fraud-model-jwt-authentication \
  -n ai-platform \
  -o yaml
```

---

## Prepare Tokens

### Machine token

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Machine identity:

```text
service-account-ai-platform-service
```

Effective role:

```text
model-deployer
```

### Human tokens

Use the PKCE flow for:

```text
viewer-user
deployer-user
admin-user
```

Save tokens as:

```text
.local/keycloak/tokens/viewer-access-token.jwt
.local/keycloak/tokens/deployer-access-token.jwt
.local/keycloak/tokens/admin-access-token.jwt
```

---

## Manual Matrix Validation

Resolve the Gateway address:

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

### Viewer GET

```bash
VIEWER_TOKEN="$(
  cat .local/keycloak/tokens/viewer-access-token.jwt
)"
```

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  --header "Authorization: Bearer ${VIEWER_TOKEN}" \
  https://fraud-model.local/
```

Expected:

```text
200
```

### Viewer POST

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  --request POST \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  --header "Authorization: Bearer ${VIEWER_TOKEN}" \
  https://fraud-model.local/
```

Expected:

```text
403
```

### Deployer POST

```bash
DEPLOYER_TOKEN="$(
  cat .local/keycloak/tokens/deployer-access-token.jwt
)"
```

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  --request POST \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  --header "Authorization: Bearer ${DEPLOYER_TOKEN}" \
  https://fraud-model.local/
```

Expected:

```text
405
```

The `405` proves Envoy allowed the request and nginx rejected the method.

### Deployer DELETE

Expected:

```text
403
```

### Admin DELETE

Expected:

```text
405
```

Again, `405` proves the Gateway authorization succeeded.

---

## Validated Matrix

```text
No token             GET      401
Invalid token        GET      401
Viewer token         GET      200
Viewer token         POST     403
Viewer token         DELETE   403
Deployer token       GET      200
Deployer token       POST     405
Deployer token       DELETE   403
Admin token          GET      200
Admin token          POST     405
Admin token          DELETE   405
```

---

## Automated Validation

Repository scripts:

```text
infrastructure/keycloak/scripts/validate-gateway-role-authorization.sh
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
```

Run:

```bash
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
```

When the service token is used as the deployer identity:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-gateway-role-authorization.sh
```

---

## Why Composite Roles Matter

The service account receives:

```text
model-deployer
```

Because `model-deployer` includes `model-viewer`, the token also contains:

```text
model-viewer
```

This means the same token can satisfy both:

```text
viewer-read rule
deployer-write rule
```

The `platform-admin` token contains all three roles and can satisfy every allow rule.

---

## Security Boundary

The role:

```text
platform-admin
```

is an AI Platform application role.

It does not grant:

```text
Keycloak realm administration
Kubernetes cluster administration
Vault administration
Gateway administration
```

Authorization remains scoped to the protected application route.

---

## Troubleshooting

### Valid token returns `403`

Check:

```text
realm_access.roles
HTTP method
claim path
role spelling
policy default action
composite-role expansion
```

Decode the token:

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/viewer-access-token.jwt
```

### Expected `403`, but received `401`

The token failed authentication before authorization.

Check:

```text
expiry
issuer
audience
signature
Authorization header
```

### Expected `200`, but received `405`

The request passed authorization but the backend does not support the method.

This is expected for nginx on methods such as:

```text
POST
PUT
PATCH
DELETE
```

### Role exists in Keycloak but not in token

Check:

```text
direct assignment
composite hierarchy
client scopes
role mapper
token freshness
```

Request a fresh token after role changes.

---

## Files Created or Modified

```text
config/platform/authentication/fraud-model-jwt-securitypolicy.yaml
infrastructure/keycloak/scripts/validate-gateway-role-authorization.sh
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
```

---

## Completion Criteria

```text
[✓] authorization uses realm_access.roles
[✓] default action is Deny
[✓] viewer can read
[✓] viewer cannot modify
[✓] deployer can read and modify
[✓] deployer cannot delete
[✓] admin can reach all methods
[✓] 401 and 403 behavior distinguished
[✓] backend 405 behavior understood
[✓] composite role inheritance validated
[✓] full role matrix passes
```
