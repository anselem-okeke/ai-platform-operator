# Phase 03 — Vault PKI and Keycloak HTTPS

## 1. Objective

This phase exposes Keycloak securely through the shared Envoy Gateway by using a TLS certificate issued from HashiCorp Vault through cert-manager.

The final request path is:

```text
Client
  ↓ HTTPS
MetalLB address: 172.19.255.200
  ↓
Gateway/gateway-system/shared-gateway
  ↓ listener: keycloak-https
Vault-issued certificate for auth.ai-platform.local
  ↓
HTTPRoute/keycloak/keycloak
  ↓
Service/keycloak:8080
  ↓
Keycloak Pod
```

The phase also adds an HTTP-to-HTTPS redirect:

```text
http://auth.ai-platform.local
  ↓ 301
https://auth.ai-platform.local
```

---

## 2. Environment used

```text
Repository:
  /mnt/data/ai-platform-operator

Kubernetes context:
  kind-ai-platform-policy

Kind cluster:
  ai-platform-policy

Gateway namespace:
  gateway-system

Gateway name:
  shared-gateway

Gateway address:
  172.19.255.200

Keycloak namespace:
  keycloak

Keycloak hostname:
  auth.ai-platform.local

Keycloak Service port:
  8080

Vault address:
  https://vault.platform.local:8200

Vault Kubernetes auth mount:
  kubernetes-kind/

Vault PKI mount:
  pki_modelservice/

cert-manager namespace:
  cert-manager
```

Existing Vault resources used by the cluster:

```text
Vault server CA Secret:
  gateway-system/vault-server-ca

Existing Vault auth mount:
  kubernetes-kind/

Existing PKI mount:
  pki_modelservice/
```

This phase creates a dedicated Keycloak certificate role and does not modify the existing ModelService certificate configuration.

---

## 3. Final resource model

### Vault resources

```text
PKI role:
  keycloak

Policy:
  cert-manager-keycloak-pki

Kubernetes auth role:
  cert-manager-keycloak
```

### Kubernetes resources

```text
ServiceAccount:
  gateway-system/cert-manager-keycloak-issuer

Role:
  gateway-system/cert-manager-keycloak-tokenrequest

RoleBinding:
  gateway-system/cert-manager-keycloak-tokenrequest

Issuer:
  gateway-system/vault-keycloak-issuer

Certificate:
  gateway-system/auth-ai-platform-local

TLS Secret:
  gateway-system/auth-ai-platform-local-tls

Gateway listener:
  gateway-system/shared-gateway → keycloak-https

HTTPS route:
  keycloak/keycloak

HTTP redirect route:
  keycloak/keycloak-http-redirect
```

---

## 4. File layout

```text
config/platform/keycloak/
├── keycloak-http-redirect.yaml
├── keycloak-httproute.yaml
├── kustomization.yaml
├── namespace.yaml
├── networkpolicy.yaml
├── postgres.yaml
├── keycloak.yaml
└── tls/
    ├── cert-manager-keycloak-serviceaccount.yaml
    ├── cert-manager-keycloak-token-rbac.yaml
    ├── keycloak-certificate.yaml
    └── vault-keycloak-issuer.yaml

infrastructure/keycloak/
├── policies/
│   └── cert-manager-keycloak-pki.hcl
└── scripts/
    ├── configure-keycloak-vault-pki.sh
    └── validate-keycloak-https.sh
```

The shared Gateway source remains outside the Keycloak directory:

```text
config/platform/shared-gateway.yaml
```

---

## 5. Prerequisites

Before starting, confirm that:

- Vault is reachable from the jumpbox.
- Vault is unsealed.
- the `pki_modelservice/` PKI engine exists;
- the `kubernetes-kind/` Vault auth mount exists;
- cert-manager is installed and healthy;
- the shared Envoy Gateway is `Programmed=True`;
- the `keycloak` namespace has the label `shared-gateway-access=true`;
- Keycloak and PostgreSQL are already running;
- `vault-server-ca` exists in `gateway-system`.

### 5.1 Verify Kubernetes context

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

### 5.2 Verify cert-manager

```bash
kubectl get deployment,pod \
  -n cert-manager
```

Expected:

```text
cert-manager             Ready
cert-manager-cainjector  Ready
cert-manager-webhook     Ready
```

### 5.3 Verify Gateway

```bash
kubectl get gateway shared-gateway \
  -n gateway-system
```

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Programmed=True
```

### 5.4 Verify namespace label

```bash
kubectl get namespace keycloak \
  --show-labels
```

The namespace must include:

```text
shared-gateway-access=true
```

Apply the label when missing:

```bash
kubectl label namespace keycloak \
  shared-gateway-access=true \
  --overwrite
```

### 5.5 Verify Vault server CA Secret

```bash
kubectl get secret vault-server-ca \
  -n gateway-system
```

---

## 6. Create the cert-manager ServiceAccount

Create:

```text
config/platform/keycloak/tls/cert-manager-keycloak-serviceaccount.yaml
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cert-manager-keycloak-issuer
  namespace: gateway-system
  labels:
    app.kubernetes.io/name: cert-manager-keycloak-issuer
    app.kubernetes.io/component: certificate-issuer
    app.kubernetes.io/part-of: ai-platform
automountServiceAccountToken: false
```

This ServiceAccount is used only by cert-manager when requesting a short-lived Kubernetes token for Vault authentication.

The ServiceAccount itself does not automatically receive a mounted Kubernetes API token.

---

## 7. Permit cert-manager to request a token

Create:

```text
config/platform/keycloak/tls/cert-manager-keycloak-token-rbac.yaml
```

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cert-manager-keycloak-tokenrequest
  namespace: gateway-system
  labels:
    app.kubernetes.io/name: cert-manager-keycloak-tokenrequest
    app.kubernetes.io/component: certificate-issuer
    app.kubernetes.io/part-of: ai-platform
rules:
  - apiGroups:
      - ""
    resources:
      - serviceaccounts/token
    resourceNames:
      - cert-manager-keycloak-issuer
    verbs:
      - create
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cert-manager-keycloak-tokenrequest
  namespace: gateway-system
  labels:
    app.kubernetes.io/name: cert-manager-keycloak-tokenrequest
    app.kubernetes.io/component: certificate-issuer
    app.kubernetes.io/part-of: ai-platform
subjects:
  - kind: ServiceAccount
    name: cert-manager
    namespace: cert-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: cert-manager-keycloak-tokenrequest
```

Validate the permission:

```bash
kubectl auth can-i create \
  serviceaccounts/cert-manager-keycloak-issuer \
  --subresource=token \
  --as=system:serviceaccount:cert-manager:cert-manager \
  -n gateway-system
```

Expected:

```text
yes
```

This Role does not permit cert-manager to request arbitrary ServiceAccount tokens. It is restricted to one named ServiceAccount.

---

## 8. Create the Vault policy

Create:

```text
infrastructure/keycloak/policies/cert-manager-keycloak-pki.hcl
```

```hcl
path "pki_modelservice/sign/keycloak" {
  capabilities = ["create", "update"]
}

path "pki_modelservice/issue/keycloak" {
  capabilities = ["create", "update"]
}
```

The policy only allows certificate issuance through the dedicated Keycloak PKI role.

It does not grant access to:

- Vault administration;
- arbitrary PKI roles;
- secret engines;
- token management;
- policy management.

---

## 9. Configure the Vault PKI role and Kubernetes auth role

Create:

```text
infrastructure/keycloak/scripts/configure-keycloak-vault-pki.sh
```

```bash
#!/usr/bin/env bash
set -euo pipefail

VAULT_PKI_PATH="${VAULT_PKI_PATH:-pki_modelservice}"
VAULT_PKI_ROLE="${VAULT_PKI_ROLE:-keycloak}"
VAULT_POLICY="${VAULT_POLICY:-cert-manager-keycloak-pki}"
VAULT_K8S_AUTH_PATH="${VAULT_K8S_AUTH_PATH:-kubernetes-kind}"
VAULT_K8S_ROLE="${VAULT_K8S_ROLE:-cert-manager-keycloak}"

KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-auth.ai-platform.local}"
SERVICE_ACCOUNT_NAME="${SERVICE_ACCOUNT_NAME:-cert-manager-keycloak-issuer}"
SERVICE_ACCOUNT_NAMESPACE="${SERVICE_ACCOUNT_NAMESPACE:-gateway-system}"
TOKEN_AUDIENCE="${TOKEN_AUDIENCE:-https://kubernetes.default.svc.cluster.local}"

POLICY_FILE="${POLICY_FILE:-infrastructure/keycloak/policies/cert-manager-keycloak-pki.hcl}"

for command_name in vault; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command is missing: ${command_name}" >&2
    exit 1
  }
done

: "${VAULT_ADDR:?VAULT_ADDR is required}"
: "${VAULT_CACERT:?VAULT_CACERT is required}"

if [[ ! -s "${POLICY_FILE}" ]]; then
  echo "ERROR: Vault policy file is missing: ${POLICY_FILE}" >&2
  exit 1
fi

echo "Checking Vault status..."
vault status >/dev/null

echo "Writing Vault PKI role ${VAULT_PKI_ROLE}..."

vault write \
  "${VAULT_PKI_PATH}/roles/${VAULT_PKI_ROLE}" \
  allowed_domains="${KEYCLOAK_HOSTNAME}" \
  allow_bare_domains=true \
  allow_subdomains=false \
  allow_glob_domains=false \
  allow_wildcard_certificates=false \
  allow_localhost=false \
  allow_ip_sans=false \
  enforce_hostnames=true \
  require_cn=true \
  server_flag=true \
  client_flag=false \
  key_type=ec \
  key_bits=256 \
  key_usage="DigitalSignature,KeyAgreement,KeyEncipherment" \
  max_ttl=720h

echo "Writing Vault policy ${VAULT_POLICY}..."

vault policy write \
  "${VAULT_POLICY}" \
  "${POLICY_FILE}"

echo "Writing Vault Kubernetes auth role ${VAULT_K8S_ROLE}..."

vault write \
  "auth/${VAULT_K8S_AUTH_PATH}/role/${VAULT_K8S_ROLE}" \
  bound_service_account_names="${SERVICE_ACCOUNT_NAME}" \
  bound_service_account_namespaces="${SERVICE_ACCOUNT_NAMESPACE}" \
  audience="${TOKEN_AUDIENCE}" \
  policies="${VAULT_POLICY}" \
  token_ttl=10m \
  token_max_ttl=30m

echo
echo "PASS: Vault Keycloak PKI configuration completed."
echo "PKI role:             ${VAULT_PKI_PATH}/roles/${VAULT_PKI_ROLE}"
echo "Vault policy:         ${VAULT_POLICY}"
echo "Kubernetes auth role: auth/${VAULT_K8S_AUTH_PATH}/role/${VAULT_K8S_ROLE}"
```

Make the script executable:

```bash
chmod +x \
  infrastructure/keycloak/scripts/configure-keycloak-vault-pki.sh
```

Validate shell syntax:

```bash
bash -n \
  infrastructure/keycloak/scripts/configure-keycloak-vault-pki.sh
```

### 9.1 Run the script from the Vault jumpbox

The Vault CLI configuration used during this implementation was:

```bash
export VAULT_ADDR=https://vault.platform.local:8200
export VAULT_CACERT=/home/jumpbox/.vault-tls/vault-ca.crt
```

Authenticate to Vault using an authorized administrative token, then run:

```bash
cd /path/to/ai-platform-operator

infrastructure/keycloak/scripts/configure-keycloak-vault-pki.sh
```

Do not commit the Vault token.

### 9.2 Verify the Vault PKI role

```bash
vault read \
  pki_modelservice/roles/keycloak
```

Important expected values:

```text
allowed_domains                 auth.ai-platform.local
allow_bare_domains              true
allow_subdomains                false
allow_wildcard_certificates     false
allow_ip_sans                   false
server_flag                     true
client_flag                     false
key_type                        ec
key_bits                        256
max_ttl                         720h
```

### 9.3 Verify the Vault policy

```bash
vault policy read \
  cert-manager-keycloak-pki
```

### 9.4 Verify the Vault Kubernetes auth role

```bash
vault read \
  auth/kubernetes-kind/role/cert-manager-keycloak
```

Important expected values:

```text
bound_service_account_names       cert-manager-keycloak-issuer
bound_service_account_namespaces  gateway-system
audience                          https://kubernetes.default.svc.cluster.local
policies                          cert-manager-keycloak-pki
```

---

## 10. Create the cert-manager Vault Issuer

Create:

```text
config/platform/keycloak/tls/vault-keycloak-issuer.yaml
```

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: vault-keycloak-issuer
  namespace: gateway-system
  labels:
    app.kubernetes.io/name: vault-keycloak-issuer
    app.kubernetes.io/component: certificate-issuer
    app.kubernetes.io/part-of: ai-platform
spec:
  vault:
    server: https://vault.platform.local:8200
    path: pki_modelservice/sign/keycloak
    caBundleSecretRef:
      name: vault-server-ca
      key: ca.crt
    auth:
      kubernetes:
        mountPath: /v1/auth/kubernetes-kind
        role: cert-manager-keycloak
        serviceAccountRef:
          name: cert-manager-keycloak-issuer
          audiences:
            - https://kubernetes.default.svc.cluster.local
```

Important distinction:

```text
vault-server-ca
  trusts the HTTPS endpoint of the Vault server

auth-ai-platform-local-tls
  later contains the certificate used by the Gateway
```

---

## 11. Create the Keycloak certificate

Create:

```text
config/platform/keycloak/tls/keycloak-certificate.yaml
```

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: auth-ai-platform-local
  namespace: gateway-system
  labels:
    app.kubernetes.io/name: auth-ai-platform-local
    app.kubernetes.io/component: edge-tls
    app.kubernetes.io/part-of: ai-platform
spec:
  secretName: auth-ai-platform-local-tls
  commonName: auth.ai-platform.local
  dnsNames:
    - auth.ai-platform.local
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
    name: vault-keycloak-issuer
    kind: Issuer
    group: cert-manager.io
```

Certificate lifetime:

```text
Duration:
  30 days

Renewal begins:
  7 days before expiry
```

`rotationPolicy: Always` ensures a new private key is generated during renewal.

---

## 12. Add the Keycloak HTTPS listener to the shared Gateway

Update:

```text
config/platform/shared-gateway.yaml
```

The Gateway must preserve the existing generic HTTP listener and add a dedicated hostname-specific HTTPS listener.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: shared-gateway
  namespace: gateway-system
spec:
  gatewayClassName: envoy-gateway
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"

    - name: keycloak-https
      hostname: auth.ai-platform.local
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: auth-ai-platform-local-tls
      allowedRoutes:
        namespaces:
          from: Selector
          selector:
            matchLabels:
              shared-gateway-access: "true"
```

Multiple HTTPS listeners may use port `443` when their hostnames are distinct. Envoy selects the listener and certificate by TLS SNI.

---

## 13. Create the Keycloak HTTPS route

Create:

```text
config/platform/keycloak/keycloak-httproute.yaml
```

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: keycloak
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak
    app.kubernetes.io/component: edge-routing
    app.kubernetes.io/part-of: ai-platform
spec:
  hostnames:
    - auth.ai-platform.local
  parentRefs:
    - name: shared-gateway
      namespace: gateway-system
      sectionName: keycloak-https
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: keycloak
          port: 8080
```

The route attaches only to the `keycloak-https` listener.

---

## 14. Create the HTTP-to-HTTPS redirect route

Create:

```text
config/platform/keycloak/keycloak-http-redirect.yaml
```

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: keycloak-http-redirect
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak-http-redirect
    app.kubernetes.io/component: edge-routing
    app.kubernetes.io/part-of: ai-platform
spec:
  hostnames:
    - auth.ai-platform.local
  parentRefs:
    - name: shared-gateway
      namespace: gateway-system
      sectionName: http
  rules:
    - filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            statusCode: 301
```

This route does not forward traffic to Keycloak. It only returns a redirect response.

---

## 15. Restrict network access to Keycloak and PostgreSQL

Create or update:

```text
config/platform/keycloak/networkpolicy.yaml
```

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: keycloak-ingress
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak
    app.kubernetes.io/component: network-security
    app.kubernetes.io/part-of: ai-platform
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: keycloak
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: envoy-gateway-system
          podSelector:
            matchLabels:
              app.kubernetes.io/component: proxy
              app.kubernetes.io/name: envoy
              gateway.envoyproxy.io/owning-gateway-name: shared-gateway
              gateway.envoyproxy.io/owning-gateway-namespace: gateway-system
      ports:
        - protocol: TCP
          port: 8080

    - from:
        - podSelector:
            matchLabels:
              keycloak-management-client: "true"
      ports:
        - protocol: TCP
          port: 9000
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: keycloak-postgres-ingress
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak-postgres
    app.kubernetes.io/component: network-security
    app.kubernetes.io/part-of: ai-platform
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: keycloak-postgres
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: keycloak
      ports:
        - protocol: TCP
          port: 5432
```

Key points:

- application traffic to Keycloak port `8080` is restricted to the Envoy data-plane Pods for `shared-gateway`;
- management port `9000` is not generally exposed;
- PostgreSQL port `5432` is restricted to the Keycloak Pods.

---

## 16. Update the Keycloak Kustomization

Update:

```text
config/platform/keycloak/kustomization.yaml
```

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - namespace.yaml
  - postgres.yaml
  - keycloak.yaml
  - networkpolicy.yaml
  - keycloak-httproute.yaml
  - keycloak-http-redirect.yaml
  - tls/cert-manager-keycloak-serviceaccount.yaml
  - tls/cert-manager-keycloak-token-rbac.yaml
  - tls/vault-keycloak-issuer.yaml
  - tls/keycloak-certificate.yaml

secretGenerator:
  - name: keycloak-postgres-credentials
    namespace: keycloak
    envs:
      - .secrets/postgres.env

  - name: keycloak-bootstrap-admin
    namespace: keycloak
    envs:
      - .secrets/bootstrap-admin.env

generatorOptions:
  disableNameSuffixHash: true
  labels:
    app.kubernetes.io/part-of: ai-platform
    app.kubernetes.io/managed-by: kustomize
```

Later phases add more generated Secrets to this same Kustomization.

---

## 17. Apply the resources

Run from the Ansible VM:

```bash
cd /mnt/data/ai-platform-operator
```

### 17.1 Apply the shared Gateway

```bash
kubectl apply \
  --dry-run=server \
  -f config/platform/shared-gateway.yaml
```

```bash
kubectl apply \
  -f config/platform/shared-gateway.yaml
```

### 17.2 Validate the Keycloak rendering

```bash
kubectl kustomize \
  config/platform/keycloak \
  > /tmp/keycloak-https-rendered.yaml
```

```bash
kubectl apply \
  --dry-run=server \
  -f /tmp/keycloak-https-rendered.yaml
```

Remove the rendered file securely:

```bash
shred -u \
  /tmp/keycloak-https-rendered.yaml
```

### 17.3 Apply the Keycloak Kustomization

```bash
kubectl apply \
  -k config/platform/keycloak
```

---

## 18. Wait for the Issuer and Certificate

### 18.1 Wait for the Issuer

```bash
kubectl wait \
  --for=condition=Ready \
  issuer/vault-keycloak-issuer \
  -n gateway-system \
  --timeout=180s
```

Expected:

```text
issuer.cert-manager.io/vault-keycloak-issuer condition met
```

Inspect:

```bash
kubectl get issuer vault-keycloak-issuer \
  -n gateway-system
```

Expected:

```text
READY=True
```

```bash
kubectl describe issuer vault-keycloak-issuer \
  -n gateway-system
```

Important expected status:

```text
Type:    Ready
Status:  True
Reason:  VaultVerified
Message: Vault verified
```

### 18.2 Wait for the Certificate

```bash
kubectl wait \
  --for=condition=Ready \
  certificate/auth-ai-platform-local \
  -n gateway-system \
  --timeout=180s
```

Expected:

```text
certificate.cert-manager.io/auth-ai-platform-local condition met
```

Inspect:

```bash
kubectl get certificate auth-ai-platform-local \
  -n gateway-system
```

Expected:

```text
READY=True
SECRET=auth-ai-platform-local-tls
```

---

## 19. Inspect the issued certificate

```bash
kubectl get secret auth-ai-platform-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.tls\.crt}' |
base64 --decode |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -serial \
  -dates \
  -ext subjectAltName
```

Observed certificate characteristics:

```text
subject=CN = auth.ai-platform.local
issuer=CN = AI Platform ModelService Root CA
DNS:auth.ai-platform.local
```

The certificate was issued by the Vault PKI root and matched the external Keycloak hostname.

---

## 20. Validate the Gateway listener

Wait for the Gateway:

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
  -o jsonpath='{range .spec.listeners[*]}name={.name}{"\t"}protocol={.protocol}{"\t"}port={.port}{"\t"}hostname={.hostname}{"\t"}certificate={.tls.certificateRefs[0].name}{"\n"}{end}'
```

Expected important listeners:

```text
name=http
protocol=HTTP
port=80

name=keycloak-https
protocol=HTTPS
port=443
hostname=auth.ai-platform.local
certificate=auth-ai-platform-local-tls
```

Check listener conditions:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.listeners[?(@.name=="keycloak-https")].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Programmed=True
Accepted=True
ResolvedRefs=True
```

---

## 21. Validate the HTTPRoutes

```bash
kubectl get httproute \
  keycloak \
  keycloak-http-redirect \
  -n keycloak
```

Expected hostnames:

```text
auth.ai-platform.local
```

### 21.1 HTTPS route

```bash
kubectl get httproute keycloak \
  -n keycloak \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Accepted=True
ResolvedRefs=True
```

### 21.2 Redirect route

```bash
kubectl get httproute keycloak-http-redirect \
  -n keycloak \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Accepted=True
ResolvedRefs=True
```

---

## 22. Verify the Envoy LoadBalancer ports

```bash
kubectl get service \
  -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=shared-gateway \
  -o wide
```

Expected:

```text
TYPE=LoadBalancer
EXTERNAL-IP=172.19.255.200
PORTS include 80 and 443
```

---

## 23. Export the Vault PKI root CA

Create the protected local directory:

```bash
mkdir -p .local/keycloak
chmod 700 .local/keycloak
```

Export `ca.crt` from the generated TLS Secret:

```bash
kubectl get secret auth-ai-platform-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.ca\.crt}' |
base64 --decode \
  > .local/keycloak/auth-ai-platform-root-ca.crt
```

```bash
chmod 644 \
  .local/keycloak/auth-ai-platform-root-ca.crt
```

Inspect:

```bash
openssl x509 \
  -in .local/keycloak/auth-ai-platform-root-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

Observed:

```text
subject=CN = AI Platform ModelService Root CA
issuer=CN = AI Platform ModelService Root CA
```

This confirms that the file is the self-signed Vault PKI root used to validate the Keycloak certificate.

---

## 24. Validate HTTP and HTTPS manually

Resolve the Gateway address:

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

```bash
echo "${GATEWAY_IP}"
```

Expected:

```text
172.19.255.200
```

### 24.1 HTTP redirect

```bash
curl \
  --silent \
  --show-error \
  --head \
  --resolve "auth.ai-platform.local:80:${GATEWAY_IP}" \
  http://auth.ai-platform.local/
```

Expected:

```text
HTTP/1.1 301 Moved Permanently
location: https://auth.ai-platform.local/
```

### 24.2 Trusted HTTPS

```bash
curl \
  --silent \
  --show-error \
  --head \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/
```

Observed:

```text
HTTP/2 302
location: https://auth.ai-platform.local/admin/
strict-transport-security: max-age=31536000; includeSubDomains
```

A `302` is expected because Keycloak redirects the root path to its administration path.

Do not use `curl -k`. Successful validation must use the trusted Vault PKI root.

---

## 25. Validate OIDC discovery

```bash
curl \
  --silent \
  --show-error \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/realms/master/.well-known/openid-configuration |
jq '{
  issuer,
  authorization_endpoint,
  token_endpoint,
  jwks_uri
}'
```

Expected:

```json
{
  "issuer": "https://auth.ai-platform.local/realms/master",
  "authorization_endpoint": "https://auth.ai-platform.local/realms/master/protocol/openid-connect/auth",
  "token_endpoint": "https://auth.ai-platform.local/realms/master/protocol/openid-connect/token",
  "jwks_uri": "https://auth.ai-platform.local/realms/master/protocol/openid-connect/certs"
}
```

This proves that Keycloak publishes the correct external HTTPS identity even though it receives internal HTTP traffic from Envoy.

---

## 26. Automated HTTPS validation

Create:

```text
infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

```bash
#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
KEYCLOAK_NAMESPACE="${KEYCLOAK_NAMESPACE:-keycloak}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-auth.ai-platform.local}"
CERTIFICATE_NAME="${CERTIFICATE_NAME:-auth-ai-platform-local}"
TLS_SECRET_NAME="${TLS_SECRET_NAME:-auth-ai-platform-local-tls}"
ISSUER_NAME="${ISSUER_NAME:-vault-keycloak-issuer}"
CA_FILE="${CA_FILE:-.local/keycloak/auth-ai-platform-root-ca.crt}"
TIMEOUT="${TIMEOUT:-180s}"

for command_name in kubectl curl openssl jq base64; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "ERROR: Required command not found: ${command_name}" >&2
    exit 1
  fi
done

echo "Checking Vault Keycloak Issuer..."

kubectl wait \
  --for=condition=Ready \
  "issuer/${ISSUER_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking Keycloak Certificate..."

kubectl wait \
  --for=condition=Ready \
  "certificate/${CERTIFICATE_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking shared Gateway..."

kubectl wait \
  --for=condition=Programmed \
  "gateway/${GATEWAY_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking Keycloak routes..."

for route_name in keycloak keycloak-http-redirect; do
  accepted="$(
    kubectl get httproute "${route_name}" \
      --namespace "${KEYCLOAK_NAMESPACE}" \
      --output jsonpath='{.status.parents[0].conditions[?(@.type=="Accepted")].status}'
  )"

  resolved_refs="$(
    kubectl get httproute "${route_name}" \
      --namespace "${KEYCLOAK_NAMESPACE}" \
      --output jsonpath='{.status.parents[0].conditions[?(@.type=="ResolvedRefs")].status}'
  )"

  [[ "${accepted}" == "True" ]] || {
    echo "ERROR: HTTPRoute/${route_name} is not Accepted." >&2
    exit 1
  }

  [[ "${resolved_refs}" == "True" ]] || {
    echo "ERROR: HTTPRoute/${route_name} has unresolved references." >&2
    exit 1
  }
done

echo "Checking Keycloak workload..."

kubectl rollout status \
  deployment/keycloak \
  --namespace "${KEYCLOAK_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking PostgreSQL workload..."

kubectl rollout status \
  statefulset/keycloak-postgres \
  --namespace "${KEYCLOAK_NAMESPACE}" \
  --timeout="${TIMEOUT}"

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

[[ -n "${gateway_ip}" ]] || {
  echo "ERROR: Gateway has no assigned address." >&2
  exit 1
}

mkdir -p "$(dirname "${CA_FILE}")"
chmod 700 "$(dirname "${CA_FILE}")"

kubectl get secret "${TLS_SECRET_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --output jsonpath='{.data.ca\.crt}' |
base64 --decode \
  > "${CA_FILE}"

chmod 644 "${CA_FILE}"

openssl x509 \
  -in "${CA_FILE}" \
  -noout \
  -subject \
  -issuer \
  >/dev/null

echo "Checking HTTP-to-HTTPS redirect..."

redirect_headers="$(
  curl \
    --silent \
    --show-error \
    --head \
    --resolve "${KEYCLOAK_HOSTNAME}:80:${gateway_ip}" \
    "http://${KEYCLOAK_HOSTNAME}/"
)"

http_status="$(
  printf '%s\n' "${redirect_headers}" |
  awk 'NR == 1 {print $2}'
)"

redirect_location="$(
  printf '%s\n' "${redirect_headers}" |
  awk '
    BEGIN {IGNORECASE=1}
    /^location:/ {
      sub(/\r$/, "", $2)
      print $2
      exit
    }
  '
)"

expected_location="https://${KEYCLOAK_HOSTNAME}/"

[[ "${http_status}" == "301" ]] || {
  echo "ERROR: HTTP returned ${http_status}, expected 301." >&2
  exit 1
}

[[ "${redirect_location}" == "${expected_location}" ]] || {
  echo "ERROR: Redirect location is '${redirect_location}'." >&2
  echo "Expected: ${expected_location}" >&2
  exit 1
}

echo "Checking trusted HTTPS..."

https_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${CA_FILE}" \
    --resolve "${KEYCLOAK_HOSTNAME}:443:${gateway_ip}" \
    "https://${KEYCLOAK_HOSTNAME}/"
)"

case "${https_status}" in
  200|302|303)
    ;;
  *)
    echo "ERROR: HTTPS returned ${https_status}." >&2
    exit 1
    ;;
esac

echo "Checking OIDC discovery..."

discovery_document="$(
  curl \
    --silent \
    --show-error \
    --cacert "${CA_FILE}" \
    --resolve "${KEYCLOAK_HOSTNAME}:443:${gateway_ip}" \
    "https://${KEYCLOAK_HOSTNAME}/realms/master/.well-known/openid-configuration"
)"

issuer="$(
  printf '%s' "${discovery_document}" |
  jq -r '.issuer'
)"

expected_issuer="https://${KEYCLOAK_HOSTNAME}/realms/master"

[[ "${issuer}" == "${expected_issuer}" ]] || {
  echo "ERROR: OIDC issuer is '${issuer}'." >&2
  echo "Expected: ${expected_issuer}" >&2
  exit 1
}

echo
echo "PASS: Vault Keycloak Issuer is ready."
echo "PASS: Keycloak certificate is ready."
echo "PASS: Gateway is Programmed."
echo "PASS: Keycloak routes are Accepted and Resolved."
echo "PASS: HTTP redirects to HTTPS."
echo "PASS: Trusted HTTPS reaches Keycloak."
echo "PASS: OIDC discovery publishes the correct external issuer."
```

Make executable and validate:

```bash
chmod +x \
  infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

```bash
bash -n \
  infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

Run:

```bash
KEYCLOAK_HOSTNAME=auth.ai-platform.local \
  infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

---

## 27. Important troubleshooting lesson: do not use `HOSTNAME`

The first version of the validation script used:

```bash
HOSTNAME="${HOSTNAME:-auth.ai-platform.local}"
```

This caused an incorrect validation request because `HOSTNAME` is normally already populated by the shell with the machine hostname, for example:

```text
server
```

The script therefore sent the wrong HTTP Host header and received:

```text
404
```

The infrastructure was working correctly. The script was wrong.

The corrected variable is:

```bash
KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-auth.ai-platform.local}"
```

Use application-specific variable names instead of common shell environment variables.

---

## 28. Troubleshooting

### 28.1 Issuer remains `Ready=False`

Inspect:

```bash
kubectl describe issuer vault-keycloak-issuer \
  -n gateway-system
```

Check cert-manager logs:

```bash
kubectl logs \
  -n cert-manager \
  deployment/cert-manager \
  --since=10m
```

Common causes:

- wrong Vault server CA;
- wrong Vault address;
- incorrect Kubernetes auth mount;
- wrong Vault role name;
- incorrect ServiceAccount audience;
- cert-manager cannot create a token for the issuer ServiceAccount;
- Vault Kubernetes auth cannot reach or validate the Kubernetes API.

### 28.2 Certificate remains `Ready=False`

Inspect:

```bash
kubectl describe certificate auth-ai-platform-local \
  -n gateway-system
```

List CertificateRequests:

```bash
kubectl get certificaterequest \
  -n gateway-system
```

Inspect the newest request:

```bash
kubectl describe certificaterequest \
  -n gateway-system \
  <certificate-request-name>
```

Common causes:

- Vault policy does not permit `pki_modelservice/sign/keycloak`;
- requested hostname is not allowed by the PKI role;
- duration exceeds `max_ttl`;
- unsupported key usage;
- issuer authentication failure.

### 28.3 Gateway listener has `ResolvedRefs=False`

Check that the TLS Secret exists in the same namespace as the Gateway:

```bash
kubectl get secret auth-ai-platform-local-tls \
  -n gateway-system
```

Inspect listener conditions:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o yaml
```

### 28.4 HTTPRoute is not accepted

Check:

- the route namespace has `shared-gateway-access=true`;
- `parentRefs.namespace` is `gateway-system`;
- `parentRefs.name` is `shared-gateway`;
- `sectionName` is `keycloak-https`;
- the listener hostname matches `auth.ai-platform.local`.

### 28.5 HTTPS connection reset

A connection reset commonly means there is no matching HTTPS listener for the TLS SNI hostname.

Verify:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .spec.listeners[*]}{.name}{"\t"}{.hostname}{"\t"}{.port}{"\n"}{end}'
```

The Gateway must include:

```text
keycloak-https  auth.ai-platform.local  443
```

### 28.6 Certificate hostname mismatch

Inspect the served certificate:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername auth.ai-platform.local \
  -CAfile .local/keycloak/auth-ai-platform-root-ca.crt \
  -verify_return_error \
  </dev/null 2>/dev/null |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -ext subjectAltName
```

Expected:

```text
subject=CN = auth.ai-platform.local
DNS:auth.ai-platform.local
```

### 28.7 OIDC discovery publishes HTTP or an internal hostname

Inspect the Keycloak environment:

```bash
kubectl get deployment keycloak \
  -n keycloak \
  -o json |
jq '.spec.template.spec.containers[0].env'
```

Confirm:

```text
KC_HOSTNAME=https://auth.ai-platform.local
KC_HTTP_ENABLED=true
KC_PROXY_HEADERS=xforwarded
```

The external URL must be HTTPS even though Envoy communicates with Keycloak over internal HTTP.

### 28.8 NetworkPolicy blocks Keycloak

Inspect:

```bash
kubectl get networkpolicy \
  -n keycloak
```

Verify Envoy Pod labels:

```bash
kubectl get pods \
  -n envoy-gateway-system \
  --show-labels
```

The NetworkPolicy selectors must match the actual Envoy data-plane labels.

---

## 29. Security considerations

### TLS termination

TLS terminates at Envoy Gateway. Keycloak receives HTTP on port `8080` inside the cluster.

### External issuer consistency

Keycloak must publish the external HTTPS hostname in OIDC metadata and tokens.

### Vault least privilege

The dedicated Vault policy permits only issuance through the Keycloak PKI role.

### Short-lived Kubernetes authentication

cert-manager requests a short-lived Kubernetes token for the dedicated issuer ServiceAccount.

### No static Vault token in Kubernetes

The Issuer uses Vault Kubernetes authentication rather than a long-lived Vault token stored in a Kubernetes Secret.

### Private-key storage

The TLS private key is stored in:

```text
gateway-system/auth-ai-platform-local-tls
```

Access to the `gateway-system` namespace and Secrets should be tightly restricted.

### HTTP redirect

Plain HTTP never forwards to Keycloak. It only redirects to HTTPS.

### Trusted validation

Validation uses the Vault PKI root CA. `curl -k` is not part of the accepted validation process.

---

## 30. Git safety

The following files are safe to commit:

```text
config/platform/keycloak/tls/*.yaml
config/platform/keycloak/keycloak-httproute.yaml
config/platform/keycloak/keycloak-http-redirect.yaml
config/platform/keycloak/networkpolicy.yaml
config/platform/shared-gateway.yaml
infrastructure/keycloak/policies/cert-manager-keycloak-pki.hcl
infrastructure/keycloak/scripts/configure-keycloak-vault-pki.sh
infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

Do not commit:

```text
Vault tokens
Vault unseal keys
Vault private keys
.local/keycloak/
TLS private keys exported from Kubernetes
real client secrets
real user passwords
```

Validate ignored local files:

```bash
git check-ignore -v \
  .local/keycloak/auth-ai-platform-root-ca.crt
```

Confirm no local certificate or secret material is tracked:

```bash
git ls-files |
grep -E \
  '(^|/)\.local/|tls\.key$|BEGIN PRIVATE KEY' &&
{
  echo "ERROR: Sensitive TLS material is tracked"
  exit 1
} || echo "PASS: No sensitive TLS material is tracked"
```

---

## 31. Completion criteria

This phase is complete when all of the following are true:

```text
[✓] Dedicated Vault PKI role created for Keycloak
[✓] Dedicated Vault policy created
[✓] Dedicated Vault Kubernetes auth role created
[✓] cert-manager issuer ServiceAccount created
[✓] TokenRequest RBAC restricted to the issuer ServiceAccount
[✓] Vault Issuer Ready=True
[✓] Keycloak Certificate Ready=True
[✓] Certificate SAN contains auth.ai-platform.local
[✓] Gateway keycloak-https listener Programmed=True
[✓] HTTPS HTTPRoute Accepted=True
[✓] Redirect HTTPRoute Accepted=True
[✓] HTTP returns 301 to HTTPS
[✓] Trusted HTTPS reaches Keycloak
[✓] OIDC discovery publishes the correct external issuer
[✓] Keycloak ingress restricted by NetworkPolicy
[✓] PostgreSQL ingress restricted to Keycloak
[✓] Automated HTTPS validation passes
[✓] No private keys, tokens or credentials committed
```

---

## 32. Final architecture after this phase

```text
Client
  │
  ├── HTTP :80
  │     ↓
  │   HTTPRoute/keycloak-http-redirect
  │     ↓ 301
  │
  └── HTTPS :443
        ↓
      MetalLB 172.19.255.200
        ↓
      Gateway/shared-gateway
        ↓ listener: keycloak-https
      Secret/auth-ai-platform-local-tls
        ↑
      Certificate/auth-ai-platform-local
        ↑
      Issuer/vault-keycloak-issuer
        ↑ Kubernetes auth
      Vault auth/kubernetes-kind
        ↓
      Vault PKI role: keycloak
        ↓
      HTTPRoute/keycloak
        ↓
      Service/keycloak:8080
        ↓
      Keycloak Pod
        ↓
      PostgreSQL StatefulSet
```

---

## 33. Next phase

The next phase creates the Keycloak application realm and OIDC clients:

```text
Realm:
  ai-platform

Clients:
  ai-platform-gateway
  ai-platform-cli
  ai-platform-service
```

See:

```text
04-keycloak-realm-and-clients.md
```
