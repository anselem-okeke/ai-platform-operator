# AI Platform REST API — Envoy Gateway and HTTPS

## 1. Purpose

This document describes how the AI Platform REST API is exposed externally through Envoy Gateway and HTTPS.

It records:

- shared Gateway architecture
- API hostname
- HTTPRoute resources
- HTTP-to-HTTPS redirect
- TLS certificate
- Vault PKI integration
- Envoy SecurityPolicy
- JWT/role enforcement
- edge/application separation
- network path
- validation and troubleshooting

---

## 2. External API Hostname

The API is exposed as:

```text
https://api.ai-platform.local
```

This is the stable external platform endpoint for REST API clients.

---

## 3. Shared Gateway

The API uses the shared Gateway:

```text
gateway-system/shared-gateway
```

The shared Gateway is not dedicated solely to the API.

It also supports other platform routes.

---

## 4. External Address

The Envoy Gateway LoadBalancer address used in the lab is:

```text
172.19.255.200
```

This address is part of the local kind/lab environment.

The stable user-facing name is:

```text
api.ai-platform.local
```

rather than the raw IP.

---

## 5. Envoy Service

The generated Envoy service is:

```text
envoy-gateway-system-shared-gateway-0457b32d
```

Namespace:

```text
envoy-gateway-system
```

HTTPS externally:

```text
443
```

Envoy target port observed:

```text
10443
```

---

## 6. Gateway Flow

```text
Client
  |
  | HTTPS :443
  v
Envoy LoadBalancer
  |
  v
shared-gateway
  |
  v
HTTPRoute
  |
  v
SecurityPolicy
  |
  v
ai-platform-api Service
  |
  | HTTP :8080
  v
API pod
```

TLS terminates at Envoy.

---

## 7. API Service Remains Internal HTTP

The Go API itself listens internally on:

```text
8080
```

The application does not terminate external TLS.

This is intentional.

Responsibilities:

```text
Envoy
  -> TLS termination
  -> edge JWT policy
  -> routing

Go API
  -> business API
  -> application authz
  -> Kubernetes integration
```

---

## 8. HTTPRoute

The API has an HTTPS route stored under:

```text
config/platform-api/
```

The route maps:

```text
api.ai-platform.local
```

to:

```text
Service/ai-platform-api
```

in:

```text
ai-platform
```

---

## 9. HTTP Redirect Route

A separate HTTPRoute handles:

```text
HTTP -> HTTPS
```

redirection.

This prevents the platform API from being intentionally consumed over clear-text external HTTP.

---

## 10. TLS Certificate

Certificate resource:

```text
api-ai-platform-local
```

TLS secret:

```text
api-ai-platform-local-tls
```

Namespace:

```text
gateway-system
```

The Gateway references this secret for the API HTTPS listener.

---

## 11. Vault PKI

The API certificate is issued through the platform Vault PKI integration.

Vault endpoint:

```text
https://vault.platform.local:8200
```

The PKI trust ultimately uses:

```text
AI Platform ModelService Root CA
```

The same platform trust is used for local CLI validation.

---

## 12. Local CA Validation

Client curl validation uses:

```text
.local/keycloak/fraud-model-root-ca.crt
```

Example:

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  https://api.ai-platform.local/healthz
```

External SecurityPolicy may require JWT before the application receives that request.

---

## 13. SecurityPolicy

The API route is protected by an Envoy:

```text
SecurityPolicy
```

The SecurityPolicy enforces:

```text
JWT validation
role authorization
```

at the edge.

---

## 14. JWT Issuer

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

Expected audience:

```text
ai-platform-gateway
```

The Envoy policy and Go API security model are aligned around the same Keycloak trust model.

---

## 15. Edge and Application Authorization

Security is deliberately duplicated across layers.

```text
Envoy
   |
   | JWT + role
   v
Go API
   |
   | JWT + role
   v
handler
```

This provides defense in depth.

---

## 16. Why Edge Authorization Matters

Rejecting unauthorized traffic at Envoy:

```text
reduces unnecessary app traffic
centralizes external policy
protects application before handler execution
```

But the application still validates security independently.

---

## 17. Why Application Authorization Still Matters

The application should not assume:

```text
anything reaching me is trusted
```

Application authorization protects against:

```text
edge misconfiguration
internal access paths
future routing changes
policy drift
```

---

## 18. Edge-Denied DELETE

A non-admin machine/deployer token attempted DELETE.

Observed:

```text
403
```

The request was rejected by Envoy.

No Go mutation audit event was emitted because the application never received the request.

---

## 19. Admin DELETE

An admin PKCE token passed edge authorization and reached the API.

The request returned:

```text
204
```

and the Go API emitted the expected audit event.

This validates both enforcement layers.

---

## 20. External Authentication Example

Obtain token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Load:

```bash
TOKEN="$(
  cat .local/keycloak/tokens/service-access-token.jwt
)"
```

Call:

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  -sS \
  -H "Authorization: Bearer ${TOKEN}" \
  https://api.ai-platform.local/api/v1/model-services
```

---

## 21. Gateway vs Internal Health

The API can be internally healthy while external Gateway access is broken.

Therefore distinguish:

```text
pod health
Service reachability
Gateway route
TLS
SecurityPolicy
DNS
```

---

## 22. External Failure Categories

If HTTPS fails, check:

```text
hostname resolution
Gateway status
HTTPRoute status
TLS certificate
TLS secret
Envoy service/endpoints
SecurityPolicy
token validity
```

---

## 23. Gateway Inspection

Inspect Gateway:

```bash
kubectl get gateway \
  shared-gateway \
  -n gateway-system \
  -o yaml
```

Check listener readiness and attached routes.

---

## 24. HTTPRoute Inspection

List API routes:

```bash
kubectl get httproute \
  -A
```

Inspect the API route:

```bash
kubectl get httproute \
  ai-platform-api \
  -n ai-platform \
  -o yaml
```

---

## 25. Redirect Route Inspection

Inspect:

```bash
kubectl get httproute \
  ai-platform-api-http-redirect \
  -n ai-platform \
  -o yaml
```

Verify it attaches to the expected Gateway/listener.

---

## 26. SecurityPolicy Inspection

Inspect:

```bash
kubectl get securitypolicy \
  ai-platform-api-jwt-authorization \
  -n ai-platform \
  -o yaml
```

Check:

```text
target reference
JWT configuration
authorization rules
conditions/status
```

---

## 27. Certificate Inspection

```bash
kubectl get certificate \
  api-ai-platform-local \
  -n gateway-system \
  -o yaml
```

Verify certificate readiness.

---

## 28. TLS Secret Inspection

```bash
kubectl get secret \
  api-ai-platform-local-tls \
  -n gateway-system
```

Do not print private key content into documentation or logs.

---

## 29. Envoy Pod Selectors

Relevant proxy labels:

```text
app.kubernetes.io/component=proxy
app.kubernetes.io/name=envoy
gateway.envoyproxy.io/owning-gateway-name=shared-gateway
gateway.envoyproxy.io/owning-gateway-namespace=gateway-system
```

These became important for NetworkPolicy egress.

---

## 30. Network Path Lesson

The API OIDC startup path exposed an important networking detail.

A policy written only against:

```text
LoadBalancer IP
```

did not match the traffic as observed by the CNI after DNAT.

The correct policy targeted Envoy pods directly through namespace/pod selectors and port:

```text
10443
```

---

## 31. HTTPS Validation

A successful external read proves:

```text
DNS/hosts
TLS
certificate trust
Gateway
HTTPRoute
SecurityPolicy
JWT
API
Kubernetes read
```

all work together.

---

## 32. External Health Endpoint Nuance

At the Go application layer:

```text
/healthz
/readyz
/metrics
```

are not application-authenticated.

But the Envoy SecurityPolicy currently targets the external HTTPS route broadly.

Therefore external health/metrics may still require JWT.

Prometheus avoids this by scraping the internal Service directly.

---

## 33. Why Prometheus Does Not Use Gateway

Prometheus scrapes:

```text
Service/ai-platform-api
```

internally.

This avoids:

```text
external TLS
JWT requirements
Gateway dependency
```

for monitoring.

That separation is intentional.

---

## 34. Gateway Availability vs API Availability

A fully accurate operational model distinguishes:

```text
API process availability
Gateway availability
external end-to-end availability
```

The current application target alert primarily observes the internal Prometheus scrape target.

---

## 35. TLS Rotation

Because TLS is managed declaratively through the certificate/secret integration, clients should not depend on a manually copied leaf certificate.

They should trust the issuing CA.

---

## 36. SecurityPolicy Change Validation

Before applying:

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform-api
```

Apply:

```bash
kubectl apply \
  -k config/platform-api
```

Then re-test both allowed and denied identities.

---

## 37. Allowed Path Validation

Use machine/deployer token for:

```text
GET
POST
PUT
PATCH
```

according to policy.

Use admin token for:

```text
DELETE
```

---

## 38. Denied Path Validation

Useful negative checks:

```text
missing token
viewer mutation
deployer delete
expired token
```

Observe whether rejection happens at Envoy or in the application.

---

## 39. SecurityPolicy and Audit Interaction

If edge policy rejects a request:

```text
Go audit log absent
```

is expected.

Therefore a complete security audit may need:

```text
Envoy evidence
+
application audit evidence
```

---

## 40. Current Gateway/HTTPS Status

```text
[✓] shared Gateway configured
[✓] API HTTPS hostname configured
[✓] API HTTPRoute configured
[✓] HTTP-to-HTTPS redirect configured
[✓] Vault-issued certificate configured
[✓] TLS secret configured
[✓] external HTTPS validated
[✓] Envoy SecurityPolicy attached
[✓] JWT enforcement validated
[✓] role enforcement validated
[✓] deployer DELETE blocked
[✓] admin DELETE allowed
```

---

## 41. Summary

The API external exposure model is:

```text
Client
  |
  | HTTPS + JWT
  v
Envoy Gateway
  |
  | TLS termination
  | HTTPRoute
  | SecurityPolicy
  v
ai-platform-api Service
  |
  | HTTP :8080
  v
Go API
```

The Gateway layer provides TLS and edge policy, while the API preserves independent application authorization and Kubernetes security controls.
