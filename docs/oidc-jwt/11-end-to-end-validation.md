# End-to-End Validation

## Purpose

This document validates the complete OIDC, JWT, TLS, Gateway, authorization, workload, and Kubernetes security path.

The test proves:

```text
Keycloak is healthy
PostgreSQL is healthy
Vault-issued certificates are trusted
Gateway listeners are programmed
HTTP redirects to HTTPS
JWT authentication is active
role authorization is active
the backend is reachable
Kubernetes tokens are not mounted
operator permissions are restricted
```

---

## End-to-End Path

```text
Client
  ↓
Keycloak token endpoint
  ↓
JWT access token
  ↓
fraud-model.local over HTTPS
  ↓
Vault-issued certificate
  ↓
Envoy Gateway
  ↓
JWT validation
  ↓
role and method authorization
  ↓
HTTPRoute/fraud-model
  ↓
Service/fraud-model
  ↓
fraud-model Pods
```

---

## Pre-Validation Checks

```bash
cd /mnt/data/ai-platform-operator
```

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

Check all critical namespaces:

```bash
kubectl get namespace \
  keycloak \
  gateway-system \
  envoy-gateway-system \
  ai-platform \
  ai-platform-operator-system
```

---

## Validate Keycloak and PostgreSQL

```bash
kubectl rollout status \
  statefulset/keycloak-postgres \
  -n keycloak \
  --timeout=180s
```

```bash
kubectl rollout status \
  deployment/keycloak \
  -n keycloak \
  --timeout=180s
```

---

## Validate the Operator

```bash
kubectl rollout status \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system \
  --timeout=180s
```

---

## Validate the Model

```bash
kubectl get modelservice fraud-model \
  -n ai-platform
```

```bash
kubectl rollout status \
  deployment/fraud-model \
  -n ai-platform \
  --timeout=180s
```

Check ready replicas:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.status.readyReplicas}{"/"}{.spec.replicas}{"\n"}'
```

Expected:

```text
2/2
```

---

## Validate the Gateway

```bash
kubectl wait \
  --for=condition=Programmed \
  gateway/shared-gateway \
  -n gateway-system \
  --timeout=180s
```

List listeners:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.listeners[*]}{.name}{" attachedRoutes="}{.attachedRoutes}{"\n"}{end}'
```

Expected to include:

```text
http
keycloak-https
fraud-model-https
```

---

## Validate Routes

```bash
kubectl get httproute \
  -n keycloak
```

```bash
kubectl get httproute \
  -n ai-platform
```

Expected:

```text
fraud-model
fraud-model-http-redirect
```

Check route conditions:

```bash
for route_name in \
  fraud-model \
  fraud-model-http-redirect
do
  echo "Route: ${route_name}"

  kubectl get httproute "${route_name}" \
    -n ai-platform \
    -o jsonpath='{range .status.parents[*].conditions[*]}{.type}{"="}{.status}{" reason="}{.reason}{"\n"}{end}'
done
```

Expected:

```text
Accepted=True
ResolvedRefs=True
```

---

## Validate Certificates

### Keycloak certificate

```bash
openssl x509 \
  -in .local/keycloak/auth-ai-platform-root-ca.crt \
  -noout \
  -subject \
  -issuer \
  -fingerprint \
  -sha256
```

### Fraud model certificate chain

```bash
openssl s_client \
  -connect 172.19.255.200:443 \
  -servername fraud-model.local \
  -CAfile .local/keycloak/fraud-model-root-ca.crt \
  -verify_return_error \
  </dev/null
```

Expected:

```text
Verify return code: 0 (ok)
```

---

## Validate HTTP Redirect

Resolve the Gateway address:

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --write-out '%{http_code}\n' \
  --resolve "fraud-model.local:80:${GATEWAY_IP}" \
  http://fraud-model.local/
```

Expected:

```text
301
```

---

## Validate Authentication

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

Expected:

```text
401
```

### Fresh machine token

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

```bash
infrastructure/keycloak/scripts/validate-machine-token.sh
```

```bash
.local/keycloak/venv/bin/python \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  --token-file .local/keycloak/tokens/service-access-token.jwt \
  --jwks-url https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  --issuer https://auth.ai-platform.local/realms/ai-platform \
  --audience ai-platform-gateway \
  --ca-file .local/keycloak/auth-ai-platform-root-ca.crt
```

---

## Validate Authorization

```bash
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
```

Expected matrix:

```text
No token             GET      401
Invalid token        GET      401
Viewer               GET      200
Viewer               POST     403
Viewer               DELETE   403
Deployer             GET      200
Deployer             POST     405
Deployer             DELETE   403
Admin                GET      200
Admin                POST     405
Admin                DELETE   405
```

---

## Validate Kubernetes Security

```bash
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
```

Expected:

```text
ServiceAccount automount=false
Pod automount=false
no workload token mounted
workload cannot access Kubernetes API
operator cannot create/delete ModelServices
operator can update ModelService status
operator cannot read Secrets
operator cannot request ServiceAccount tokens
```

---

## Combined Validation Script

Repository file:

```text
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

The script validates:

```text
Keycloak rollout
PostgreSQL rollout
fraud-model rollout
Gateway Programmed condition
SecurityPolicy Accepted condition
ServiceAccount automount=false
PodSpec automount=false
HTTP 301 redirect
trusted HTTPS certificate
missing token returns 401
invalid token returns 401
fresh machine token GET succeeds
```

Run:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

Expected ending:

```text
PASS: End-to-end OIDC/JWT request path validated.
```

---

## Full Validation Order

```bash
infrastructure/keycloak/scripts/validate-keycloak-installation.sh
```

```bash
infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

```bash
infrastructure/keycloak/scripts/validate-machine-token.sh
```

```bash
.local/keycloak/venv/bin/python \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  --token-file .local/keycloak/tokens/service-access-token.jwt \
  --jwks-url https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  --issuer https://auth.ai-platform.local/realms/ai-platform \
  --audience ai-platform-gateway \
  --ca-file .local/keycloak/auth-ai-platform-root-ca.crt
```

```bash
infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh
```

```bash
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
```

```bash
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
```

```bash
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

---

## Interpreting Failures

```text
TLS failure:
  certificate, CA, SNI, or listener problem

401:
  token missing, invalid, expired, wrong issuer, wrong audience, or JWKS failure

403:
  token valid but role or method denied

404:
  route or hostname mismatch

405:
  Gateway allowed the method, backend does not implement it

503:
  backend Service or Pod readiness problem
```

---

## Evidence to Capture

For audit or troubleshooting, capture:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o yaml \
  > /tmp/shared-gateway.yaml
```

```bash
kubectl get httproute \
  -A \
  -o yaml \
  > /tmp/httproutes.yaml
```

```bash
kubectl get securitypolicy \
  -A \
  -o yaml \
  > /tmp/securitypolicies.yaml
```

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o yaml \
  > /tmp/fraud-model-modelservice.yaml
```

Do not capture raw JWTs, client secrets, passwords, or private keys in shared evidence.

---

## Completion Criteria

```text
[✓] Keycloak healthy
[✓] PostgreSQL healthy
[✓] operator healthy
[✓] ModelService reconciled
[✓] fraud-model Pods ready
[✓] Gateway Programmed
[✓] routes Accepted
[✓] certificates trusted
[✓] HTTP redirects to HTTPS
[✓] no token returns 401
[✓] invalid token returns 401
[✓] valid token accepted
[✓] role matrix passes
[✓] workload token not mounted
[✓] operator RBAC restricted
[✓] combined validation script passes
```
