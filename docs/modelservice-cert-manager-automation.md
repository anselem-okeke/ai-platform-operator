# cert-manager Certificate Automation for ModelService Gateway TLS

## Purpose

This document records the completed cert-manager milestone for the `ModelService` platform. It replaces manually generated certificates and manually maintained Kubernetes TLS Secrets with automated issuance, renewal, reissuance, and key rotation.

The validated flow is:

```text
SelfSigned ClusterIssuer
        ↓
Development Root CA Certificate
        ↓
development-root-ca Secret
        ↓
development-ca Issuer
        ↓
fraud-model-local Certificate
        ↓
fraud-model-local-tls Secret
        ↓
Envoy Gateway HTTPS listener
        ↓
HTTPRoute
        ↓
ModelService Service
```

The operator remains portable: it attaches an `HTTPRoute` to an HTTPS listener but does not depend directly on Vault, ACME, AWS, Azure, Google Cloud, or another certificate backend.

---

## Environment

```text
Project:                    /mnt/data/ai-platform-operator
Kubernetes context:         kind-ai-platform-policy
Application namespace:      ai-platform
Gateway namespace:          gateway-system
Envoy namespace:            envoy-gateway-system
cert-manager namespace:     cert-manager
GatewayClass:               envoy
Gateway:                    shared-gateway
Gateway address:            172.19.255.200
Hostname:                   fraud-model.local
TLS Secret:                 fraud-model-local-tls
ModelService:               fraud-model
```

## Responsibility split

### Platform administrator

Owns cert-manager, issuers, CA lifecycle, Gateway listeners, certificate references, trust distribution, monitoring, and production rotation procedures.

### ModelService operator

Owns workload resources, NetworkPolicy, Service, and the operator-managed `HTTPRoute`.

The operator references the HTTPS listener through:

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

---

## Files

```text
config/platform/cert-manager-development-ca.yaml
config/platform/fraud-model-certificate.yaml
config/platform/shared-gateway.yaml
config/platform/fraud-model-http-redirect.yaml
config/samples/platform_v1alpha1_modelservice.yaml
.local/tls/cert-manager-development-ca.crt
```

Keep `.local/` out of Git:

```gitignore
.local/
```

---

## 1. Install cert-manager

```bash
helm upgrade --install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

For reproducible environments, add an approved pinned version:

```bash
helm upgrade --install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --version <approved-version> \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

Wait for the controllers:

```bash
kubectl rollout status deployment/cert-manager \
  -n cert-manager --timeout=300s

kubectl rollout status deployment/cert-manager-webhook \
  -n cert-manager --timeout=300s

kubectl rollout status deployment/cert-manager-cainjector \
  -n cert-manager --timeout=300s
```

Verify:

```bash
kubectl get pods -n cert-manager
kubectl get crd | grep cert-manager.io
```

Expected CRDs include:

```text
certificates.cert-manager.io
certificaterequests.cert-manager.io
issuers.cert-manager.io
clusterissuers.cert-manager.io
```

Optional webhook check:

```bash
kubectl apply --dry-run=server -f - <<'EOF2'
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: webhook-test
  namespace: gateway-system
spec:
  selfSigned: {}
EOF2
```

---

## 2. Bootstrap the development CA

Create `config/platform/cert-manager-development-ca.yaml`:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: development-selfsigned-bootstrap
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: development-root-ca
  namespace: gateway-system
spec:
  isCA: true
  commonName: AI Platform Development Root CA

  subject:
    organizations:
      - AI Platform Development

  secretName: development-root-ca

  duration: 87600h
  renewBefore: 8760h

  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always

  issuerRef:
    name: development-selfsigned-bootstrap
    kind: ClusterIssuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: development-ca
  namespace: gateway-system
spec:
  ca:
    secretName: development-root-ca
```

Apply:

```bash
kubectl apply \
  -f config/platform/cert-manager-development-ca.yaml
```

Verify:

```bash
kubectl get clusterissuer development-selfsigned-bootstrap
kubectl get certificate development-root-ca -n gateway-system
kubectl get issuer development-ca -n gateway-system
```

Expected:

```text
development-selfsigned-bootstrap   Ready=True
development-root-ca                Ready=True
development-ca                     Ready=True
```

Detailed checks:

```bash
kubectl describe clusterissuer development-selfsigned-bootstrap
kubectl describe certificate development-root-ca -n gateway-system
kubectl describe issuer development-ca -n gateway-system
```

### Temporary startup warning

The Issuer may initially report:

```text
Error getting keypair for CA issuer:
secrets "development-root-ca" not found
```

This is expected when the Issuer reconciles before the root CA Secret exists. No correction is needed after the Issuer reaches:

```text
Ready=True
Reason=KeyPairVerified
Message=Signing CA verified
```

---

## 3. Verify the root CA Secret

```bash
kubectl get secret development-root-ca \
  -n gateway-system \
  -o go-template='type={{.type}}{{"\n"}}keys={{range $key, $value := .data}}{{$key}} {{end}}{{"\n"}}'
```

Expected:

```text
type=kubernetes.io/tls
keys=ca.crt tls.crt tls.key
```

Do not print or decode `tls.key`.

The Go template is used because this kubectl JSONPath form is invalid:

```text
{range $key,$value := .data}
```

---

## 4. Export the public development CA

```bash
mkdir -p .local/tls
chmod 700 .local/tls

kubectl get secret development-root-ca \
  -n gateway-system \
  -o jsonpath='{.data.ca\.crt}' |
base64 --decode \
  > .local/tls/cert-manager-development-ca.crt

chmod 644 .local/tls/cert-manager-development-ca.crt
```

Inspect:

```bash
openssl x509 \
  -in .local/tls/cert-manager-development-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

Expected subject and issuer:

```text
O = AI Platform Development
CN = AI Platform Development Root CA
```

---

## 5. Create the server Certificate

Create `config/platform/fraud-model-certificate.yaml`:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: fraud-model-local
  namespace: gateway-system
spec:
  secretName: fraud-model-local-tls

  commonName: fraud-model.local

  dnsNames:
    - fraud-model.local

  duration: 720h
  renewBefore: 168h

  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always

  usages:
    - digital signature
    - key encipherment
    - server auth

  issuerRef:
    name: development-ca
    kind: Issuer
    group: cert-manager.io
```

This requests a 30-day certificate, renewal seven days before expiry, and a new private key on each issuance.

---

## 6. Migrate from the manually managed TLS Secret

Delete the manually created Secret:

```bash
kubectl delete secret fraud-model-local-tls \
  -n gateway-system
```

Immediately apply the Certificate:

```bash
kubectl apply \
  -f config/platform/fraud-model-certificate.yaml
```

Watch issuance:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -w
```

Expected:

```text
NAME                READY   SECRET
fraud-model-local   True    fraud-model-local-tls
```

Verify the recreated Secret:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

Expected:

```text
TYPE                DATA
kubernetes.io/tls   3
```

---

## 7. Verify cert-manager ownership

```bash
kubectl describe certificate fraud-model-local \
  -n gateway-system
```

Expected status:

```text
Ready=True
Reason=Ready
Message=Certificate is up to date and has not expired
```

Inspect Secret annotations:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o yaml |
grep -E \
  'cert-manager.io|controller.cert-manager.io|name: fraud-model-local-tls'
```

Expected metadata includes:

```text
cert-manager.io/certificate-name: fraud-model-local
cert-manager.io/common-name: fraud-model.local
cert-manager.io/issuer-kind: Issuer
cert-manager.io/issuer-name: development-ca
```

Confirm the relationship:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='certificate={.metadata.name}{"\n"}secret={.spec.secretName}{"\n"}ready={.status.conditions[?(@.type=="Ready")].status}{"\n"}'
```

Expected:

```text
certificate=fraud-model-local
secret=fraud-model-local-tls
ready=True
```

---

## 8. Gateway HTTPS listener

The Gateway references the cert-manager-managed Secret:

```yaml
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

The Secret name did not change, so the Gateway manifest did not need to change during migration.

Verify:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide
```

Expected:

```text
PROGRAMMED=True
```

Inspect the HTTPS listener:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.listeners[?(@.name=="https")].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Programmed=True reason=Programmed
Accepted=True reason=Accepted
ResolvedRefs=True reason=ResolvedRefs
```

---

## 9. ModelService route and HTTP redirect

The operator-managed route attaches to the HTTPS listener:

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

Verify:

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

The platform-managed redirect route is:

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

---

## 10. Inspect and verify the issued certificate

Export the leaf certificate:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.tls\.crt}' |
base64 --decode \
  > .local/tls/cert-manager-fraud-model.crt
```

Inspect:

```bash
openssl x509 \
  -in .local/tls/cert-manager-fraud-model.crt \
  -noout \
  -subject \
  -issuer \
  -serial \
  -dates \
  -ext subjectAltName
```

Expected:

```text
subject=CN = fraud-model.local
issuer=O = AI Platform Development, CN = AI Platform Development Root CA
DNS:fraud-model.local
```

Verify against the root CA:

```bash
openssl verify \
  -CAfile .local/tls/cert-manager-development-ca.crt \
  .local/tls/cert-manager-fraud-model.crt
```

Expected:

```text
.local/tls/cert-manager-fraud-model.crt: OK
```

---

## 11. HTTPS and redirect tests

Set the Gateway address:

```bash
GATEWAY_IP=$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)
```

Trusted HTTPS test:

```bash
curl \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Expected: NGINX welcome page.

TLS chain validation:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -CAfile .local/tls/cert-manager-development-ca.crt \
  -verify_return_error \
  </dev/null 2>/dev/null |
grep "Verify return code"
```

Expected:

```text
Verify return code: 0 (ok)
```

HTTP redirect:

```bash
curl -I \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Expected:

```text
HTTP/1.1 301 Moved Permanently
location: https://fraud-model.local/
```

Follow the redirect:

```bash
curl -L \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:80:"$GATEWAY_IP" \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  http://fraud-model.local/
```

Expected: NGINX welcome page.

---

## 12. Renewal and rotation validation

Inspect lifecycle status:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='notBefore={.status.notBefore}{"\n"}notAfter={.status.notAfter}{"\n"}renewalTime={.status.renewalTime}{"\n"}revision={.status.revision}{"\n"}'
```

Validated example:

```text
notBefore=2026-07-31T17:21:18Z
notAfter=2026-08-30T17:21:18Z
renewalTime=2026-08-23T17:21:18Z
revision=2
```

Record the current serial:

```bash
OLD_SERIAL=$(
  kubectl get secret fraud-model-local-tls \
    -n gateway-system \
    -o jsonpath='{.data.tls\.crt}' |
  base64 --decode |
  openssl x509 -noout -serial |
  cut -d= -f2
)

echo "old serial: $OLD_SERIAL"
```

With `cmctl`:

```bash
cmctl renew fraud-model-local \
  -n gateway-system
```

Without `cmctl`, trigger development reissuance by deleting the generated Secret:

```bash
kubectl delete secret fraud-model-local-tls \
  -n gateway-system
```

Watch:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -w
```

Expected transition:

```text
READY=False
READY=True
```

Read the new serial:

```bash
NEW_SERIAL=$(
  kubectl get secret fraud-model-local-tls \
    -n gateway-system \
    -o jsonpath='{.data.tls\.crt}' |
  base64 --decode |
  openssl x509 -noout -serial |
  cut -d= -f2
)

printf 'old serial: %s\nnew serial: %s\n' \
  "$OLD_SERIAL" \
  "$NEW_SERIAL"
```

Validated example:

```text
old serial: 3D1053B7CB6A6624618A4C42F751A66D485F4DC9
new serial: 1A1F0A142A713CBFF9B4EAEE18F51584C9F9B83C
```

The changed serial confirms reissuance. `rotationPolicy: Always` also causes generation of a new private key.

Retest HTTPS after rotation:

```bash
kubectl get secret development-root-ca \
  -n gateway-system \
  -o jsonpath='{.data.ca\.crt}' |
base64 --decode \
  > .local/tls/cert-manager-development-ca.crt

curl \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

Expected: NGINX welcome page.

---

## 13. Cleanup

After cert-manager is validated, remove obsolete manually generated files:

```bash
rm -f \
  .local/tls/fraud-model.local.key \
  .local/tls/fraud-model.local.csr \
  .local/tls/fraud-model.local.crt \
  .local/tls/fraud-model-openssl.cnf \
  .local/tls/local-ca.key \
  .local/tls/local-ca.crt \
  .local/tls/local-ca.srl
```

Keep:

```text
.local/tls/cert-manager-development-ca.crt
```

This is a public trust certificate, not a private key.

---

## 14. Troubleshooting

### cert-manager resource kinds are unknown

Symptoms:

```text
no matches for kind "ClusterIssuer" in version "cert-manager.io/v1"
no matches for kind "Certificate" in version "cert-manager.io/v1"
```

Check:

```bash
kubectl get crd | grep cert-manager.io
kubectl get pods -n cert-manager
```

Install or repair cert-manager before applying the manifests.

### CA Issuer temporarily cannot find its Secret

This can occur during initial bootstrap. Check the final state:

```bash
kubectl get certificate development-root-ca -n gateway-system
kubectl get issuer development-ca -n gateway-system
```

No action is required when both are `Ready=True`.

### Gateway reports unresolved certificate references

Confirm:

```bash
kubectl get secret fraud-model-local-tls -n gateway-system
kubectl get gateway shared-gateway -n gateway-system -o yaml
```

The Secret must be in `gateway-system`, match `certificateRefs[].name`, use type `kubernetes.io/tls`, and contain `tls.crt` and `tls.key`.

### HTTPS fails with unknown issuer

Use the exported development CA:

```bash
curl \
  --cacert .local/tls/cert-manager-development-ca.crt \
  --resolve fraud-model.local:443:"$GATEWAY_IP" \
  https://fraud-model.local/
```

### Shell reports `Expected:: command not found`

Only paste fenced command blocks into the terminal. Explanatory prose and expected-output examples are not shell commands.

### `cmctl` is unavailable

For this development validation, deleting the generated TLS Secret triggers cert-manager reissuance. In production, use an approved operational renewal workflow.

---

## 15. Security guidance

- Never commit CA or leaf private keys.
- Never print or decode `tls.key` during routine checks.
- Use this self-signed CA only for development and controlled testing.
- Do not distribute the development CA as a production trust anchor.
- Restrict access to the `gateway-system` namespace.
- Limit who may modify Issuers, Certificates, TLS Secrets, and Gateway certificate references.
- Monitor `Certificate Ready=False`, issuance failures, and upcoming expiration.
- Document root CA rotation separately from leaf-certificate rotation.
- Use Vault PKI, ACME, or an approved enterprise CA in production.

---

## 16. Portability

Different users can use different certificate providers while preserving the operator contract:

```text
Development cluster → self-signed CA Issuer
Public cluster      → ACME / Let’s Encrypt
Enterprise cluster  → Vault PKI
Private environment → internal corporate CA
Cloud platform      → provider-integrated issuer
```

The following remain unchanged:

```text
ModelService CR
HTTPRoute
Gateway HTTPS listener
Kubernetes TLS Secret contract
ModelService backend
```

Only the issuer configuration changes.

---

## 17. Completion checklist

- [x] cert-manager installed
- [x] cert-manager CRDs available
- [x] bootstrap `ClusterIssuer` created
- [x] bootstrap `ClusterIssuer` is `Ready=True`
- [x] development root CA Certificate created
- [x] development root CA is `Ready=True`
- [x] root CA Secret contains `ca.crt`, `tls.crt`, and `tls.key`
- [x] namespaced CA Issuer created
- [x] CA Issuer is `Ready=True`
- [x] `fraud-model-local` Certificate created
- [x] SAN contains `fraud-model.local`
- [x] certificate uses ECDSA P-256
- [x] `rotationPolicy: Always` configured
- [x] cert-manager manages `fraud-model-local-tls`
- [x] manually maintained TLS Secret replaced
- [x] Gateway remains `Programmed=True`
- [x] HTTPS listener is `Accepted=True`
- [x] HTTPS listener is `ResolvedRefs=True`
- [x] HTTPS listener is `Programmed=True`
- [x] leaf certificate validates against the root CA
- [x] HTTPS reaches the ModelService
- [x] HTTP redirects to HTTPS
- [x] redirect-following request reaches the ModelService
- [x] `renewalTime` populated
- [x] forced reissuance recreated the Secret
- [x] certificate revision increased
- [x] certificate serial changed
- [x] Gateway served the rotated certificate
- [x] HTTPS remained functional after rotation
- [x] obsolete manual certificate files removed

---

## 18. Milestone status

```text
cert-manager certificate automation: COMPLETE
Vault PKI integration:               NEXT
OIDC/JWT authentication:             LATER
```

## 19. Next milestone: Vault PKI

The next flow will be:

```text
cert-manager Certificate
        ↓
Vault-backed Issuer
        ↓
Vault PKI role
        ↓
Vault PKI engine
        ↓
signed certificate
        ↓
fraud-model-local-tls Secret
        ↓
Envoy Gateway
```

The Gateway, HTTPRoute, hostname, TLS Secret contract, and ModelService remain unchanged. Only the issuer reference and Vault integration configuration change.
