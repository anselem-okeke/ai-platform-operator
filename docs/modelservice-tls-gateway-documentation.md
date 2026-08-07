# TLS Termination and HTTP-to-HTTPS Redirect for ModelService

## Overview

This document describes the complete implementation and validation of TLS termination for an externally exposed `ModelService` using Kubernetes Gateway API and Envoy Gateway.

The design keeps certificate provisioning and shared Gateway infrastructure outside the ModelService operator, while the operator manages the application-specific `HTTPRoute`.

The final request flow is:

```text
HTTP request
    ↓ port 80
Shared Gateway
    ↓ 301 redirect
HTTPS request
    ↓ port 443
TLS termination using a Kubernetes TLS Secret
    ↓
Operator-managed HTTPRoute
    ↓
ModelService Service
    ↓
ModelService Pods
```

The implementation was validated on a kind cluster using:

- Calico for NetworkPolicy enforcement;
- Envoy Gateway for Gateway API support;
- MetalLB for a LoadBalancer address;
- a locally generated development CA and server certificate;
- a Kubernetes `kubernetes.io/tls` Secret;
- an HTTPS listener on the shared Gateway;
- an HTTP redirect route;
- an operator-managed `HTTPRoute` attached to the HTTPS listener.

---

## Design principles

### Separation of responsibilities

The shared platform infrastructure owns:

- `GatewayClass`;
- shared `Gateway`;
- TLS certificate lifecycle;
- TLS Secret;
- HTTP-to-HTTPS redirect policy;
- certificate issuer integration.

The ModelService operator owns:

- `HTTPRoute` for the application;
- backend Service routing;
- hostname and path configuration;
- NetworkPolicy allowance for Gateway data-plane traffic;
- lifecycle and drift correction of the application route.

This separation keeps the operator portable across clusters and certificate providers.

### Provider-neutral certificate contract

The Gateway consumes a standard Kubernetes TLS Secret:

```yaml
apiVersion: v1
kind: Secret
type: kubernetes.io/tls
```

The Secret may later be created by:

- cert-manager with ACME or Let's Encrypt;
- cert-manager with Vault PKI;
- External Secrets Operator with Vault KV;
- a cloud secret or certificate manager;
- a corporate CA;
- a manually generated development certificate.

The operator does not need to know which provider created the Secret.

---

## Environment

The validated cluster used:

```text
Cluster context: kind-ai-platform-policy
Gateway namespace: gateway-system
Envoy data-plane namespace: envoy-gateway-system
Application namespace: ai-platform
Gateway name: shared-gateway
Gateway address: 172.19.255.200
Application hostname: fraud-model.local
Backend Service: fraud-model
Backend port: 8080
TLS Secret: fraud-model-local-tls
```

The ModelService workload used:

```text
Image: nginxinc/nginx-unprivileged:1.31-alpine
Replicas: 2
Service port: 8080
```

---

## Files created or updated

```text
/mnt/data/ai-platform-operator/.local/tls/
/mnt/data/ai-platform-operator/config/platform/shared-gateway.yaml
/mnt/data/ai-platform-operator/config/platform/fraud-model-http-redirect.yaml
/mnt/data/ai-platform-operator/config/samples/platform_v1alpha1_modelservice.yaml
```

The `.local/` directory stores local certificate material and must not be committed to Git.

---

# 1. Local TLS workspace

Create the local TLS directory:

```bash
cd /mnt/data/ai-platform-operator

mkdir -p .local/tls
chmod 700 .local/tls
```

Add it to `.gitignore`:

```bash
grep -qxF '.local/' .gitignore || \
  printf '\n.local/\n' >> .gitignore
```

This protects:

- the local CA private key;
- the server private key;
- the certificate-signing request;
- generated serial files.

---

# 2. Local development certificate authority

Generate the CA private key:

```bash
openssl genrsa \
  -out .local/tls/local-ca.key \
  4096

chmod 600 .local/tls/local-ca.key
```

Create the self-signed CA certificate:

```bash
openssl req \
  -x509 \
  -new \
  -sha256 \
  -days 3650 \
  -key .local/tls/local-ca.key \
  -out .local/tls/local-ca.crt \
  -subj "/C=DE/O=AI Platform Development/CN=AI Platform Local Development CA"
```

Inspect it:

```bash
openssl x509 \
  -in .local/tls/local-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

Expected characteristics:

```text
Subject: AI Platform Local Development CA
Issuer:  AI Platform Local Development CA
```

Because this is a self-signed development CA, its subject and issuer are the same.

---

# 3. Server certificate configuration

Create:

```text
.local/tls/fraud-model-openssl.cnf
```

Contents:

```ini
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = distinguished_name
req_extensions = request_extensions

[distinguished_name]
C = DE
O = AI Platform Development
CN = fraud-model.local

[request_extensions]
subjectAltName = @subject_alt_names
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth

[subject_alt_names]
DNS.1 = fraud-model.local
```

The Subject Alternative Name is essential because modern TLS clients validate the requested hostname against the certificate SAN.

---

# 4. Server key and certificate-signing request

Generate the server private key:

```bash
openssl genrsa \
  -out .local/tls/fraud-model.local.key \
  2048

chmod 600 .local/tls/fraud-model.local.key
```

Generate the CSR:

```bash
openssl req \
  -new \
  -key .local/tls/fraud-model.local.key \
  -out .local/tls/fraud-model.local.csr \
  -config .local/tls/fraud-model-openssl.cnf
```

Inspect the SAN:

```bash
openssl req \
  -in .local/tls/fraud-model.local.csr \
  -noout \
  -subject \
  -text |
  grep -A2 "Subject Alternative Name"
```

Expected:

```text
DNS:fraud-model.local
```

---

# 5. Sign the server certificate

Sign the CSR with the local CA:

```bash
openssl x509 \
  -req \
  -sha256 \
  -days 825 \
  -in .local/tls/fraud-model.local.csr \
  -CA .local/tls/local-ca.crt \
  -CAkey .local/tls/local-ca.key \
  -CAcreateserial \
  -out .local/tls/fraud-model.local.crt \
  -extensions request_extensions \
  -extfile .local/tls/fraud-model-openssl.cnf
```

Inspect the certificate:

```bash
openssl x509 \
  -in .local/tls/fraud-model.local.crt \
  -noout \
  -subject \
  -issuer \
  -dates \
  -ext subjectAltName
```

Expected:

```text
Subject CN: fraud-model.local
Issuer CN: AI Platform Local Development CA
SAN: DNS:fraud-model.local
```

Verify the certificate chain:

```bash
openssl verify \
  -CAfile .local/tls/local-ca.crt \
  .local/tls/fraud-model.local.crt
```

Expected:

```text
.local/tls/fraud-model.local.crt: OK
```

---

# 6. Verify that the certificate and private key match

Calculate the public-key hashes:

```bash
CERT_PUBLIC_KEY_HASH=$(
  openssl x509 \
    -in .local/tls/fraud-model.local.crt \
    -pubkey \
    -noout |
  openssl sha256
)

KEY_PUBLIC_KEY_HASH=$(
  openssl pkey \
    -in .local/tls/fraud-model.local.key \
    -pubout |
  openssl sha256
)

printf 'certificate: %s\nkey:         %s\n' \
  "$CERT_PUBLIC_KEY_HASH" \
  "$KEY_PUBLIC_KEY_HASH"
```

The hashes must be identical.

This prevents creating a TLS Secret with a certificate and private key that do not belong together.

---

# 7. Kubernetes TLS Secret

The Secret was created in the Gateway namespace because the HTTPS listener references it from the Gateway object.

Create or update the Secret idempotently:

```bash
kubectl create secret tls fraud-model-local-tls \
  -n gateway-system \
  --cert=.local/tls/fraud-model.local.crt \
  --key=.local/tls/fraud-model.local.key \
  --dry-run=client \
  -o yaml |
kubectl apply -f -
```

Verify it:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

Expected:

```text
NAME                    TYPE                DATA
fraud-model-local-tls   kubernetes.io/tls   2
```

Inspect only safe metadata:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o jsonpath='name={.metadata.name}{"\n"}type={.type}{"\n"}'
```

Expected:

```text
name=fraud-model-local-tls
type=kubernetes.io/tls
```

Do not print or commit the decoded private key.

---

# 8. Shared Gateway with HTTP and HTTPS listeners

File:

```text
config/platform/shared-gateway.yaml
```

Configuration:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: envoy
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: shared-gateway
  namespace: gateway-system
spec:
  gatewayClassName: envoy

  listeners:
    - name: http
      protocol: HTTP
      port: 80
      hostname: fraud-model.local

      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"

    - name: https
      protocol: HTTPS
      port: 443
      hostname: fraud-model.local

      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: fraud-model-local-tls

      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"
```

Apply it:

```bash
kubectl apply \
  -f config/platform/shared-gateway.yaml
```

The Gateway terminates TLS at port 443 and forwards plain HTTP inside the cluster to the backend Service.

---

# 9. Namespace permission for route attachment

The Gateway allows routes only from namespaces with:

```text
shared-gateway-access=true
```

Apply the label:

```bash
kubectl label namespace ai-platform \
  shared-gateway-access=true \
  --overwrite
```

Verify:

```bash
kubectl get namespace ai-platform \
  --show-labels
```

Expected label:

```text
shared-gateway-access=true
```

---

# 10. ModelService route attached to HTTPS

The operator-managed route was configured through the ModelService resource.

Relevant sample configuration:

```yaml
spec:
  exposure:
    enabled: true
    hostname: fraud-model.local
    pathPrefix: /
    gatewayName: shared-gateway
    gatewayNamespace: gateway-system
    gatewaySectionName: https
    gatewayDataPlaneNamespace: envoy-gateway-system
```

The important change for TLS was:

```yaml
gatewaySectionName: https
```

Apply the ModelService:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Verify the generated route:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='gateway={.spec.parentRefs[0].name}{"\n"}namespace={.spec.parentRefs[0].namespace}{"\n"}listener={.spec.parentRefs[0].sectionName}{"\n"}hostname={.spec.hostnames[0]}{"\n"}'
```

Expected:

```text
gateway=shared-gateway
namespace=gateway-system
listener=https
hostname=fraud-model.local
```

Verify route status:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Accepted=True reason=Accepted
ResolvedRefs=True reason=ResolvedRefs
```

---

# 11. HTTP-to-HTTPS redirect route

File:

```text
config/platform/fraud-model-http-redirect.yaml
```

Configuration:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: fraud-model-http-redirect
  namespace: ai-platform
spec:
  parentRefs:
    - name: shared-gateway
      namespace: gateway-system
      sectionName: http

  hostnames:
    - fraud-model.local

  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            statusCode: 301
```

Apply it:

```bash
kubectl apply \
  -f config/platform/fraud-model-http-redirect.yaml
```

This route has no backend. It returns a redirect response instead of forwarding traffic.

Verify:

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform \
  -o jsonpath='listener={.spec.parentRefs[0].sectionName}{"\n"}hostname={.spec.hostnames[0]}{"\n"}redirectScheme={.spec.rules[0].filters[0].requestRedirect.scheme}{"\n"}statusCode={.spec.rules[0].filters[0].requestRedirect.statusCode}{"\n"}'
```

Expected:

```text
listener=http
hostname=fraud-model.local
redirectScheme=https
statusCode=301
```

---

# 12. Gateway and Envoy validation

The final Gateway state was:

```text
NAME             CLASS   ADDRESS          PROGRAMMED
shared-gateway   envoy   172.19.255.200   True
```

The generated Envoy Service exposed both ports:

```text
80:32324/TCP
443:31429/TCP
```

Validated command:

```bash
kubectl get service \
  -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=shared-gateway
```

Validated result:

```text
TYPE           EXTERNAL-IP      PORT(S)
LoadBalancer   172.19.255.200   80:32324/TCP,443:31429/TCP
```

---

# 13. HTTPS validation with the trusted development CA

Set the Gateway IP:

```bash
GATEWAY_IP=$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)
```

Send a trusted HTTPS request:

```bash
curl \
  --cacert .local/tls/local-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Validated result:

```text
Welcome to nginx!
```

This proved the entire flow:

```text
Client
→ trusted TLS handshake
→ Envoy Gateway HTTPS listener
→ TLS termination
→ HTTPRoute hostname match
→ Service backend
→ ModelService Pod
```

---

# 14. Direct certificate verification

Run:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -CAfile .local/tls/local-ca.crt \
  -verify_return_error \
  </dev/null 2>/dev/null |
grep "Verify return code"
```

Validated result:

```text
Verify return code: 0 (ok)
```

This confirmed:

- the certificate chain was trusted;
- the certificate was valid for the requested server name;
- Envoy served the expected certificate;
- the TLS listener was functioning correctly.

---

# 15. HTTP redirect validation

Run:

```bash
curl -I \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Validated response:

```text
HTTP/1.1 301 Moved Permanently
location: https://fraud-model.local/
```

This confirmed that plain HTTP requests are upgraded to HTTPS.

---

# 16. Follow the redirect to HTTPS

Run:

```bash
curl -L \
  --cacert .local/tls/local-ca.crt \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Validated result:

```text
Welcome to nginx!
```

This proved the complete sequence:

```text
HTTP request
→ 301 redirect
→ HTTPS connection
→ trusted certificate
→ HTTPRoute
→ ModelService
```

---

# 17. Invalid hostname test

Run:

```bash
curl \
  --cacert .local/tls/local-ca.crt \
  --resolve wrong-host.local:443:"$GATEWAY_IP" \
  https://wrong-host.local/
```

Validated result:

```text
curl: (35) Recv failure: Connection reset by peer
```

This was acceptable for this Gateway configuration.

The HTTPS listener was restricted to:

```text
fraud-model.local
```

Envoy rejected the unmatched SNI hostname before successfully serving the application route.

The exact failure can differ by Gateway implementation. Some environments may instead return a certificate hostname mismatch or a TLS alert.

---

# 18. Untrusted CA test

Run the HTTPS request without providing the local CA:

```bash
curl \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Validated result:

```text
curl: (60) SSL certificate problem: unable to get local issuer certificate
```

This was expected because the local development CA was not installed in the host operating system's trust store.

This test confirmed that certificate verification was active and that the successful test depended on explicitly trusting the development CA.

Do not use `curl -k` as proof of correct TLS validation because it disables certificate verification.

---

# 19. Final resource validation

The following command summarized the complete platform state:

```bash
printf '\nGateway:\n'
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide

printf '\nTLS Secret:\n'
kubectl get secret fraud-model-local-tls \
  -n gateway-system

printf '\nRoutes:\n'
kubectl get httproute \
  -n ai-platform

printf '\nEnvoy Service:\n'
kubectl get service \
  -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=shared-gateway

printf '\nModelService:\n'
kubectl get modelservice fraud-model \
  -n ai-platform
```

Validated state:

```text
Gateway:
shared-gateway   envoy   172.19.255.200   Programmed=True

TLS Secret:
fraud-model-local-tls   kubernetes.io/tls   2

Routes:
fraud-model                 ["fraud-model.local"]
fraud-model-http-redirect   ["fraud-model.local"]

Envoy Service:
LoadBalancer   172.19.255.200   ports 80 and 443

ModelService:
Ready   2 replicas   nginxinc/nginx-unprivileged:1.31-alpine
```

---

# 20. Security properties achieved

The implementation now provides:

- encrypted external traffic;
- certificate-chain validation;
- hostname validation;
- TLS termination at the shared Gateway;
- automatic redirection from HTTP to HTTPS;
- separation between shared platform infrastructure and application routing;
- no private keys stored in Git;
- an internal ClusterIP backend;
- Gateway-controlled external exposure;
- compatibility with future automated certificate issuers.

The backend remains internal:

```text
http://fraud-model.ai-platform.svc.cluster.local:8080
```

External clients use:

```text
https://fraud-model.local/
```

---

# 21. Portability model

The operator itself does not create certificates or depend on a particular secret-management system.

The portable contract is:

```text
Certificate provider
        ↓
Kubernetes TLS Secret
        ↓
Gateway HTTPS listener
        ↓
Operator-managed HTTPRoute
        ↓
ModelService
```

Possible certificate providers include:

| Environment | Certificate source |
|---|---|
| Local kind development | locally generated CA and certificate |
| Public production | cert-manager with ACME or Let's Encrypt |
| Enterprise private platform | cert-manager with Vault PKI |
| Existing static certificate workflow | Vault KV and External Secrets Operator |
| Cloud-managed platform | cloud CA or secret-manager integration |
| Air-gapped environment | internal CA |

This avoids coupling the ModelService API to Vault, AWS, Azure, Google Cloud, or any other provider.

---

# 22. Recommended next step

The manual TLS Secret proved the Gateway and routing design.

The next improvement is certificate lifecycle automation:

```text
cert-manager Certificate
        ↓
Issuer or ClusterIssuer
        ↓
Kubernetes TLS Secret
        ↓
Gateway HTTPS listener
```

Recommended sequence:

1. install cert-manager;
2. create a development Issuer;
3. create a `Certificate` resource;
4. let cert-manager generate `fraud-model-local-tls`;
5. confirm the Gateway automatically uses the renewed Secret;
6. configure Vault PKI as an Issuer;
7. document alternate issuers for other platform users.

For the existing Vault VM, the preferred long-term certificate architecture is:

```text
cert-manager
    ↓ authenticates to Vault
Vault PKI
    ↓ issues certificate
cert-manager
    ↓ writes and renews Kubernetes TLS Secret
Gateway
    ↓ terminates HTTPS
```

Vault KV plus External Secrets Operator remains useful for static certificate material, but Vault PKI plus cert-manager is preferable for dynamic issuance and automatic renewal.

---

# Completion checklist

```text
[✓] Local development CA created
[✓] Server certificate generated
[✓] Certificate SAN contains fraud-model.local
[✓] Certificate verifies against local CA
[✓] Certificate and private key match
[✓] Kubernetes TLS Secret created
[✓] TLS Secret stored in gateway-system
[✓] Shared Gateway HTTP listener configured
[✓] Shared Gateway HTTPS listener configured
[✓] HTTPS listener references TLS Secret
[✓] Gateway remains Programmed=True
[✓] Envoy LoadBalancer exposes ports 80 and 443
[✓] ai-platform namespace allowed to attach routes
[✓] ModelService HTTPRoute attached to HTTPS listener
[✓] HTTPRoute Accepted=True
[✓] HTTPRoute ResolvedRefs=True
[✓] HTTP-to-HTTPS redirect route created
[✓] HTTP request returns 301
[✓] Location header points to HTTPS
[✓] Trusted HTTPS request succeeds
[✓] OpenSSL verification returns code 0
[✓] Redirect-following request reaches NGINX
[✓] Wrong hostname is rejected
[✓] Untrusted CA is rejected
[✓] Backend ModelService remains Ready
```

