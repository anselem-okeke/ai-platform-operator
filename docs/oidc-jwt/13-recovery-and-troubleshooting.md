# Recovery and Troubleshooting

## Purpose

This document provides scenario-based recovery procedures for the OIDC, JWT, Keycloak, Vault PKI, Envoy Gateway, ModelService, and Kubernetes security implementation.

---

## Diagnostic Order

Use this order:

```text
1. Confirm current Kubernetes context
2. Confirm namespaces and controllers
3. Confirm Keycloak and PostgreSQL
4. Confirm certificates
5. Confirm Gateway and routes
6. Confirm SecurityPolicy
7. Confirm token claims and expiry
8. Confirm authorization roles
9. Confirm backend readiness
10. Confirm NetworkPolicies and RBAC
```

---

## Confirm the Active Cluster

```bash
kubectl config current-context
kind get clusters
kubectl cluster-info
```

Expected context:

```text
kind-ai-platform-policy
```

A wrong context can make every expected resource appear missing.

---

## Keycloak Pod Not Ready

Check:

```bash
kubectl get pods \
  -n keycloak \
  -o wide
```

```bash
kubectl describe pod \
  -n keycloak \
  -l app.kubernetes.io/name=keycloak
```

```bash
kubectl logs \
  -n keycloak \
  deployment/keycloak \
  --tail=200
```

Check PostgreSQL:

```bash
kubectl get statefulset,pod,service,pvc \
  -n keycloak
```

```bash
kubectl logs \
  -n keycloak \
  statefulset/keycloak-postgres \
  --tail=200
```

Common causes:

```text
database unavailable
wrong credentials
PVC not bound
invalid Keycloak environment variables
bootstrap administrator secret missing
```

---

## Keycloak HTTPS Connection Reset

Likely cause:

```text
no matching Gateway HTTPS listener
```

Check:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o yaml
```

Confirm:

```text
listener hostname:
  auth.ai-platform.local

protocol:
  HTTPS

port:
  443

certificateRef:
  auth-ai-platform-local-tls
```

---

## Certificate Not Ready

Check the Certificate:

```bash
kubectl get certificate \
  -A
```

```bash
kubectl describe certificate \
  -n gateway-system \
  auth-ai-platform-local
```

Check CertificateRequests:

```bash
kubectl get certificaterequest \
  -A
```

Check Issuer:

```bash
kubectl get issuer \
  -A
```

Check cert-manager logs:

```bash
kubectl logs \
  -n cert-manager \
  deployment/cert-manager \
  --tail=200
```

Common causes:

```text
Vault role mismatch
Vault auth role mismatch
wrong PKI mount
wrong policy path
invalid CA trust
TokenRequest permission missing
```

---

## OIDC Discovery Shows Wrong Hostname

Check:

```bash
curl \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/realms/ai-platform/.well-known/openid-configuration |
jq '.issuer'
```

Expected:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Check Keycloak:

```text
KC_HOSTNAME=https://auth.ai-platform.local
KC_HTTP_ENABLED=true
KC_PROXY_HEADERS=xforwarded
```

---

## `kcadm` Returns `401 Unauthorized`

Remove the stale config:

```bash
rm -f /tmp/ai-platform-roles-users-kcadm.config
```

Authenticate again inside the Keycloak Pod.

Possible causes:

```text
expired admin session
wrong bootstrap admin credentials
realm change invalidated session
wrong admin realm
```

---

## Token Endpoint Returns `invalid_client`

Check:

```text
client ID
client secret
client enabled state
serviceAccountsEnabled
```

Confirm Secret keys:

```bash
kubectl get secret ai-platform-service-client-credentials \
  -n keycloak \
  -o json |
jq -r '.data | keys[]'
```

---

## Valid JWT Returns `401`

Check token expiry first:

```bash
infrastructure/keycloak/scripts/validate-machine-token.sh
```

Request a fresh token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Then check:

```text
issuer
audience
signature
kid
JWKS reachability
Authorization header
```

Validate cryptographically:

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

## Valid JWT Returns `403`

Authentication succeeded.

Check:

```text
realm_access.roles
HTTP method
claim path
role spelling
defaultAction
composite role inheritance
```

Decode the token and inspect roles.

---

## Allowed Request Returns `405`

This normally means:

```text
Envoy authorized the request
nginx received the request
nginx does not implement the method
```

It is not an authentication or authorization failure.

---

## JWKS Retrieval Fails

Test from an Envoy-compatible Pod:

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
    --fail \
    http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
```

Check:

```text
Service DNS
port 8080
Keycloak health
NetworkPolicy labels
NetworkPolicy namespace selectors
```

---

## `fraud-model.local` Route Missing

Check:

```bash
kubectl get namespace ai-platform
kubectl get modelservice fraud-model -n ai-platform
kubectl get httproute fraud-model -n ai-platform
```

Check operator:

```bash
kubectl get pods \
  -n ai-platform-operator-system
```

Check logs:

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager \
  --tail=200
```

---

## Operator Does Not Restore Child Resources

Confirm the parent exists:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform
```

Confirm the operator has permissions:

```bash
kubectl auth can-i create deployments.apps \
  --as=system:serviceaccount:ai-platform-operator-system:ai-platform-operator-controller-manager \
  -n ai-platform
```

Check owner references and reconciliation errors.

---

## HTTPS Serves the Wrong Certificate

Use explicit SNI:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  </dev/null \
  2>/dev/null |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -ext subjectAltName
```

Check hostname-specific listeners.

---

## HTTP Returns `200` Instead of `301`

Check:

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform \
  -o yaml
```

Confirm:

```text
sectionName: http
RequestRedirect
scheme: https
statusCode: 301
```

---

## HTTPS Returns `503`

Check backend health:

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=fraud-model
```

```bash
kubectl get endpointslice \
  -n ai-platform \
  -l kubernetes.io/service-name=fraud-model
```

Check Service port and NetworkPolicy.

---

## Workload Still Has a Kubernetes Token

Check the ModelService:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.security.automountServiceAccountToken}{"\n"}'
```

Check the ServiceAccount:

```bash
kubectl get serviceaccount fraud-model \
  -n ai-platform \
  -o jsonpath='{.automountServiceAccountToken}{"\n"}'
```

Check the Pod template:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.automountServiceAccountToken}{"\n"}'
```

Restart the Deployment after changing the template.

---

## SecurityPolicy Not Accepted

```bash
kubectl describe securitypolicy fraud-model-jwt-authentication \
  -n ai-platform
```

Check:

```text
targetRefs
namespace
HTTPRoute existence
installed CRD version
unsupported fields
JWKS provider configuration
```

---

## Full Cluster Rebuild Recovery

Recommended order:

```text
1. Recreate kind cluster
2. Install Gateway API CRDs
3. Install Envoy Gateway
4. Install MetalLB
5. Install cert-manager
6. Restore Vault Kubernetes auth integration
7. Restore shared Gateway
8. Restore Keycloak namespace and PostgreSQL
9. Restore Keycloak HTTPS certificate and routes
10. Restore realm, clients, roles, and users
11. Install ModelService CRD
12. Deploy operator
13. Apply ModelService/fraud-model
14. Apply fraud-model TLS resources
15. Apply redirect and SecurityPolicy
16. Run all validation scripts
```

---

## Evidence Collection

```bash
kubectl get all \
  -n keycloak \
  -o wide
```

```bash
kubectl get gateway,httproute \
  -A \
  -o yaml
```

```bash
kubectl get securitypolicy \
  -A \
  -o yaml
```

```bash
kubectl get certificate,certificaterequest,issuer \
  -A \
  -o yaml
```

```bash
kubectl get events \
  -A \
  --sort-by='.lastTimestamp'
```

Do not include raw secrets, tokens, passwords, or private keys in shared logs.

---

## Recovery Validation

After recovery:

```bash
infrastructure/keycloak/scripts/validate-keycloak-installation.sh
infrastructure/keycloak/scripts/validate-keycloak-https.sh
infrastructure/keycloak/scripts/get-machine-token.sh
infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

---

## Completion Criteria

```text
[✓] recovery order documented
[✓] Keycloak failures documented
[✓] Vault and certificate failures documented
[✓] Gateway listener failures documented
[✓] route failures documented
[✓] JWT 401 causes documented
[✓] authorization 403 causes documented
[✓] backend 405 and 503 explained
[✓] operator recovery documented
[✓] Kubernetes token-mount recovery documented
[✓] evidence collection documented
```
