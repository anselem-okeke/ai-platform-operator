# Envoy JWT Authentication

## Purpose

This document explains how Envoy Gateway validates JSON Web Tokens before requests reach the `fraud-model` backend.

It covers:

- the `SecurityPolicy` resource;
- issuer and audience validation;
- internal JWKS retrieval;
- attachment to `HTTPRoute/fraud-model`;
- authentication outcomes;
- validation commands and scripts;
- troubleshooting.

---

## Final Request Path

```text
Client
  ↓ HTTPS
fraud-model.local
  ↓
Envoy Gateway
  ↓
SecurityPolicy/fraud-model-jwt-authentication
  ├── signature validation
  ├── issuer validation
  ├── audience validation
  └── expiry validation
  ↓
HTTPRoute/fraud-model
  ↓
Service/fraud-model
  ↓
fraud-model Pods
```

---

## Environment

```text
Application namespace:
  ai-platform

Gateway namespace:
  gateway-system

Gateway:
  shared-gateway

Protected route:
  ai-platform/fraud-model

SecurityPolicy:
  ai-platform/fraud-model-jwt-authentication

Issuer:
  https://auth.ai-platform.local/realms/ai-platform

Audience:
  ai-platform-gateway

JWKS endpoint:
  http://keycloak.keycloak.svc.cluster.local:8080/
  realms/ai-platform/protocol/openid-connect/certs
```

---

## Why the Issuer and JWKS URLs Differ

The token issuer is the externally visible identity of the Keycloak realm:

```text
https://auth.ai-platform.local/realms/ai-platform
```

The JWKS URI is the internal Kubernetes Service endpoint Envoy uses to retrieve public signing keys:

```text
http://keycloak.keycloak.svc.cluster.local:8080/
realms/ai-platform/protocol/openid-connect/certs
```

The issuer claim must match the public issuer exactly.

The JWKS URI may use the internal cluster address because Envoy only needs the public signing keys.

---

## SecurityPolicy Manifest

Repository file:

```text
config/platform/authentication/fraud-model-jwt-securitypolicy.yaml
```

```yaml
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: SecurityPolicy
metadata:
  name: fraud-model-jwt-authentication
  namespace: ai-platform
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
          uri: >-
            http://keycloak.keycloak.svc.cluster.local:8080/realms/
            ai-platform/protocol/openid-connect/certs
```

Keep the actual `uri` on one line in the repository manifest.

---

## Kustomization

Repository file:

```text
config/platform/authentication/kustomization.yaml
```

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - fraud-model-http-redirect.yaml
  - fraud-model-jwt-securitypolicy.yaml
```

Apply:

```bash
kubectl apply \
  -k config/platform/authentication
```

---

## Validate the Installed CRD

```bash
kubectl get crd securitypolicies.gateway.envoyproxy.io
```

Inspect the supported schema:

```bash
kubectl explain securitypolicy.spec.jwt
```

```bash
kubectl explain securitypolicy.spec.jwt.providers
```

```bash
kubectl explain securitypolicy.spec.jwt.providers.remoteJWKS
```

This confirms that the installed Envoy Gateway version supports the fields used by the manifest.

---

## Confirm the Target Route Exists

```bash
kubectl get httproute fraud-model \
  -n ai-platform
```

Inspect:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

The policy and route must be in the same namespace.

---

## Apply the Policy

```bash
kubectl apply \
  -f config/platform/authentication/fraud-model-jwt-securitypolicy.yaml
```

Verify:

```bash
kubectl get securitypolicy \
  -n ai-platform
```

---

## Validate Policy Status

```bash
kubectl get securitypolicy fraud-model-jwt-authentication \
  -n ai-platform \
  -o yaml
```

Compact condition check:

```bash
kubectl get securitypolicy fraud-model-jwt-authentication \
  -n ai-platform \
  -o json |
jq '
  .status.ancestors[]?.conditions[]? |
  {
    type,
    status,
    reason,
    message
  }
'
```

Expected:

```text
Accepted=True
```

The exact status structure may vary slightly by Envoy Gateway version.

---

## Test JWKS Reachability Inside the Cluster

Create a temporary Pod with labels compatible with the Keycloak NetworkPolicy:

```bash
kubectl run jwks-check \
  -n envoy-gateway-system \
  --image=curlimages/curl:8.14.1 \
  --restart=Never \
  --labels='gateway.envoyproxy.io/owning-gateway-name=shared-gateway,gateway.envoyproxy.io/owning-gateway-namespace=gateway-system' \
  --rm \
  -i \
  -- \
  curl \
    --silent \
    --show-error \
    --fail \
    http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
```

Expected: a JWKS document containing one or more public keys.

---

## Authentication Outcomes

### No token

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  https://fraud-model.local/
```

Expected:

```text
401
```

### Invalid token

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  --header 'Authorization: Bearer invalid-token' \
  https://fraud-model.local/
```

Expected:

```text
401
```

### Valid machine token

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

```bash
ACCESS_TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
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
  --header "Authorization: Bearer ${ACCESS_TOKEN}" \
  https://fraud-model.local/
```

Expected before authorization rules are added:

```text
200
```

---

## Validation Script

Repository file:

```text
infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh
```

The script should validate:

```text
Gateway address exists
SecurityPolicy is accepted
HTTP redirects to HTTPS
HTTPS certificate is trusted
missing token returns 401
invalid token returns 401
fresh valid token reaches the backend
```

Run:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh
```

---

## Token Expiry

Access tokens expire after approximately:

```text
300 seconds
```

A previously valid token can later return:

```text
401 Unauthorized
```

Always request a fresh machine token immediately before validation:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

---

## NetworkPolicy Considerations

Keycloak is protected by NetworkPolicy.

Envoy must be allowed to reach:

```text
keycloak.keycloak.svc.cluster.local:8080
```

The temporary JWKS test Pod also needed labels matching the Envoy data plane.

When JWKS retrieval is blocked, Envoy cannot verify token signatures.

---

## Troubleshooting

### `SecurityPolicy` is not accepted

Check:

```bash
kubectl describe securitypolicy fraud-model-jwt-authentication \
  -n ai-platform
```

Common causes:

```text
wrong target route name
wrong namespace
unsupported CRD fields
invalid provider configuration
route does not exist
```

### Every request returns `401`

Check:

```text
token expiry
issuer
audience
JWKS reachability
token signature
Authorization header format
```

The header must be:

```text
Authorization: Bearer <JWT>
```

### JWKS retrieval fails

Test from a correctly labeled Pod.

Check:

```bash
kubectl get networkpolicy \
  -n keycloak
```

Check Keycloak Service:

```bash
kubectl get service keycloak \
  -n keycloak
```

### Valid token has wrong audience

The access token must contain:

```text
ai-platform-gateway
```

Check the audience mapper on:

```text
ai-platform-cli
ai-platform-service
```

### Valid token has wrong issuer

The token must contain exactly:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Check:

```text
KC_HOSTNAME
realm name
issuer configured in SecurityPolicy
```

---

## Files Created or Modified

```text
config/platform/authentication/fraud-model-jwt-securitypolicy.yaml
config/platform/authentication/kustomization.yaml
infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh
```

---

## Completion Criteria

```text
[✓] SecurityPolicy CRD available
[✓] fraud-model route exists
[✓] SecurityPolicy targets HTTPRoute/fraud-model
[✓] issuer configured correctly
[✓] audience configured correctly
[✓] internal JWKS endpoint configured
[✓] JWKS reachable from Envoy data plane
[✓] SecurityPolicy accepted
[✓] missing token returns 401
[✓] invalid token returns 401
[✓] expired token returns 401
[✓] valid token reaches the route
[✓] validation script passes
```
