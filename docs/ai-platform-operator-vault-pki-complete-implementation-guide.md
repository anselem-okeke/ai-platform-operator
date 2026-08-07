# AI Platform Operator — Complete Vault PKI Implementation Guide

## External HashiCorp Vault, cert-manager Kubernetes authentication, Vault PKI issuance, durable TokenReview identity, renewal, and private-key rotation

**Project:** `ai-platform-operator`  
**Repository:** `/mnt/data/ai-platform-operator`  
**Implementation date:** 2026-08-03  
**Kubernetes context:** `kind-ai-platform-policy`  
**Vault server:** `https://vault.platform.local:8200`  
**Vault VM address:** `192.168.0.61`  
**Ansible VM address:** `192.168.0.58`  
**Application hostname:** `fraud-model.local`  
**Gateway namespace:** `gateway-system`  
**cert-manager namespace:** `cert-manager`  
**ModelService namespace:** `ai-platform`  
**Gateway:** `shared-gateway`  
**TLS Secret:** `fraud-model-local-tls`

---

# 1. Purpose

This document records the complete Vault PKI implementation performed for the AI Platform Operator environment.

It starts from the working pre-Vault state:

```text
Certificate/fraud-model-local
        ↓
Issuer/development-ca
        ↓
cert-manager development root CA
        ↓
Secret/fraud-model-local-tls
        ↓
Envoy Gateway
```

It finishes with:

```text
Certificate/fraud-model-local
        ↓
Issuer/vault-issuer
        ↓
short-lived Kubernetes ServiceAccount JWT
        ↓
Vault auth mount: kubernetes-kind/
        ↓
Vault role: cert-manager-modelservice
        ↓
Vault policy: cert-manager-modelservice-pki
        ↓
Vault PKI signing endpoint:
pki_modelservice/sign/modelservice
        ↓
Secret/fraud-model-local-tls
        ↓
Envoy Gateway
```

The implementation includes:

1. validating Vault health and TLS;
2. fixing Pod-to-Vault networking;
3. configuring Pod DNS for `vault.platform.local`;
4. distributing the Vault server CA;
5. proving trusted HTTPS from a Kubernetes Pod;
6. storing all Kubernetes resources as code;
7. creating cert-manager TokenRequest RBAC;
8. creating a dedicated Vault Kubernetes auth mount for kind;
9. exposing the kind Kubernetes API securely to Vault;
10. creating a Kubernetes TokenReview identity;
11. resolving JWT audience failures;
12. creating the Vault PKI engine;
13. creating the Vault signing role and policy;
14. creating the cert-manager Vault Issuer;
15. switching the existing Certificate from the development CA to Vault;
16. validating issuance;
17. validating forced renewal;
18. validating private-key rotation;
19. validating Gateway continuity;
20. replacing the temporary reviewer token with a durable declarative token Secret;
21. creating reproducible Vault setup and validation scripts;
22. documenting exact failures and corrections.

This document stops before OIDC/JWT authentication for end users.

---

# 2. Final architecture

```text
cert-manager controller
namespace: cert-manager
        │
        │ TokenRequest
        ▼
ServiceAccount/cert-manager-vault-issuer
namespace: gateway-system
        │
        │ short-lived JWT
        │ audiences:
        │ - vault://gateway-system/vault-issuer
        │ - https://kubernetes.default.svc.cluster.local
        ▼
Vault
https://vault.platform.local:8200
        │
        │ auth/kubernetes-kind/login
        ▼
Vault Kubernetes auth backend
mount: kubernetes-kind/
        │
        │ TokenReview
        ▼
kind Kubernetes API
https://kubernetes:6444
        │
        │ reviewer identity:
        │ ServiceAccount/vault-token-reviewer
        ▼
Vault validates:
- ServiceAccount name
- namespace
- audience
        │
        ▼
Vault token with policy:
cert-manager-modelservice-pki
        │
        ▼
pki_modelservice/sign/modelservice
        │
        ▼
CertificateRequest/fraud-model-local-*
        │
        ▼
Secret/fraud-model-local-tls
        │
        ▼
Gateway/shared-gateway
        │
        ▼
HTTPS fraud-model.local
```

---

# 3. Responsibility split

## 3.1 ModelService operator

The operator continues to manage:

- Deployment;
- Service;
- ServiceAccount;
- PersistentVolumeClaim;
- PodDisruptionBudget;
- NetworkPolicy;
- application HTTPRoute;
- workload status;
- route drift correction.

The operator does not authenticate to Vault and does not create certificates.

## 3.2 cert-manager

cert-manager manages:

- Kubernetes `Certificate`;
- `CertificateRequest`;
- private-key generation;
- Vault authentication;
- certificate issuance;
- renewal scheduling;
- Kubernetes TLS Secret updates;
- private-key rotation.

## 3.3 Vault

Vault manages:

- the private certificate authority;
- PKI signing policy;
- signing role restrictions;
- Kubernetes JWT authentication;
- Vault client-token issuance;
- certificate signing.

## 3.4 Shared Gateway

The Gateway consumes:

```text
Secret/gateway-system/fraud-model-local-tls
```

The Gateway does not care whether that Secret was created by:

- the development CA;
- Vault PKI;
- ACME;
- a corporate CA;
- another certificate provider.

This provider-neutral Secret contract is why Vault could replace the development issuer without changing the Gateway or ModelService operator.

---

# 4. Environment used

| Component | Value |
|---|---|
| Project directory | `/mnt/data/ai-platform-operator` |
| Kubernetes context | `kind-ai-platform-policy` |
| Kubernetes version | `v1.36.1` |
| Pod CIDR | `10.244.0.0/16` |
| Service CIDR | `10.96.0.0/16` |
| Kubernetes API host port | `6444` |
| Ansible VM LAN IP | `192.168.0.58` |
| Jumpbox IP | `192.168.0.28` |
| Vault VM IP | `192.168.0.61` |
| Vault DNS name | `vault.platform.local` |
| Vault port | `8200` |
| Vault version | `2.0.3` |
| Vault storage | Raft |
| Vault seal type | Shamir |
| Existing Talos auth mount | `kubernetes/` |
| New kind auth mount | `kubernetes-kind/` |
| Vault PKI mount | `pki_modelservice/` |
| Vault PKI role | `modelservice` |
| Vault auth role | `cert-manager-modelservice` |
| Vault policy | `cert-manager-modelservice-pki` |
| cert-manager Issuer | `gateway-system/vault-issuer` |
| Certificate | `gateway-system/fraud-model-local` |
| TLS Secret | `gateway-system/fraud-model-local-tls` |
| Gateway | `gateway-system/shared-gateway` |
| Gateway address | `172.19.255.200` |
| Application hostname | `fraud-model.local` |

---

# 5. Starting state

Before beginning Vault PKI, these resources were healthy:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='issuerName={.spec.issuerRef.name}{"\n"}issuerKind={.spec.issuerRef.kind}{"\n"}ready={.status.conditions[?(@.type=="Ready")].status}{"\n"}'
```

Initial expected state:

```text
issuerName=development-ca
issuerKind=Issuer
ready=True
```

Gateway:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide
```

Expected:

```text
shared-gateway   envoy   172.19.255.200   True
```

This development certificate remained active until the Vault Issuer was fully ready.

---

# 6. Validate Vault health

Run on the jumpbox:

```bash
export VAULT_ADDR="https://vault.platform.local:8200"
export VAULT_CACERT="/home/jumpbox/.vault-tls/vault-ca.crt"
```

Verify:

```bash
vault status
```

Validated Vault state:

```text
Initialized: true
Sealed:      false
HA Enabled:  true
Active:      true
Storage:     raft
Version:     2.0.3
```

List auth methods:

```bash
vault auth list
```

Initial result included:

```text
kubernetes/
token/
```

The existing `kubernetes/` mount was already configured for another Kubernetes cluster, using an API endpoint such as:

```text
https://192.168.0.210:6443
```

It was not reused for kind.

**Design decision:** create a separate auth mount:

```text
kubernetes-kind/
```

This prevents changing or breaking the existing Talos/ESO integration.

---

# 7. Validate the Vault server certificate

Run on a host that can reach Vault:

```bash
openssl s_client \
  -connect vault.platform.local:8200 \
  -servername vault.platform.local \
  -CAfile /home/jumpbox/.vault-tls/vault-ca.crt \
  -verify_return_error \
  </dev/null
```

Validated server certificate:

```text
subject: CN=vault.platform.local
issuer:  CN=platform-internal-ca
SAN:     vault.platform.local
valid:   Jun 17 2026 to Sep 19 2028
```

Health endpoint:

```bash
curl \
  --cacert /home/jumpbox/.vault-tls/vault-ca.crt \
  https://vault.platform.local:8200/v1/sys/health
```

Expected:

```json
{
  "initialized": true,
  "sealed": false,
  "standby": false
}
```

Do not use `curl -k` as proof of correct trust.

---

# 8. Fix Pod-to-Vault network connectivity

## 8.1 Original problem

The earlier kind Pod CIDR overlapped with the LAN:

```text
Pod CIDR: 192.168.0.0/16
Vault IP: 192.168.0.61
```

Pods treated the Vault address as part of the Pod network.

## 8.2 Corrected kind network

The cluster was recreated with:

```yaml
networking:
  disableDefaultCNI: true
  podSubnet: 10.244.0.0/16
  serviceSubnet: 10.96.0.0/16
  apiServerAddress: "0.0.0.0"
  apiServerPort: 6444
```

Validated file:

```text
kind-calico-config.yaml
```

After recreation and Calico installation, Pods received addresses such as:

```text
10.244.42.x
10.244.82.x
```

## 8.3 Test direct connectivity

Example:

```bash
kubectl run vault-ip-connectivity-test \
  -n default \
  --image=curlimages/curl:latest \
  --restart=Never \
  --command -- \
  curl -skv \
  --connect-timeout 10 \
  https://192.168.0.61:8200/v1/sys/health
```

Wait and inspect:

```bash
kubectl wait \
  -n default \
  --for=jsonpath='{.status.phase}'=Succeeded \
  pod/vault-ip-connectivity-test \
  --timeout=60s

kubectl logs \
  -n default \
  vault-ip-connectivity-test
```

Validated:

```text
HTTP/2 200
initialized=true
sealed=false
standby=false
```

Clean up:

```bash
kubectl delete pod vault-ip-connectivity-test \
  -n default
```

---

# 9. Configure cluster DNS for Vault

Pods initially needed to resolve:

```text
vault.platform.local → 192.168.0.61
```

## 9.1 Back up CoreDNS

```bash
cd /mnt/data/ai-platform-operator

kubectl get configmap coredns \
  -n kube-system \
  -o yaml \
  > .local/cluster-backup/coredns-before-vault.yaml
```

## 9.2 Edit CoreDNS

```bash
kubectl edit configmap coredns \
  -n kube-system
```

Add this block to the `Corefile`, before the normal `forward` directive:

```text
hosts {
    192.168.0.61 vault.platform.local
    fallthrough
}
```

Conceptual Corefile section:

```text
.:53 {
    errors
    health {
       lameduck 5s
    }
    ready

    hosts {
        192.168.0.61 vault.platform.local
        fallthrough
    }

    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
       ttl 30
    }

    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
```

## 9.3 Restart CoreDNS

```bash
kubectl rollout restart deployment/coredns \
  -n kube-system

kubectl rollout status deployment/coredns \
  -n kube-system \
  --timeout=120s
```

## 9.4 Validate DNS

```bash
kubectl run vault-dns-test \
  -n default \
  --image=busybox:1.36 \
  --restart=Never \
  --command -- \
  nslookup vault.platform.local
```

Inspect:

```bash
kubectl logs \
  -n default \
  vault-dns-test
```

Validated result:

```text
Server:  10.96.0.10
Name:    vault.platform.local
Address: 192.168.0.61
```

Clean up:

```bash
kubectl delete pod vault-dns-test \
  -n default
```

---

# 10. Import the Vault server CA into the project

The public CA certificate was copied to the Ansible VM and stored locally at:

```text
.local/tls/vault-server-ca.crt
```

Create the local directory:

```bash
mkdir -p .local/tls
chmod 700 .local/tls
```

Verify the certificate:

```bash
openssl x509 \
  -in .local/tls/vault-server-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

This public CA may be represented in a Kubernetes manifest. The private CA key must never be copied or committed.

---

# 11. Validate trusted Vault HTTPS from a Pod

Create a temporary ConfigMap:

```bash
kubectl create configmap vault-server-ca \
  -n default \
  --from-file=ca.crt=.local/tls/vault-server-ca.crt \
  --dry-run=client \
  -o yaml |
kubectl apply -f -
```

Run a Pod that mounts the CA:

```bash
kubectl run vault-trusted-connectivity-test \
  -n default \
  --image=curlimages/curl:latest \
  --restart=Never \
  --overrides='
{
  "spec": {
    "containers": [
      {
        "name": "vault-trusted-connectivity-test",
        "image": "curlimages/curl:latest",
        "command": [
          "sh",
          "-c",
          "curl -sSv --cacert /vault-ca/ca.crt --connect-timeout 10 https://vault.platform.local:8200/v1/sys/health"
        ],
        "volumeMounts": [
          {
            "name": "vault-ca",
            "mountPath": "/vault-ca",
            "readOnly": true
          }
        ]
      }
    ],
    "volumes": [
      {
        "name": "vault-ca",
        "configMap": {
          "name": "vault-server-ca"
        }
      }
    ]
  }
}'
```

Wait:

```bash
kubectl wait \
  -n default \
  --for=jsonpath='{.status.phase}'=Succeeded \
  pod/vault-trusted-connectivity-test \
  --timeout=60s
```

Inspect:

```bash
kubectl logs \
  -n default \
  vault-trusted-connectivity-test
```

Validated TLS evidence:

```text
Host vault.platform.local resolved to 192.168.0.61
subject: CN=vault.platform.local
issuer: CN=platform-internal-ca
subjectAltName matched
OpenSSL verify result: 0
SSL certificate verified
HTTP/2 200
```

Clean up the temporary resources:

```bash
kubectl delete pod vault-trusted-connectivity-test \
  -n default

kubectl delete configmap vault-server-ca \
  -n default
```

---

# 12. Repository structure for Vault PKI

Create:

```bash
mkdir -p \
  config/platform/vault \
  infrastructure/vault/scripts \
  infrastructure/vault/policies
```

Final logical structure:

```text
config/platform/vault/
├── cert-manager-vault-serviceaccount.yaml
├── cert-manager-vault-token-rbac.yaml
├── vault-server-ca-secret.yaml
├── vault-token-reviewer.yaml
├── vault-token-reviewer-secret.yaml
├── vault-issuer.yaml
├── fraud-model-vault-certificate.yaml
└── kustomization.yaml

infrastructure/vault/
├── README.md
├── variables.env.example
├── policies/
│   └── cert-manager-modelservice-pki.hcl
└── scripts/
    ├── export-kind-auth-material.sh
    ├── configure-kind-auth.sh
    ├── configure-modelservice-pki.sh
    ├── validate-vault-pki-integration.sh
    └── validate-certificate-rotation.sh
```

---

# 13. Create the cert-manager Vault ServiceAccount

File:

```text
config/platform/vault/cert-manager-vault-serviceaccount.yaml
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cert-manager-vault-issuer
  namespace: gateway-system
automountServiceAccountToken: false
```

Why:

- cert-manager requests a short-lived token only when needed;
- no default token is mounted into a Pod;
- the identity is dedicated to the Vault Issuer;
- the ServiceAccount is namespaced with the Issuer.

---

# 14. Create TokenRequest and TokenReview RBAC

File:

```text
config/platform/vault/cert-manager-vault-token-rbac.yaml
```

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cert-manager-vault-tokenrequest
  namespace: gateway-system
rules:
  - apiGroups: [""]
    resources: ["serviceaccounts/token"]
    resourceNames: ["cert-manager-vault-issuer"]
    verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: cert-manager-vault-tokenrequest
  namespace: gateway-system
subjects:
  - kind: ServiceAccount
    name: cert-manager
    namespace: cert-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: cert-manager-vault-tokenrequest
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: cert-manager-vault-issuer-auth-delegator
subjects:
  - kind: ServiceAccount
    name: cert-manager-vault-issuer
    namespace: gateway-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:auth-delegator
```

Apply the safe prerequisites:

```bash
kubectl apply \
  -f config/platform/vault/cert-manager-vault-serviceaccount.yaml

kubectl apply \
  -f config/platform/vault/cert-manager-vault-token-rbac.yaml
```

Verify:

```bash
kubectl get serviceaccount cert-manager-vault-issuer \
  -n gateway-system
```

Check TokenRequest permission:

```bash
kubectl auth can-i create \
  serviceaccounts/cert-manager-vault-issuer \
  --subresource=token \
  --as=system:serviceaccount:cert-manager:cert-manager \
  -n gateway-system
```

Expected:

```text
yes
```

Note: the local `kubectl` did not support the attempted `--resource-name` flag. The working form used `resource/name` with `--subresource=token`.

---

# 15. Create the Vault server CA Secret as code

File:

```text
config/platform/vault/vault-server-ca-secret.yaml
```

Use Kustomize `secretGenerator` rather than committing a manually encoded value.

Example `kustomization.yaml` fragment:

```yaml
secretGenerator:
  - name: vault-server-ca
    namespace: gateway-system
    files:
      - ca.crt=../../../.local/tls/vault-server-ca.crt

generatorOptions:
  disableNameSuffixHash: true
```

Important:

- the certificate is public;
- `.local/tls/vault-server-ca.crt` must exist before rendering;
- the generated Secret name must remain exactly `vault-server-ca`;
- the Vault Issuer reads `ca.crt`.

Rendered result:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vault-server-ca
  namespace: gateway-system
type: Opaque
data:
  ca.crt: <base64-encoded-public-CA>
```

Do not commit a private key.

---

# 16. Create the Vault Issuer manifest

File:

```text
config/platform/vault/vault-issuer.yaml
```

Final working form:

```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: vault-issuer
  namespace: gateway-system
spec:
  vault:
    server: https://vault.platform.local:8200
    path: pki_modelservice/sign/modelservice

    caBundleSecretRef:
      name: vault-server-ca
      key: ca.crt

    auth:
      kubernetes:
        mountPath: /v1/auth/kubernetes-kind
        role: cert-manager-modelservice
        serviceAccountRef:
          name: cert-manager-vault-issuer
          audiences:
            - https://kubernetes.default.svc.cluster.local
```

cert-manager adds its generated Vault audience for the Issuer. The explicit extra audience allows Kubernetes TokenReview to accept the token.

Effective audiences:

```text
vault://gateway-system/vault-issuer
https://kubernetes.default.svc.cluster.local
```

---

# 17. Create the Vault-backed Certificate manifest

File:

```text
config/platform/vault/fraud-model-vault-certificate.yaml
```

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
    name: vault-issuer
    kind: Issuer
    group: cert-manager.io
```

The Certificate name and Secret name are unchanged from the development-CA implementation.

---

# 18. Create Kustomize management

File:

```text
config/platform/vault/kustomization.yaml
```

Final example:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - cert-manager-vault-serviceaccount.yaml
  - cert-manager-vault-token-rbac.yaml
  - vault-token-reviewer.yaml
  - vault-token-reviewer-secret.yaml
  - vault-issuer.yaml

secretGenerator:
  - name: vault-server-ca
    namespace: gateway-system
    files:
      - ca.crt=../../../.local/tls/vault-server-ca.crt

generatorOptions:
  disableNameSuffixHash: true
```

The Vault-backed Certificate may be kept outside the initial Kustomization so the issuer can be proven ready before switching production certificate issuance:

```text
config/platform/vault/fraud-model-vault-certificate.yaml
```

Render before applying:

```bash
kubectl kustomize config/platform/vault
```

Review:

- namespaces;
- ServiceAccount names;
- RBAC subjects;
- CA Secret;
- auth mount;
- Vault role;
- signing path.

---

# 19. Initial application behavior

The complete Kustomization was applied before Vault was ready:

```bash
kubectl apply -k config/platform/vault
```

This created:

```text
ServiceAccount/cert-manager-vault-issuer
Role/cert-manager-vault-tokenrequest
RoleBinding/cert-manager-vault-tokenrequest
ClusterRoleBinding/cert-manager-vault-issuer-auth-delegator
Secret/vault-server-ca
Issuer/vault-issuer
```

The `vault-issuer` became:

```text
Ready=False
```

Initial error:

```text
POST https://vault.platform.local:8200/v1/auth/kubernetes-kind/login
Code: 403
permission denied
```

This did not affect the active development certificate:

```text
issuerName=development-ca
ready=True
Gateway Programmed=True
```

The Issuer object was allowed to remain. cert-manager retried automatically while Vault was configured.

---

# 20. Export the kind Kubernetes CA

Create:

```text
infrastructure/vault/scripts/export-kind-auth-material.sh
```

Example:

```bash
#!/usr/bin/env bash
set -euo pipefail

CONTEXT="${CONTEXT:-kind-ai-platform-policy}"
OUTPUT_DIR="${OUTPUT_DIR:-.local/vault-auth}"
CA_FILE="${OUTPUT_DIR}/kind-ai-platform-policy-ca.crt"

mkdir -p "${OUTPUT_DIR}"
chmod 700 "${OUTPUT_DIR}"

kubectl config view \
  --context "${CONTEXT}" \
  --raw \
  --minify \
  -o jsonpath='{.clusters[0].cluster.certificate-authority-data}' |
base64 --decode \
  > "${CA_FILE}"

chmod 644 "${CA_FILE}"

openssl x509 \
  -in "${CA_FILE}" \
  -noout \
  -subject \
  -issuer \
  -dates

echo "Exported Kubernetes CA to ${CA_FILE}"
```

Run:

```bash
chmod +x infrastructure/vault/scripts/*.sh

infrastructure/vault/scripts/export-kind-auth-material.sh
```

Validated file:

```text
.local/vault-auth/kind-ai-platform-policy-ca.crt
```

Copy it to the jumpbox:

```bash
scp \
  .local/vault-auth/kind-ai-platform-policy-ca.crt \
  jumpbox@jumpbox:/home/jumpbox/kind-ai-platform-policy-ca.crt
```

Verify on the jumpbox:

```bash
openssl x509 \
  -in /home/jumpbox/kind-ai-platform-policy-ca.crt \
  -noout \
  -subject \
  -issuer \
  -dates
```

---

# 21. Expose the kind API to Vault

## 21.1 Confirm host binding

On the Ansible VM:

```bash
kubectl config view --minify \
  -o jsonpath='{.clusters[0].cluster.server}{"\n"}'

sudo ss -lntp |
grep 6444

docker ps \
  --format 'table {{.Names}}\t{{.Ports}}' |
grep ai-platform-policy-control-plane
```

Validated:

```text
https://0.0.0.0:6444
0.0.0.0:6444 LISTEN
0.0.0.0:6444->6443/tcp
```

This confirms that the API is reachable through the Ansible VM LAN address.

## 21.2 Determine the Ansible VM address

```bash
ANSIBLE_VM_IP=$(
  ip route get 192.168.0.61 |
  sed -n 's/.* src \([^ ]*\).*/\1/p'
)

echo "$ANSIBLE_VM_IP"
```

Validated:

```text
192.168.0.58
```

## 21.3 Initial IP test

From the jumpbox:

```bash
curl \
  --cacert /home/jumpbox/kind-ai-platform-policy-ca.crt \
  https://192.168.0.58:6444/version
```

Network connectivity worked, but TLS failed:

```text
SSL: no alternative certificate subject name matches target host name '192.168.0.58'
```

The CA was correct; the IP was not a server-certificate SAN.

## 21.4 Inspect API server SANs

On the Ansible VM:

```bash
echo |
openssl s_client \
  -connect 127.0.0.1:6444 \
  -servername kubernetes \
  2>/dev/null |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -ext subjectAltName
```

Validated SANs included:

```text
DNS:ai-platform-policy-control-plane
DNS:host.docker.internal
DNS:kubernetes
DNS:kubernetes.default
DNS:kubernetes.default.svc
DNS:kubernetes.default.svc.cluster.local
DNS:localhost
IP Address:10.96.0.1
IP Address:172.19.0.7
IP Address:0.0.0.0
IP Address:172.17.0.1
IP Address:127.0.0.1
```

## 21.5 Map the certificate-valid hostname

On the jumpbox:

```bash
echo '192.168.0.58 kubernetes' |
sudo tee -a /etc/hosts
```

Verify:

```bash
getent hosts kubernetes
```

Expected:

```text
192.168.0.58 kubernetes
```

Test:

```bash
curl \
  --cacert /home/jumpbox/kind-ai-platform-policy-ca.crt \
  https://kubernetes:6444/version
```

Validated response:

```json
{
  "major": "1",
  "minor": "36",
  "gitVersion": "v1.36.1"
}
```

## 21.6 Add the same mapping on the Vault VM

The important correction was that the Vault server itself performs TokenReview. A mapping only on the jumpbox is not sufficient.

On the Vault VM:

```bash
echo '192.168.0.58 kubernetes' |
sudo tee -a /etc/hosts
```

Verify:

```bash
getent hosts kubernetes
```

Connectivity check:

```bash
curl -k \
  https://kubernetes:6444/version
```

`-k` was acceptable only for this narrow connectivity test on the Vault VM when the Kubernetes CA was not installed there. Vault itself was configured with the correct Kubernetes CA.

Final API endpoint used by Vault:

```text
https://kubernetes:6444
```

---

# 22. Create Vault environment variables

File:

```text
infrastructure/vault/variables.env.example
```

Final example:

```bash
VAULT_ADDR=https://vault.platform.local:8200
VAULT_CACERT=/home/jumpbox/.vault-tls/vault-ca.crt

VAULT_K8S_AUTH_PATH=kubernetes-kind
VAULT_K8S_ROLE=cert-manager-modelservice
VAULT_POLICY=cert-manager-modelservice-pki

VAULT_PKI_PATH=pki_modelservice
VAULT_PKI_ROLE=modelservice

KUBERNETES_HOST=https://kubernetes:6444
KUBERNETES_CA_FILE=/home/jumpbox/kind-ai-platform-policy-ca.crt

TOKEN_AUDIENCE=https://kubernetes.default.svc.cluster.local

# Temporary file used only while writing the reviewer JWT into Vault.
TOKEN_REVIEWER_JWT_FILE=/tmp/kind-token-reviewer.jwt
```

Create the local environment:

```bash
cp infrastructure/vault/variables.env.example \
  infrastructure/vault/variables.env
```

Edit local values when necessary:

```bash
nano infrastructure/vault/variables.env
```

The real `variables.env` may contain environment-specific paths and must not contain Vault root tokens or reviewer JWT values.

Load:

```bash
set -a
source infrastructure/vault/variables.env
set +a
```

Verify non-secret values:

```bash
printf '%s\n' \
  "$VAULT_ADDR" \
  "$KUBERNETES_HOST" \
  "$VAULT_K8S_AUTH_PATH" \
  "$VAULT_PKI_PATH"
```

Expected:

```text
https://vault.platform.local:8200
https://kubernetes:6444
kubernetes-kind
pki_modelservice
```

---

# 23. Create the Vault PKI policy

File:

```text
infrastructure/vault/policies/cert-manager-modelservice-pki.hcl
```

```hcl
path "pki_modelservice/sign/modelservice" {
  capabilities = ["create", "update"]
}

path "pki_modelservice/issue/modelservice" {
  capabilities = ["create", "update"]
}
```

The policy grants only signing/issuance through the restricted role.

It does not grant:

- PKI administration;
- root generation;
- issuer deletion;
- policy administration;
- auth administration;
- arbitrary Vault access.

---

# 24. Configure the Vault PKI engine

File:

```text
infrastructure/vault/scripts/configure-modelservice-pki.sh
```

Reproducible example:

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${VAULT_ADDR:?Set VAULT_ADDR}"
: "${VAULT_CACERT:?Set VAULT_CACERT}"

VAULT_PKI_PATH="${VAULT_PKI_PATH:-pki_modelservice}"
VAULT_PKI_ROLE="${VAULT_PKI_ROLE:-modelservice}"
VAULT_POLICY="${VAULT_POLICY:-cert-manager-modelservice-pki}"

SCRIPT_DIR="$(
  cd "$(dirname "${BASH_SOURCE[0]}")" &&
  pwd
)"

POLICY_FILE="$(
  cd "${SCRIPT_DIR}/../policies" &&
  pwd
)/cert-manager-modelservice-pki.hcl"

if ! vault secrets list -format=json |
  jq -e --arg path "${VAULT_PKI_PATH}/" 'has($path)' >/dev/null
then
  vault secrets enable \
    -path="${VAULT_PKI_PATH}" \
    pki
fi

vault secrets tune \
  -max-lease-ttl=87600h \
  "${VAULT_PKI_PATH}"

if ! vault read \
  "${VAULT_PKI_PATH}/issuer/default" \
  >/dev/null 2>&1
then
  vault write \
    "${VAULT_PKI_PATH}/root/generate/internal" \
    common_name="AI Platform ModelService Root CA" \
    ttl=87600h \
    key_type=ec \
    key_bits=256
fi

vault write \
  "${VAULT_PKI_PATH}/roles/${VAULT_PKI_ROLE}" \
  allowed_domains="fraud-model.local" \
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

vault policy write \
  "${VAULT_POLICY}" \
  "${POLICY_FILE}"

echo
echo "Vault ModelService PKI configured."
echo "PKI mount: ${VAULT_PKI_PATH}/"
echo "PKI role:  ${VAULT_PKI_ROLE}"
echo "Policy:    ${VAULT_POLICY}"
```

The observed role output showed some defaults such as `allow_ip_sans=true`, `allow_localhost=true`, `client_flag=true`, and `allow_wildcard_certificates=true`. For a hardened final implementation, explicitly set the desired values shown above and then verify the live role.

Run on the jumpbox:

```bash
chmod +x infrastructure/vault/scripts/*.sh

infrastructure/vault/scripts/configure-modelservice-pki.sh
```

Validated output included:

```text
Success! Enabled the pki secrets engine at: pki_modelservice/
Success! Tuned the secrets engine at: pki_modelservice/
Success! Uploaded policy: cert-manager-modelservice-pki
```

Verify:

```bash
vault secrets list
```

Expected:

```text
pki_modelservice/   pki
```

Read role:

```bash
vault read pki_modelservice/roles/modelservice
```

Important values:

```text
allowed_domains     [fraud-model.local]
allow_bare_domains  true
allow_subdomains    false
max_ttl             720h
key_type            ec
key_bits            256
```

Read policy:

```bash
vault policy read cert-manager-modelservice-pki
```

---

# 25. Create the dedicated kind Kubernetes auth mount

The existing `kubernetes/` mount was preserved.

The new mount is:

```text
kubernetes-kind/
```

Initial activation:

```bash
vault auth enable \
  -path=kubernetes-kind \
  kubernetes
```

Verify:

```bash
vault auth list
```

Expected:

```text
kubernetes-kind/    kubernetes
kubernetes/         kubernetes
token/              token
```

---

# 26. Create the initial Vault auth role

Initial role:

```bash
vault write auth/kubernetes-kind/role/cert-manager-modelservice \
  bound_service_account_names="cert-manager-vault-issuer" \
  bound_service_account_namespaces="gateway-system" \
  audience="vault://gateway-system/vault-issuer" \
  policies="cert-manager-modelservice-pki" \
  token_ttl=10m \
  token_max_ttl=30m
```

This later required an audience correction.

Final working role:

```bash
vault write auth/kubernetes-kind/role/cert-manager-modelservice \
  bound_service_account_names="cert-manager-vault-issuer" \
  bound_service_account_namespaces="gateway-system" \
  audience="https://kubernetes.default.svc.cluster.local" \
  policies="cert-manager-modelservice-pki" \
  token_ttl=10m \
  token_max_ttl=30m
```

Verify:

```bash
vault read \
  auth/kubernetes-kind/role/cert-manager-modelservice
```

Final important values:

```text
audience:
  https://kubernetes.default.svc.cluster.local

bound_service_account_names:
  [cert-manager-vault-issuer]

bound_service_account_namespaces:
  [gateway-system]

policies:
  [cert-manager-modelservice-pki]

token_ttl:
  10m

token_max_ttl:
  30m
```

---

# 27. Understand the initial reviewer-token problem

Initial auth configuration omitted `token_reviewer_jwt`.

Read:

```bash
vault read auth/kubernetes-kind/config
```

Initial result:

```text
kubernetes_host         https://kubernetes:6444
token_reviewer_jwt_set  false
```

When `token_reviewer_jwt` is not set, Vault may use the submitted login token to call TokenReview.

The cert-manager login token had the Vault audience:

```text
vault://gateway-system/vault-issuer
```

Kubernetes expected:

```text
https://kubernetes.default.svc.cluster.local
```

Direct TokenReview showed:

```text
invalid bearer token,
token audiences ["vault://gateway-system/vault-issuer"]
is invalid for target audiences
["https://kubernetes.default.svc.cluster.local"]
```

A dedicated reviewer identity was therefore created.

---

# 28. Create the reviewer ServiceAccount as code

File:

```text
config/platform/vault/vault-token-reviewer.yaml
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault-token-reviewer
  namespace: gateway-system
automountServiceAccountToken: false
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-token-reviewer-auth-delegator
subjects:
  - kind: ServiceAccount
    name: vault-token-reviewer
    namespace: gateway-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:auth-delegator
```

Apply:

```bash
kubectl apply \
  -f config/platform/vault/vault-token-reviewer.yaml
```

Validate:

```bash
kubectl auth can-i create \
  tokenreviews.authentication.k8s.io \
  --as=system:serviceaccount:gateway-system:vault-token-reviewer
```

Expected:

```text
yes
```

The warning that `tokenreviews` is not namespace-scoped is informational.

---

# 29. Temporary reviewer-token proof

Generate an initial 24-hour reviewer token:

```bash
REVIEWER_TOKEN="$(
  kubectl create token vault-token-reviewer \
    -n gateway-system \
    --duration=24h
)"
```

Verify without printing:

```bash
test -n "$REVIEWER_TOKEN" &&
echo "Reviewer token created"
```

Transfer to jumpbox:

```bash
printf '%s' "$REVIEWER_TOKEN" |
ssh jumpbox@jumpbox \
  'umask 077; cat > /tmp/kind-token-reviewer.jwt'
```

Clear locally:

```bash
unset REVIEWER_TOKEN
```

On the jumpbox, configure:

```bash
vault write auth/kubernetes-kind/config \
  kubernetes_host="https://kubernetes:6444" \
  kubernetes_ca_cert=@/home/jumpbox/kind-ai-platform-policy-ca.crt \
  token_reviewer_jwt=@/tmp/kind-token-reviewer.jwt \
  disable_iss_validation=true
```

Verify:

```bash
vault read auth/kubernetes-kind/config
```

Expected:

```text
kubernetes_host         https://kubernetes:6444
token_reviewer_jwt_set  true
```

Delete temporary file:

```bash
shred -u /tmp/kind-token-reviewer.jwt
```

This proved the design but was not durable because the token expired after 24 hours.

---

# 30. Diagnose the JWT audience failure

Create a login token with only the Vault audience:

```bash
LOGIN_TOKEN="$(
  kubectl create token cert-manager-vault-issuer \
    -n gateway-system \
    --audience='vault://gateway-system/vault-issuer' \
    --duration=10m
)"
```

Create a reviewer token:

```bash
REVIEWER_TOKEN="$(
  kubectl create token vault-token-reviewer \
    -n gateway-system \
    --duration=10m
)"
```

Call TokenReview:

```bash
curl --silent --show-error \
  --cacert .local/vault-auth/kind-ai-platform-policy-ca.crt \
  --header "Authorization: Bearer ${REVIEWER_TOKEN}" \
  --header 'Content-Type: application/json' \
  --request POST \
  --data "$(jq -n \
    --arg token "$LOGIN_TOKEN" \
    '{
      apiVersion:"authentication.k8s.io/v1",
      kind:"TokenReview",
      spec:{token:$token}
    }')" \
  https://kubernetes:6444/apis/authentication.k8s.io/v1/tokenreviews |
jq
```

Observed error:

```text
token audiences ["vault://gateway-system/vault-issuer"]
is invalid for target audiences
["https://kubernetes.default.svc.cluster.local"]
```

Decode claims without printing the full JWT:

```bash
printf '%s' "$LOGIN_TOKEN" |
cut -d. -f2 |
python3 -c '
import base64, json, sys
value = sys.stdin.read().strip()
value += "=" * (-len(value) % 4)
print(json.dumps(json.loads(base64.urlsafe_b64decode(value)), indent=2))
'
```

Validated claims:

```text
sub:
system:serviceaccount:gateway-system:cert-manager-vault-issuer

aud:
vault://gateway-system/vault-issuer
```

Clear:

```bash
unset LOGIN_TOKEN REVIEWER_TOKEN
```

---

# 31. Add the Kubernetes API audience to the Issuer

Update `vault-issuer.yaml`:

```yaml
serviceAccountRef:
  name: cert-manager-vault-issuer
  audiences:
    - https://kubernetes.default.svc.cluster.local
```

Apply:

```bash
kubectl apply \
  -f config/platform/vault/vault-issuer.yaml
```

cert-manager then requests a token with both effective audiences:

```text
vault://gateway-system/vault-issuer
https://kubernetes.default.svc.cluster.local
```

---

# 32. Prove two-audience TokenReview

Create:

```bash
LOGIN_TOKEN="$(
  kubectl create token cert-manager-vault-issuer \
    -n gateway-system \
    --audience='vault://gateway-system/vault-issuer' \
    --audience='https://kubernetes.default.svc.cluster.local' \
    --duration=10m
)"

REVIEWER_TOKEN="$(
  kubectl create token vault-token-reviewer \
    -n gateway-system \
    --audience='https://kubernetes.default.svc.cluster.local' \
    --duration=10m
)"
```

Perform TokenReview with the API audience:

```bash
curl --silent --show-error \
  --cacert .local/vault-auth/kind-ai-platform-policy-ca.crt \
  --header "Authorization: Bearer ${REVIEWER_TOKEN}" \
  --header 'Content-Type: application/json' \
  --request POST \
  --data "$(jq -n \
    --arg token "$LOGIN_TOKEN" \
    '{
      apiVersion: "authentication.k8s.io/v1",
      kind: "TokenReview",
      spec: {
        token: $token,
        audiences: [
          "https://kubernetes.default.svc.cluster.local"
        ]
      }
    }')" \
  https://kubernetes:6444/apis/authentication.k8s.io/v1/tokenreviews |
jq '{
  authenticated: .status.authenticated,
  username: .status.user.username,
  audiences: .status.audiences,
  error: .status.error
}'
```

Validated:

```json
{
  "authenticated": true,
  "username": "system:serviceaccount:gateway-system:cert-manager-vault-issuer",
  "audiences": [
    "https://kubernetes.default.svc.cluster.local"
  ],
  "error": null
}
```

This proved:

- the reviewer could call TokenReview;
- the login JWT was valid;
- the ServiceAccount identity was correct;
- the API audience was correct.

---

# 33. Correct the Vault role audience

The Vault role was changed from:

```text
vault://gateway-system/vault-issuer
```

to:

```text
https://kubernetes.default.svc.cluster.local
```

Run on jumpbox:

```bash
vault write auth/kubernetes-kind/role/cert-manager-modelservice \
  bound_service_account_names="cert-manager-vault-issuer" \
  bound_service_account_namespaces="gateway-system" \
  audience="https://kubernetes.default.svc.cluster.local" \
  policies="cert-manager-modelservice-pki" \
  token_ttl=10m \
  token_max_ttl=30m
```

Verify:

```bash
vault read auth/kubernetes-kind/role/cert-manager-modelservice
```

---

# 34. Fix hostname resolution on the actual Vault VM

Even after TokenReview worked manually from the Ansible VM, Vault login still returned:

```text
403 permission denied
```

The reason was that the `kubernetes` hostname was mapped on the jumpbox but not on the actual Vault VM.

Vault itself performs:

```text
POST https://kubernetes:6444/apis/authentication.k8s.io/v1/tokenreviews
```

Add on the Vault VM:

```bash
echo '192.168.0.58 kubernetes' |
sudo tee -a /etc/hosts
```

Verify:

```bash
getent hosts kubernetes
```

Expected:

```text
192.168.0.58 kubernetes
```

After this correction, Vault login succeeded.

---

# 35. Validate Vault login manually

On the Ansible VM:

```bash
LOGIN_TOKEN="$(
  kubectl create token cert-manager-vault-issuer \
    -n gateway-system \
    --audience='vault://gateway-system/vault-issuer' \
    --audience='https://kubernetes.default.svc.cluster.local' \
    --duration=10m
)"
```

Use `--resolve` because the Ansible host did not initially resolve `vault.platform.local`:

```bash
curl --silent --show-error \
  --cacert .local/tls/vault-server-ca.crt \
  --resolve vault.platform.local:8200:192.168.0.61 \
  --header 'Content-Type: application/json' \
  --request POST \
  --data "$(jq -n \
    --arg jwt "$LOGIN_TOKEN" \
    --arg role 'cert-manager-modelservice' \
    '{jwt:$jwt,role:$role}')" \
  https://vault.platform.local:8200/v1/auth/kubernetes-kind/login |
jq '{
  errors,
  authenticated: (.auth != null),
  policies: .auth.policies,
  lease_duration: .auth.lease_duration
}'
```

Validated:

```json
{
  "errors": null,
  "authenticated": true,
  "policies": [
    "cert-manager-modelservice-pki",
    "default"
  ],
  "lease_duration": 600
}
```

Clear:

```bash
unset LOGIN_TOKEN
```

Never print or store the returned Vault client token.

---

# 36. Final reproducible `configure-kind-auth.sh`

File:

```text
infrastructure/vault/scripts/configure-kind-auth.sh
```

Final working script:

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${VAULT_ADDR:?Set VAULT_ADDR}"
: "${VAULT_CACERT:?Set VAULT_CACERT}"
: "${KUBERNETES_HOST:?Set KUBERNETES_HOST}"
: "${KUBERNETES_CA_FILE:?Set KUBERNETES_CA_FILE}"
: "${TOKEN_REVIEWER_JWT_FILE:?Set TOKEN_REVIEWER_JWT_FILE}"

VAULT_K8S_AUTH_PATH="${VAULT_K8S_AUTH_PATH:-kubernetes-kind}"
VAULT_K8S_ROLE="${VAULT_K8S_ROLE:-cert-manager-modelservice}"
VAULT_POLICY="${VAULT_POLICY:-cert-manager-modelservice-pki}"
TOKEN_AUDIENCE="${TOKEN_AUDIENCE:-https://kubernetes.default.svc.cluster.local}"

for file in \
  "${KUBERNETES_CA_FILE}" \
  "${TOKEN_REVIEWER_JWT_FILE}"
do
  if [[ ! -s "${file}" ]]; then
    echo "ERROR: Required file is missing or empty: ${file}" >&2
    exit 1
  fi
done

if ! vault auth list -format=json |
  jq -e \
    --arg path "${VAULT_K8S_AUTH_PATH}/" \
    'has($path)' \
    >/dev/null
then
  vault auth enable \
    -path="${VAULT_K8S_AUTH_PATH}" \
    kubernetes
fi

vault write \
  "auth/${VAULT_K8S_AUTH_PATH}/config" \
  kubernetes_host="${KUBERNETES_HOST}" \
  kubernetes_ca_cert=@"${KUBERNETES_CA_FILE}" \
  token_reviewer_jwt=@"${TOKEN_REVIEWER_JWT_FILE}" \
  disable_iss_validation=true

vault write \
  "auth/${VAULT_K8S_AUTH_PATH}/role/${VAULT_K8S_ROLE}" \
  bound_service_account_names="cert-manager-vault-issuer" \
  bound_service_account_namespaces="gateway-system" \
  audience="${TOKEN_AUDIENCE}" \
  policies="${VAULT_POLICY}" \
  token_ttl=10m \
  token_max_ttl=30m

echo
echo "Vault Kubernetes authentication configured."
echo "Auth mount: ${VAULT_K8S_AUTH_PATH}/"
echo "Role:       ${VAULT_K8S_ROLE}"
echo "Audience:   ${TOKEN_AUDIENCE}"
```

Make executable:

```bash
chmod +x \
  infrastructure/vault/scripts/configure-kind-auth.sh
```

Validate syntax:

```bash
bash -n \
  infrastructure/vault/scripts/configure-kind-auth.sh
```

Optional static analysis:

```bash
shellcheck \
  infrastructure/vault/scripts/configure-kind-auth.sh
```

---

# 37. Wait for the Vault Issuer

Force a harmless metadata update:

```bash
kubectl annotate issuer vault-issuer \
  -n gateway-system \
  vault-pki-recheck="$(date +%s)" \
  --overwrite
```

Wait:

```bash
kubectl wait \
  --for=condition=Ready \
  issuer/vault-issuer \
  -n gateway-system \
  --timeout=180s
```

Validated:

```text
issuer.cert-manager.io/vault-issuer condition met
```

Verify:

```bash
kubectl get issuer vault-issuer \
  -n gateway-system
```

Expected:

```text
NAME           READY
vault-issuer   True
```

Describe:

```bash
kubectl describe issuer vault-issuer \
  -n gateway-system
```

Target condition:

```text
Type:    Ready
Status:  True
```

---

# 38. Switch the Certificate to Vault

Apply:

```bash
kubectl apply \
  -f config/platform/vault/fraud-model-vault-certificate.yaml
```

Watch:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -w
```

Inspect CertificateRequests:

```bash
kubectl get certificaterequest \
  -n gateway-system
```

Validated:

```text
fraud-model-local-2
APPROVED=True
READY=True
ISSUER=vault-issuer
```

Verify the Certificate reference:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='issuerName={.spec.issuerRef.name}{"\n"}ready={.status.conditions[?(@.type=="Ready")].status}{"\n"}'
```

Validated:

```text
issuerName=vault-issuer
ready=True
```

---

# 39. Validate the Vault-issued TLS Secret

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

Validated:

```text
TYPE                DATA
kubernetes.io/tls   3
```

Inspect certificate:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.tls\.crt}' |
base64 --decode |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -dates \
  -serial
```

Validated:

```text
subject=CN = fraud-model.local
issuer=CN = AI Platform ModelService Root CA
notBefore=Aug 3 16:35:05 2026 GMT
notAfter=Sep 2 16:35:35 2026 GMT
serial=612BAB3FB396345A644841D9BA5EF33A2D7523EE
```

This proves the live certificate was signed by Vault rather than the development root CA.

---

# 40. Validate Gateway continuity

```bash
kubectl get issuer vault-issuer \
  -n gateway-system

kubectl get certificate fraud-model-local \
  -n gateway-system

kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide
```

Validated:

```text
vault-issuer        Ready=True
fraud-model-local   Ready=True
shared-gateway      Programmed=True
```

The Secret name did not change, so Envoy Gateway automatically consumed the updated certificate.

---

# 41. Install `cmctl`

On the Ansible VM:

```bash
cd /tmp

OS="$(
  uname -s |
  tr '[:upper:]' '[:lower:]'
)"

ARCH="$(
  uname -m |
  sed 's/x86_64/amd64/' |
  sed 's/aarch64/arm64/'
)"

curl -fsSLo cmctl \
  "https://github.com/cert-manager/cmctl/releases/latest/download/cmctl_${OS}_${ARCH}"

chmod +x cmctl

sudo install \
  -m 0755 \
  cmctl \
  /usr/local/bin/cmctl
```

Verify:

```bash
command -v cmctl
cmctl version --client
cmctl check api --wait=2m
```

Validated:

```text
/usr/local/bin/cmctl
Client Version: v2.5.0
The cert-manager API is ready
```

---

# 42. Create the non-destructive validation script

File:

```text
infrastructure/vault/scripts/validate-vault-pki-integration.sh
```

```bash
#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-gateway-system}"

echo "=== Vault Issuer ==="
kubectl get issuer vault-issuer \
  --namespace "${NAMESPACE}"

kubectl wait \
  --for=condition=Ready \
  issuer/vault-issuer \
  --namespace "${NAMESPACE}" \
  --timeout=180s

echo
echo "=== Certificate ==="
kubectl get certificate fraud-model-local \
  --namespace "${NAMESPACE}"

kubectl wait \
  --for=condition=Ready \
  certificate/fraud-model-local \
  --namespace "${NAMESPACE}" \
  --timeout=180s

echo
echo "=== Latest CertificateRequest ==="
kubectl get certificaterequests \
  --namespace "${NAMESPACE}" \
  --sort-by=.metadata.creationTimestamp

echo
echo "=== Live certificate ==="
kubectl get secret fraud-model-local-tls \
  --namespace "${NAMESPACE}" \
  --output jsonpath='{.data.tls\.crt}' |
base64 --decode |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -dates \
  -serial

echo
echo "=== Gateway ==="
kubectl get gateway shared-gateway \
  --namespace "${NAMESPACE}" \
  --output wide
```

Make executable and validate:

```bash
chmod +x \
  infrastructure/vault/scripts/validate-vault-pki-integration.sh

bash -n \
  infrastructure/vault/scripts/validate-vault-pki-integration.sh
```

Run:

```bash
infrastructure/vault/scripts/validate-vault-pki-integration.sh
```

---

# 43. Create the renewal and key-rotation validation script

File:

```text
infrastructure/vault/scripts/validate-certificate-rotation.sh
```

```bash
#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-gateway-system}"
CERTIFICATE_NAME="${CERTIFICATE_NAME:-fraud-model-local}"
SECRET_NAME="${SECRET_NAME:-fraud-model-local-tls}"
ISSUER_NAME="${ISSUER_NAME:-vault-issuer}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
TIMEOUT="${TIMEOUT:-180s}"

command -v kubectl >/dev/null 2>&1 || {
  echo "ERROR: kubectl is required." >&2
  exit 1
}

command -v openssl >/dev/null 2>&1 || {
  echo "ERROR: openssl is required." >&2
  exit 1
}

command -v cmctl >/dev/null 2>&1 || {
  echo "ERROR: cmctl is required." >&2
  exit 1
}

secret_certificate() {
  kubectl get secret "${SECRET_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.data.tls\.crt}' |
  base64 --decode
}

secret_private_key() {
  kubectl get secret "${SECRET_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.data.tls\.key}' |
  base64 --decode
}

certificate_serial() {
  secret_certificate |
  openssl x509 -noout -serial |
  cut -d= -f2
}

certificate_issuer() {
  secret_certificate |
  openssl x509 -noout -issuer |
  sed 's/^issuer=//'
}

private_key_fingerprint() {
  secret_private_key |
  openssl pkey -pubout 2>/dev/null |
  openssl sha256 |
  awk '{print $2}'
}

echo "Checking Vault Issuer..."

kubectl wait \
  --for=condition=Ready \
  "issuer/${ISSUER_NAME}" \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

echo "Checking current Certificate..."

kubectl wait \
  --for=condition=Ready \
  "certificate/${CERTIFICATE_NAME}" \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

CURRENT_ISSUER="$(
  kubectl get certificate "${CERTIFICATE_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.spec.issuerRef.name}'
)"

if [[ "${CURRENT_ISSUER}" != "${ISSUER_NAME}" ]]; then
  echo "ERROR: Certificate uses '${CURRENT_ISSUER}', expected '${ISSUER_NAME}'." >&2
  exit 1
fi

OLD_SERIAL="$(certificate_serial)"
OLD_KEY_FINGERPRINT="$(private_key_fingerprint)"

echo "Before renewal:"
printf '  Serial:          %s\n' "${OLD_SERIAL}"
printf '  Key fingerprint: %s\n' "${OLD_KEY_FINGERPRINT}"

echo "Requesting renewal with cmctl..."

cmctl renew "${CERTIFICATE_NAME}" \
  --namespace "${NAMESPACE}"

echo "Waiting for a new certificate serial..."

DEADLINE=$((SECONDS + 180))

while true; do
  NEW_SERIAL="$(certificate_serial)"

  if [[ "${NEW_SERIAL}" != "${OLD_SERIAL}" ]]; then
    break
  fi

  if (( SECONDS >= DEADLINE )); then
    echo "ERROR: Certificate serial did not change within 180 seconds." >&2
    exit 1
  fi

  sleep 5
done

kubectl wait \
  --for=condition=Ready \
  "certificate/${CERTIFICATE_NAME}" \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

NEW_SERIAL="$(certificate_serial)"
NEW_KEY_FINGERPRINT="$(private_key_fingerprint)"
NEW_ISSUER="$(certificate_issuer)"

echo "After renewal:"
printf '  Serial:          %s\n' "${NEW_SERIAL}"
printf '  Key fingerprint: %s\n' "${NEW_KEY_FINGERPRINT}"
printf '  Certificate CA:  %s\n' "${NEW_ISSUER}"

if [[ "${OLD_SERIAL}" == "${NEW_SERIAL}" ]]; then
  echo "ERROR: Certificate serial did not rotate." >&2
  exit 1
fi

if [[ "${OLD_KEY_FINGERPRINT}" == "${NEW_KEY_FINGERPRINT}" ]]; then
  echo "ERROR: Private key did not rotate." >&2
  exit 1
fi

if [[ "${NEW_ISSUER}" != *"AI Platform ModelService Root CA"* ]]; then
  echo "ERROR: Renewed certificate was not signed by the expected Vault CA." >&2
  exit 1
fi

PROGRAMMED="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'
)"

if [[ "${PROGRAMMED}" != "True" ]]; then
  echo "ERROR: Gateway is not Programmed after certificate rotation." >&2
  exit 1
fi

echo
echo "PASS: Vault certificate renewal succeeded."
echo "PASS: Certificate serial changed."
echo "PASS: Private key rotated."
echo "PASS: Renewed certificate was signed by Vault."
echo "PASS: Gateway remained Programmed."
```

Make executable:

```bash
chmod +x \
  infrastructure/vault/scripts/validate-certificate-rotation.sh
```

Validate syntax:

```bash
bash -n \
  infrastructure/vault/scripts/validate-certificate-rotation.sh
```

Optional ShellCheck:

```bash
sudo apt update
sudo apt install -y shellcheck

shellcheck \
  infrastructure/vault/scripts/validate-vault-pki-integration.sh \
  infrastructure/vault/scripts/validate-certificate-rotation.sh
```

These scripts are executed directly. They are not Kubernetes manifests:

```text
YAML manifest     → kubectl apply -f
Kustomize folder  → kubectl apply -k
Bash script       → ./script.sh
```

---

# 44. Validate forced renewal and private-key rotation

Run:

```bash
infrastructure/vault/scripts/validate-certificate-rotation.sh
```

Validated output:

```text
Checking Vault Issuer...
issuer.cert-manager.io/vault-issuer condition met

Checking current Certificate...
certificate.cert-manager.io/fraud-model-local condition met

Before renewal:
  Serial:
  612BAB3FB396345A644841D9BA5EF33A2D7523EE

  Key fingerprint:
  0578bfdb10074694327e4690233dfed2ec782a6e6ef5db5229af6d391a94f8b7

Requesting renewal with cmctl...
Manually triggered issuance of Certificate gateway-system/fraud-model-local

After renewal:
  Serial:
  75E95A2AC3FD59D0411A59A1AC073FA1AA1E27F6

  Key fingerprint:
  5bc8b9b2a6c3416e5052aa2d1ef2de8e9c4a5722f8a67ee3293dd7481e025fc7

  Certificate CA:
  CN = AI Platform ModelService Root CA

PASS: Vault certificate renewal succeeded.
PASS: Certificate serial changed.
PASS: Private key rotated.
PASS: Renewed certificate was signed by Vault.
PASS: Gateway remained Programmed.
```

This proves:

- Vault authentication continued to work;
- the Vault signing endpoint worked;
- cert-manager created a new certificate;
- the certificate serial changed;
- `rotationPolicy: Always` generated a new key;
- the Gateway remained functional.

---

# 45. Replace the temporary reviewer token with a durable declarative token

The 24-hour reviewer token was only a proof.

A declarative ServiceAccount-token Secret was created for this isolated lab.

## 45.1 Security note

Modern Kubernetes recommends short-lived TokenRequest tokens.

A manually created:

```text
kubernetes.io/service-account-token
```

Secret is long-lived and should be used only when its operational tradeoff is accepted.

For production, prefer:

- automated reviewer-token rotation;
- Vault running in Kubernetes and using a mounted token;
- a JWT/OIDC auth design that does not need TokenReview;
- another supported workload identity mechanism.

The long-lived token used here has only `system:auth-delegator` TokenReview permission, but it is still sensitive.

## 45.2 Create the Secret manifest

File:

```text
config/platform/vault/vault-token-reviewer-secret.yaml
```

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vault-token-reviewer-token
  namespace: gateway-system
  annotations:
    kubernetes.io/service-account.name: vault-token-reviewer
type: kubernetes.io/service-account-token
```

The manifest contains no JWT value.

Kubernetes populates:

```text
ca.crt
namespace
token
```

Add to Kustomize:

```yaml
resources:
  - vault-token-reviewer-secret.yaml
```

Apply:

```bash
kubectl apply \
  -f config/platform/vault/vault-token-reviewer-secret.yaml
```

Wait:

```bash
kubectl wait \
  --for=jsonpath='{.data.token}' \
  secret/vault-token-reviewer-token \
  -n gateway-system \
  --timeout=60s
```

Verify without decoding:

```bash
kubectl get secret vault-token-reviewer-token \
  -n gateway-system
```

Validated:

```text
TYPE                                  DATA
kubernetes.io/service-account-token   3
```

---

# 46. Transfer the durable reviewer token securely

From the Ansible VM:

```bash
kubectl get secret vault-token-reviewer-token \
  -n gateway-system \
  -o jsonpath='{.data.token}' |
base64 --decode |
ssh jumpbox@jumpbox \
  'umask 077; cat > /tmp/kind-token-reviewer.jwt'
```

Confirm without printing:

```bash
ssh jumpbox@jumpbox \
  'test -s /tmp/kind-token-reviewer.jwt && echo "Reviewer token received"'
```

Expected:

```text
Reviewer token received
```

---

# 47. Write the durable reviewer token into Vault

On the jumpbox:

```bash
cd /mnt/data/ai-platform-operator

set -a
source infrastructure/vault/variables.env
set +a

export TOKEN_REVIEWER_JWT_FILE=/tmp/kind-token-reviewer.jwt
export TOKEN_AUDIENCE=https://kubernetes.default.svc.cluster.local
```

Run the reproducible script:

```bash
infrastructure/vault/scripts/configure-kind-auth.sh
```

Validated:

```text
Success! Data written to: auth/kubernetes-kind/config
Success! Data written to: auth/kubernetes-kind/role/cert-manager-modelservice

Vault Kubernetes authentication configured.
Auth mount: kubernetes-kind/
Role:       cert-manager-modelservice
Audience:   https://kubernetes.default.svc.cluster.local
```

Verify config:

```bash
vault read auth/kubernetes-kind/config |
grep -E 'kubernetes_host|token_reviewer_jwt_set'
```

Validated:

```text
kubernetes_host         https://kubernetes:6444
token_reviewer_jwt_set  true
```

Verify role:

```bash
vault read auth/kubernetes-kind/role/cert-manager-modelservice |
grep -E \
  'audience|bound_service_account_names|bound_service_account_namespaces|policies'
```

Validated:

```text
audience:
https://kubernetes.default.svc.cluster.local

bound_service_account_names:
[cert-manager-vault-issuer]

bound_service_account_namespaces:
[gateway-system]

policies:
[cert-manager-modelservice-pki]
```

Delete the transferred copy:

```bash
shred -u /tmp/kind-token-reviewer.jwt
```

Vault stores the configured credential internally.

---

# 48. Validate authentication after durable-token replacement

On the Ansible VM:

```bash
LOGIN_TOKEN="$(
  kubectl create token cert-manager-vault-issuer \
    -n gateway-system \
    --audience='vault://gateway-system/vault-issuer' \
    --audience='https://kubernetes.default.svc.cluster.local' \
    --duration=10m
)"
```

Test:

```bash
curl --silent --show-error \
  --cacert .local/tls/vault-server-ca.crt \
  --resolve vault.platform.local:8200:192.168.0.61 \
  --header 'Content-Type: application/json' \
  --request POST \
  --data "$(jq -n \
    --arg jwt "$LOGIN_TOKEN" \
    --arg role 'cert-manager-modelservice' \
    '{jwt:$jwt,role:$role}')" \
  https://vault.platform.local:8200/v1/auth/kubernetes-kind/login |
jq '{
  errors,
  authenticated: (.auth != null),
  policies: .auth.policies
}'
```

Validated:

```json
{
  "errors": null,
  "authenticated": true,
  "policies": [
    "cert-manager-modelservice-pki",
    "default"
  ]
}
```

Clear:

```bash
unset LOGIN_TOKEN
```

Final Kubernetes health:

```bash
kubectl get issuer vault-issuer \
  -n gateway-system

kubectl get certificate fraud-model-local \
  -n gateway-system

kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide
```

Expected:

```text
vault-issuer        Ready=True
fraud-model-local   Ready=True
shared-gateway      Programmed=True
```

---

# 49. Full bootstrap order

For a clean rebuild:

```text
1. Restore the kind cluster.
2. Restore Calico, Gateway API, Envoy Gateway, MetalLB, and cert-manager.
3. Restore shared Gateway and development certificate.
4. Confirm Vault is initialized, unsealed, active, and reachable.
5. Configure CoreDNS for vault.platform.local.
6. Import the public Vault server CA locally.
7. Prove trusted Vault HTTPS from a Pod.
8. Export the kind Kubernetes CA.
9. Copy the Kubernetes CA to the jumpbox.
10. Expose the kind API on 0.0.0.0:6444.
11. Map kubernetes → Ansible VM IP on jumpbox and Vault VM.
12. Verify https://kubernetes:6444/version from the jumpbox.
13. Apply cert-manager Vault ServiceAccount and TokenRequest RBAC.
14. Apply reviewer ServiceAccount and auth-delegator binding.
15. Apply the durable reviewer-token Secret.
16. Transfer the reviewer JWT to the jumpbox temporarily.
17. Run configure-modelservice-pki.sh.
18. Run configure-kind-auth.sh.
19. Delete the temporary reviewer JWT file.
20. Apply the Vault CA Secret and Vault Issuer.
21. Wait for vault-issuer Ready=True.
22. Apply the Vault-backed Certificate.
23. Wait for Certificate Ready=True.
24. Inspect the issuer and certificate chain.
25. Run non-destructive validation.
26. Run renewal and private-key rotation validation.
27. Confirm Gateway Programmed=True.
```

---

# 50. Recovery command sequence

## 50.1 Kubernetes side

```bash
cd /mnt/data/ai-platform-operator

kubectl apply \
  -f config/platform/vault/cert-manager-vault-serviceaccount.yaml

kubectl apply \
  -f config/platform/vault/cert-manager-vault-token-rbac.yaml

kubectl apply \
  -f config/platform/vault/vault-token-reviewer.yaml

kubectl apply \
  -f config/platform/vault/vault-token-reviewer-secret.yaml
```

Wait for reviewer token:

```bash
kubectl wait \
  --for=jsonpath='{.data.token}' \
  secret/vault-token-reviewer-token \
  -n gateway-system \
  --timeout=60s
```

Transfer:

```bash
kubectl get secret vault-token-reviewer-token \
  -n gateway-system \
  -o jsonpath='{.data.token}' |
base64 --decode |
ssh jumpbox@jumpbox \
  'umask 077; cat > /tmp/kind-token-reviewer.jwt'
```

## 50.2 Vault side

On jumpbox:

```bash
cd /mnt/data/ai-platform-operator

set -a
source infrastructure/vault/variables.env
set +a

export TOKEN_REVIEWER_JWT_FILE=/tmp/kind-token-reviewer.jwt
```

Configure:

```bash
infrastructure/vault/scripts/configure-modelservice-pki.sh

infrastructure/vault/scripts/configure-kind-auth.sh
```

Clean:

```bash
shred -u /tmp/kind-token-reviewer.jwt
```

## 50.3 Apply Issuer

On Ansible VM:

```bash
kubectl apply -k config/platform/vault
```

Wait:

```bash
kubectl wait \
  --for=condition=Ready \
  issuer/vault-issuer \
  -n gateway-system \
  --timeout=180s
```

## 50.4 Switch Certificate

```bash
kubectl apply \
  -f config/platform/vault/fraud-model-vault-certificate.yaml
```

Wait:

```bash
kubectl wait \
  --for=condition=Ready \
  certificate/fraud-model-local \
  -n gateway-system \
  --timeout=180s
```

Validate:

```bash
infrastructure/vault/scripts/validate-vault-pki-integration.sh
```

---

# 51. Troubleshooting

## 51.1 `vault.platform.local` cannot resolve from Pods

Test:

```bash
kubectl run vault-dns-test \
  -n default \
  --image=busybox:1.36 \
  --restart=Never \
  --command -- \
  nslookup vault.platform.local
```

Fix CoreDNS:

```text
hosts {
    192.168.0.61 vault.platform.local
    fallthrough
}
```

Restart:

```bash
kubectl rollout restart deployment/coredns \
  -n kube-system
```

---

## 51.2 Pod can connect only with `curl -k`

The Vault CA is missing or incorrect.

Verify:

```bash
openssl x509 \
  -in .local/tls/vault-server-ca.crt \
  -noout \
  -subject \
  -issuer
```

Mount it and use:

```bash
curl \
  --cacert /vault-ca/ca.crt \
  https://vault.platform.local:8200/v1/sys/health
```

---

## 51.3 Pod cannot reach `192.168.0.61`

Check Pod CIDR:

```bash
kubectl get pods -A -o wide
```

Do not overlap the physical LAN.

Validated design:

```text
Pod CIDR:  10.244.0.0/16
LAN:       192.168.0.0/16
```

---

## 51.4 Jumpbox cannot resolve `kubernetes`

Add:

```bash
echo '192.168.0.58 kubernetes' |
sudo tee -a /etc/hosts
```

The same mapping must exist on the actual Vault VM.

---

## 51.5 Kubernetes API TLS hostname mismatch

Error:

```text
no alternative certificate subject name matches 192.168.0.58
```

Use a SAN from the API server certificate:

```text
kubernetes
```

Connect to:

```text
https://kubernetes:6444
```

---

## 51.6 Vault Issuer returns `403 permission denied`

Inspect:

```bash
kubectl describe issuer vault-issuer \
  -n gateway-system
```

Check:

```bash
vault auth list
vault read auth/kubernetes-kind/config
vault read auth/kubernetes-kind/role/cert-manager-modelservice
```

Validate:

```text
auth mount exists
token_reviewer_jwt_set=true
Kubernetes host=https://kubernetes:6444
ServiceAccount name matches
namespace matches
audience matches
policy exists
Vault VM resolves kubernetes
```

---

## 51.7 TokenReview audience error

Error:

```text
token audiences ["vault://..."]
is invalid for target audiences
["https://kubernetes.default.svc.cluster.local"]
```

Issuer must request the API audience:

```yaml
serviceAccountRef:
  name: cert-manager-vault-issuer
  audiences:
    - https://kubernetes.default.svc.cluster.local
```

Vault role:

```text
audience=https://kubernetes.default.svc.cluster.local
```

---

## 51.8 `token_reviewer_jwt_set=false`

Configure a reviewer JWT:

```bash
vault write auth/kubernetes-kind/config \
  kubernetes_host="https://kubernetes:6444" \
  kubernetes_ca_cert=@/home/jumpbox/kind-ai-platform-policy-ca.crt \
  token_reviewer_jwt=@/tmp/kind-token-reviewer.jwt \
  disable_iss_validation=true
```

---

## 51.9 Vault CLI not installed on Ansible VM

Run Vault administration commands on the jumpbox, where:

```text
vault
VAULT_ADDR
VAULT_CACERT
```

are configured.

Do not confuse prompts:

```text
ansible@server
jumpbox@jumpbox
Vault VM
```

Each command in this guide indicates the correct host.

---

## 51.10 Issuer exists before Vault is configured

Expected temporary state:

```text
Ready=False
VaultError
403 permission denied
```

The development Certificate remains active until its `issuerRef` is changed.

After fixing Vault:

```bash
kubectl annotate issuer vault-issuer \
  -n gateway-system \
  vault-pki-recheck="$(date +%s)" \
  --overwrite
```

---

## 51.11 Certificate stays on development-ca

Check:

```bash
kubectl get certificate fraud-model-local \
  -n gateway-system \
  -o jsonpath='{.spec.issuerRef.name}{"\n"}'
```

Apply:

```bash
kubectl apply \
  -f config/platform/vault/fraud-model-vault-certificate.yaml
```

---

## 51.12 Certificate Ready but wrong issuer

Inspect:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.tls\.crt}' |
base64 --decode |
openssl x509 \
  -noout \
  -issuer
```

Expected:

```text
CN = AI Platform ModelService Root CA
```

---

## 51.13 Rotation script says serial did not change

Check CertificateRequests:

```bash
kubectl get certificaterequests \
  -n gateway-system \
  --sort-by=.metadata.creationTimestamp
```

Describe latest:

```bash
kubectl describe certificaterequest \
  -n gateway-system \
  "$(kubectl get certificaterequest \
    -n gateway-system \
    --sort-by=.metadata.creationTimestamp \
    -o jsonpath='{.items[-1:].metadata.name}')"
```

Check Issuer:

```bash
kubectl get issuer vault-issuer \
  -n gateway-system
```

---

# 52. Security rules

## Never commit

```text
Vault root token
Vault unseal keys
Vault private keys
internal CA private keys
ServiceAccount JWT values
decoded reviewer token
temporary /tmp/kind-token-reviewer.jwt
real variables.env containing secrets
.local secret material
```

## Safe to commit

```text
ServiceAccount manifests
RBAC manifests
Issuer manifests
Certificate manifests
Kustomization
Vault HCL policy
Vault setup scripts
public CA certificate, when intentionally managed
documentation
```

## Runtime hygiene

```bash
unset LOGIN_TOKEN
unset REVIEWER_TOKEN
unset VAULT_TOKEN

shred -u /tmp/kind-token-reviewer.jwt
```

Restrict Secret access in `gateway-system`.

---

# 53. Suggested `.gitignore`

```gitignore
# Local generated and sensitive material
.local/
infrastructure/vault/variables.env

# Tokens and private keys
*.token
*.jwt
*.key
*.pem

# Vault credentials
.vault-token
vault-token
unseal-keys*
root-token*

# Keep example configuration
!infrastructure/vault/variables.env.example
```

Do not blindly ignore all `.crt` files if a public CA certificate is intentionally versioned.

---

# 54. Final validation matrix

| Validation | Result |
|---|---:|
| Vault initialized | ✅ |
| Vault unsealed | ✅ |
| Vault active | ✅ |
| Vault HTTPS trusted | ✅ |
| Pod-to-Vault connectivity | ✅ |
| Pod DNS for `vault.platform.local` | ✅ |
| Vault server CA available in cluster | ✅ |
| Existing Talos auth mount preserved | ✅ |
| Dedicated `kubernetes-kind/` mount | ✅ |
| kind API reachable from Vault | ✅ |
| Kubernetes API TLS hostname valid | ✅ |
| Reviewer ServiceAccount | ✅ |
| Reviewer TokenReview permission | ✅ |
| Durable reviewer-token Secret | ✅ |
| Reviewer JWT stored in Vault | ✅ |
| Temporary transferred JWT removed | ✅ |
| cert-manager TokenRequest RBAC | ✅ |
| Login JWT audiences correct | ✅ |
| Vault role audience correct | ✅ |
| Manual Vault login succeeds | ✅ |
| `pki_modelservice/` enabled | ✅ |
| ModelService root CA created | ✅ |
| PKI signing role created | ✅ |
| Least-privilege signing policy created | ✅ |
| Vault Issuer `Ready=True` | ✅ |
| Certificate switched to `vault-issuer` | ✅ |
| CertificateRequest approved | ✅ |
| CertificateRequest `Ready=True` | ✅ |
| TLS Secret updated | ✅ |
| Certificate issuer is Vault CA | ✅ |
| Gateway remains `Programmed=True` | ✅ |
| Forced renewal succeeds | ✅ |
| Serial changes | ✅ |
| Private key rotates | ✅ |
| Renewed certificate signed by Vault | ✅ |
| Validation scripts stored as code | ✅ |
| Setup scripts stored as code | ✅ |

---

# 55. Final state

Vault:

```text
auth/kubernetes-kind/
pki_modelservice/
policy/cert-manager-modelservice-pki
role/cert-manager-modelservice
PKI role/modelservice
```

Kubernetes:

```text
ServiceAccount/gateway-system/cert-manager-vault-issuer
Role/gateway-system/cert-manager-vault-tokenrequest
RoleBinding/gateway-system/cert-manager-vault-tokenrequest
ClusterRoleBinding/cert-manager-vault-issuer-auth-delegator

ServiceAccount/gateway-system/vault-token-reviewer
ClusterRoleBinding/vault-token-reviewer-auth-delegator
Secret/gateway-system/vault-token-reviewer-token

Secret/gateway-system/vault-server-ca
Issuer/gateway-system/vault-issuer
Certificate/gateway-system/fraud-model-local
Secret/gateway-system/fraud-model-local-tls
```

Live certificate:

```text
Subject:
CN=fraud-model.local

Issuer:
CN=AI Platform ModelService Root CA

Managed by:
cert-manager

Signed by:
Vault PKI

Consumed by:
Envoy Gateway
```

---

# 56. Phase boundary

This Vault PKI milestone is complete.

Completed:

```text
[✓] external Vault trusted by Kubernetes
[✓] dedicated kind authentication
[✓] cert-manager workload identity
[✓] durable TokenReview identity
[✓] private CA and signing role
[✓] automated issuance
[✓] automated renewal
[✓] private-key rotation
[✓] Gateway continuity
[✓] infrastructure stored as code
[✓] validation stored as code
```

The next independent phase is:

```text
OIDC/JWT authentication for AI platform users
        ↓
authorization and role enforcement
```

OIDC/JWT user authentication must not be confused with the Kubernetes JWT authentication cert-manager uses to obtain a Vault token. They solve different problems:

```text
cert-manager → Vault Kubernetes auth
    purpose: certificate issuance identity

end user → AI platform OIDC/JWT
    purpose: user authentication and access control
```
