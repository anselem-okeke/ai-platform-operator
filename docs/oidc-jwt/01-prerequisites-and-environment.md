# OIDC/JWT Security for the AI Platform Operator

## Phase 1 — Prerequisites and Environment

## 1. Purpose

This document defines the environment, software, cluster components, network assumptions, names, paths, and pre-flight checks required before reproducing the OIDC/JWT security implementation for the AI Platform Operator.

Complete this phase before installing Keycloak, configuring Vault PKI, creating OIDC clients, or attaching an Envoy Gateway `SecurityPolicy`.

The implementation documented in this series protects the following model-serving workload:

```text
ModelService: fraud-model
Namespace:    ai-platform
Hostname:     fraud-model.local
```

The OpenID Connect identity provider is exposed as:

```text
Keycloak hostname: auth.ai-platform.local
Realm:             ai-platform
Expected audience: ai-platform-gateway
```

---

## 2. Environment Used for This Implementation

### 2.1 Repository and execution host

The implementation was performed from the Ansible VM:

```text
Host role:       Ansible and Kubernetes administration VM
Repository path: /mnt/data/ai-platform-operator
```

All commands in the documentation assume the working directory is:

```bash
cd /mnt/data/ai-platform-operator
```

### 2.2 Kubernetes cluster

```text
Cluster type:       kind
Kind cluster name:  ai-platform-policy
kubectl context:    kind-ai-platform-policy
Kubernetes version: v1.36.1
API endpoint port:  6444 on the Ansible VM
```

The kind control-plane Pod or node is expected to resemble:

```text
ai-platform-policy-control-plane
```

### 2.3 Cluster networking

```text
Gateway implementation: Envoy Gateway
LoadBalancer provider:   MetalLB
Gateway address:         172.19.255.200
CNI/network policy:      Calico
```

The Gateway address may change after a cluster rebuild. Commands later in this guide resolve it dynamically from Gateway status rather than relying permanently on `172.19.255.200`.

### 2.4 Shared Gateway

```text
Namespace: gateway-system
Name:      shared-gateway
```

The final Gateway configuration contains:

```text
Listener: http
Protocol: HTTP
Port:     80

Listener: keycloak-https
Hostname: auth.ai-platform.local
Protocol: HTTPS
Port:     443

Listener: fraud-model-https
Hostname: fraud-model.local
Protocol: HTTPS
Port:     443
```

The HTTPS listeners share port `443`. Envoy selects the correct listener and TLS certificate through Server Name Indication.

### 2.5 Keycloak

```text
Namespace:          keycloak
Image:              quay.io/keycloak/keycloak:26.7.0
External hostname:  auth.ai-platform.local
External URL:       https://auth.ai-platform.local
Application port:   8080
Management port:    9000
```

### 2.6 PostgreSQL for Keycloak

```text
Image:        postgres:17.6-alpine
Namespace:    keycloak
Workload:     StatefulSet/keycloak-postgres
Storage size: 5Gi
```

### 2.7 Vault

Vault is external to the kind cluster.

```text
Vault address:  https://vault.platform.local:8200
Vault IP:       192.168.0.61
Vault version:  2.0.3
Storage:        integrated Raft
Seal type:      Shamir
PKI mount:      pki_modelservice/
Kubernetes auth mount used by kind: kubernetes-kind/
```

The existing Talos-cluster authentication mount at `kubernetes/` is preserved. The kind cluster uses the dedicated `kubernetes-kind/` mount.

### 2.8 Administrative hosts

```text
Ansible VM: 192.168.0.58
Jumpbox:    192.168.0.28
Vault VM:   192.168.0.61
```

Vault administration commands are run from the jumpbox when access to the Vault CLI, root or administrative token, and Vault CA file is required.

---

## 3. Required Local Tools

The Ansible VM should provide these commands:

```text
kubectl
kind
docker
jq
curl
openssl
git
make
Go
Python 3
bash
base64
sed
awk
grep
```

Kustomize is used through `kubectl kustomize` and `kubectl apply -k`, so a separate Kustomize binary is not mandatory.

### 3.1 Verify command availability

Run:

```bash
for command_name in \
  kubectl \
  kind \
  docker \
  jq \
  curl \
  openssl \
  git \
  make \
  go \
  python3 \
  bash \
  base64 \
  sed \
  awk \
  grep
do
  if command -v "${command_name}" >/dev/null 2>&1; then
    printf 'PASS: %-10s %s\n' \
      "${command_name}" \
      "$(command -v "${command_name}")"
  else
    printf 'ERROR: required command is missing: %s\n' \
      "${command_name}" >&2
  fi
done
```

Every required command should print `PASS`.

### 3.2 Record tool versions

```bash
kubectl version --client
kind version
docker version --format '{{.Client.Version}}'
jq --version
curl --version | head -n 1
openssl version
git --version
make --version | head -n 1
go version
python3 --version
```

Store version output in troubleshooting notes whenever reproducing the implementation on another machine.

---

## 4. Required Kubernetes Components

The cluster must contain the following components before the OIDC/JWT implementation can be completed:

| Component | Purpose |
|---|---|
| Gateway API CRDs | Defines `Gateway`, `HTTPRoute`, and related APIs |
| Envoy Gateway | Implements Gateway API and `SecurityPolicy` |
| MetalLB | Assigns a reachable LoadBalancer address to Envoy |
| cert-manager | Requests and renews Vault-issued certificates |
| Calico | Provides CNI networking and `NetworkPolicy` enforcement |
| local-path storage | Provides development PVC storage |
| AI Platform Operator CRD | Defines `ModelService` |
| AI Platform Operator controller | Reconciles `ModelService` child resources |
| Vault PKI | Issues certificates for platform hostnames |

### 4.1 Expected namespaces

The completed cluster contains namespaces resembling:

```text
ai-platform
calico-system
cert-manager
default
envoy-gateway-system
gateway-system
keycloak
kube-node-lease
kube-public
kube-system
local-path-storage
metallb-system
tigera-operator
```

The `ai-platform` namespace may not exist during the earliest bootstrap stage. It must exist before applying the sample `ModelService`.

### 4.2 Verify namespaces

```bash
kubectl get namespaces
```

### 4.3 Verify cluster workloads

```bash
kubectl get pods --all-namespaces -o wide
```

Investigate Pods in these states before continuing:

```text
CrashLoopBackOff
ImagePullBackOff
ErrImagePull
Pending
Error
```

A concise unhealthy-Pod check:

```bash
kubectl get pods -A -o json |
jq -r '
  .items[]
  | select(
      .status.phase != "Running"
      and .status.phase != "Succeeded"
    )
  | "\(.metadata.namespace)/\(.metadata.name) phase=\(.status.phase)"
'
```

---

## 5. Kubernetes Context and Cluster Verification

Using the wrong context can make resources appear missing or cause changes in the wrong cluster.

### 5.1 Confirm the current context

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

### 5.2 Display available contexts

```bash
kubectl config get-contexts
```

### 5.3 Select the correct context when required

```bash
kubectl config use-context kind-ai-platform-policy
```

### 5.4 Confirm the kind cluster

```bash
kind get clusters
```

Expected to include:

```text
ai-platform-policy
```

### 5.5 Confirm cluster identity

```bash
kubectl cluster-info
```

```bash
kubectl get nodes -o wide
```

```bash
kubectl version
```

Do not proceed when `kubectl` points to a different cluster or the API server is unavailable.

---

## 6. Gateway API and Envoy Gateway Checks

### 6.1 Verify Gateway API CRDs

```bash
kubectl get crd |
grep 'gateway.networking.k8s.io'
```

Required CRDs include at least:

```text
gatewayclasses.gateway.networking.k8s.io
gateways.gateway.networking.k8s.io
httproutes.gateway.networking.k8s.io
referencegrants.gateway.networking.k8s.io
```

### 6.2 Verify the GatewayClass

```bash
kubectl get gatewayclass
```

The shared Gateway must reference an accepted Envoy Gateway class.

Inspect details:

```bash
kubectl get gatewayclass -o yaml
```

### 6.3 Verify Envoy Gateway controller

```bash
kubectl get deployment,pod \
  -n envoy-gateway-system \
  -o wide
```

### 6.4 Verify the shared Gateway

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o wide
```

Check status conditions:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o json |
jq -r '
  .status.conditions[]? |
  "\(.type)=\(.status) reason=\(.reason) message=\(.message)"
'
```

Expected important condition:

```text
Programmed=True
```

### 6.5 Resolve the current Gateway address

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"

printf 'Gateway address: %s\n' "${GATEWAY_IP}"
```

The implementation originally used:

```text
172.19.255.200
```

### 6.6 Verify the Envoy LoadBalancer Service

```bash
kubectl get service \
  -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=shared-gateway \
  -o wide
```

The Service should expose ports `80` and `443` after both HTTP and HTTPS listeners are configured.

---

## 7. Envoy Gateway SecurityPolicy Prerequisite

JWT authentication and role authorization rely on Envoy Gateway's `SecurityPolicy` custom resource.

### 7.1 Verify the API resource

```bash
kubectl api-resources |
grep -i securitypolic
```

Expected API group and version:

```text
gateway.envoyproxy.io/v1alpha1
```

### 7.2 Verify the CRD

```bash
kubectl get crd \
  securitypolicies.gateway.envoyproxy.io
```

### 7.3 Inspect the installed schema

Always inspect the cluster's installed schema rather than copying fields blindly from another Envoy Gateway release:

```bash
kubectl explain securitypolicy.spec.jwt \
  --api-version=gateway.envoyproxy.io/v1alpha1
```

```bash
kubectl explain securitypolicy.spec.jwt.providers \
  --api-version=gateway.envoyproxy.io/v1alpha1 \
  --recursive
```

```bash
kubectl explain securitypolicy.spec.authorization \
  --api-version=gateway.envoyproxy.io/v1alpha1 \
  --recursive
```

Required capabilities include:

```text
JWT providers
issuer validation
audience validation
remote JWKS
JWT claim authorization
HTTP method matching
default deny behavior
```

---

## 8. cert-manager Prerequisites

### 8.1 Verify cert-manager workloads

```bash
kubectl get deployment,pod \
  -n cert-manager \
  -o wide
```

Expected Deployments:

```text
cert-manager
cert-manager-cainjector
cert-manager-webhook
```

### 8.2 Wait for rollouts

```bash
for deployment_name in \
  cert-manager \
  cert-manager-cainjector \
  cert-manager-webhook
do
  kubectl rollout status \
    "deployment/${deployment_name}" \
    -n cert-manager \
    --timeout=180s
done
```

### 8.3 Verify certificate APIs

```bash
kubectl api-resources |
grep 'cert-manager.io'
```

Required resources include:

```text
certificates
certificaterequests
issuers
clusterissuers
```

### 8.4 Existing Vault-related resources

The implementation reuses these existing resources where applicable:

```text
Namespace: gateway-system
Issuer:    vault-issuer
Secret:    vault-server-ca
```

The dedicated Keycloak HTTPS phase creates a separate issuer:

```text
Issuer: vault-keycloak-issuer
```

Do not overwrite an existing working issuer unless the documented phase explicitly requires it.

---

## 9. Vault Prerequisites

### 9.1 Required Vault environment on the jumpbox

```bash
export VAULT_ADDR='https://vault.platform.local:8200'
export VAULT_CACERT='/home/jumpbox/.vault-tls/vault-ca.crt'
```

Verify the CA path exists:

```bash
test -s "${VAULT_CACERT}" &&
echo 'PASS: Vault CA file exists' ||
{
  echo 'ERROR: Vault CA file is missing' >&2
  exit 1
}
```

### 9.2 Verify Vault availability

```bash
vault status
```

Expected characteristics:

```text
Sealed:     false
HA Enabled: true
Storage:    raft
```

Stop when Vault is sealed. Unseal it according to the platform's Vault operating procedure before continuing.

### 9.3 Verify the PKI mount

```bash
vault secrets list -detailed
```

Expected mount:

```text
pki_modelservice/
```

### 9.4 Verify the kind Kubernetes auth mount

```bash
vault auth list
```

Expected mount:

```text
kubernetes-kind/
```

The separate existing mount below is not replaced:

```text
kubernetes/
```

### 9.5 Required Vault permissions

The administrator executing the Vault configuration scripts must be able to:

```text
create and update PKI roles
create and update Vault policies
create and update Kubernetes auth roles
read relevant mount configuration for validation
```

Never store a Vault administrative token in the repository.

---

## 10. DNS and Hostname Resolution

The implementation uses private development hostnames:

```text
auth.ai-platform.local
fraud-model.local
vault.platform.local
```

### 10.1 `curl --resolve`

Most validation commands avoid changing system DNS by using:

```bash
curl --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/
```

and:

```bash
curl --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  https://fraud-model.local/
```

This forces the hostname to the Gateway address while preserving the correct HTTP `Host` header and TLS SNI value.

### 10.2 Browser-based PKCE login

A browser cannot use `curl --resolve`. The desktop running the browser must resolve:

```text
auth.ai-platform.local
```

For a lab environment, add a hosts-file entry pointing to a reachable Gateway address.

Example:

```text
172.19.255.200 auth.ai-platform.local
```

The exact hosts file depends on the desktop operating system.

### 10.3 Python JWKS validation

The Python JWT validation helper resolves the hostname normally. In the lab, `/etc/hosts` on the Ansible VM may require:

```bash
grep -qE \
  "^[[:space:]]*${GATEWAY_IP}[[:space:]]+auth\.ai-platform\.local([[:space:]]|$)" \
  /etc/hosts ||
printf '%s %s\n' \
  "${GATEWAY_IP}" \
  'auth.ai-platform.local' |
sudo tee -a /etc/hosts
```

### 10.4 Vault hostname

The hosts running Vault CLI or services that contact Vault must resolve:

```text
vault.platform.local
```

Do not replace hostname validation with `curl -k` or `VAULT_SKIP_VERIFY=true`. The implementation depends on trusted CA validation.

---

## 11. Namespace Access to the Shared Gateway

The shared Gateway allows routes only from namespaces carrying this label:

```text
shared-gateway-access=true
```

### 11.1 Verify the Keycloak namespace

```bash
kubectl get namespace keycloak \
  --show-labels
```

### 11.2 Verify the model namespace

```bash
kubectl get namespace ai-platform \
  --show-labels
```

Both namespaces must include:

```text
shared-gateway-access=true
```

Apply when required:

```bash
kubectl label namespace keycloak \
  shared-gateway-access=true \
  --overwrite
```

```bash
kubectl label namespace ai-platform \
  shared-gateway-access=true \
  --overwrite
```

Without this label, `HTTPRoute` objects may not attach to the shared Gateway.

---

## 12. AI Platform Operator Prerequisites

### 12.1 Verify the ModelService CRD

```bash
kubectl get crd \
  modelservices.platform.anselem.dev
```

```bash
kubectl api-resources |
grep -i modelservice
```

### 12.2 Verify the API schema

```bash
kubectl explain modelservice.spec
```

Important sections include:

```text
health
rollout
podDisruptionBudget
networkPolicy
exposure
resources
security
storage
```

After Kubernetes security hardening, verify:

```bash
kubectl explain \
  modelservice.spec.security.automountServiceAccountToken
```

Expected type:

```text
boolean
```

Expected secure default in the generated CRD:

```text
false
```

### 12.3 Verify the operator controller

```bash
kubectl get deployment \
  -A \
  -l control-plane=controller-manager \
  -o custom-columns='NAMESPACE:.metadata.namespace,NAME:.metadata.name,READY:.status.readyReplicas,SERVICEACCOUNT:.spec.template.spec.serviceAccountName'
```

Save its identity:

```bash
OPERATOR_NAMESPACE="$(
  kubectl get deployment \
    -A \
    -l control-plane=controller-manager \
    -o jsonpath='{.items[0].metadata.namespace}'
)"

OPERATOR_DEPLOYMENT="$(
  kubectl get deployment \
    -A \
    -l control-plane=controller-manager \
    -o jsonpath='{.items[0].metadata.name}'
)"

OPERATOR_SERVICE_ACCOUNT="$(
  kubectl get deployment \
    -n "${OPERATOR_NAMESPACE}" \
    "${OPERATOR_DEPLOYMENT}" \
    -o jsonpath='{.spec.template.spec.serviceAccountName}'
)"

printf 'Operator namespace:      %s\n' "${OPERATOR_NAMESPACE}"
printf 'Operator deployment:     %s\n' "${OPERATOR_DEPLOYMENT}"
printf 'Operator ServiceAccount: %s\n' "${OPERATOR_SERVICE_ACCOUNT}"
```

### 12.4 Wait for the operator

```bash
kubectl rollout status \
  "deployment/${OPERATOR_DEPLOYMENT}" \
  -n "${OPERATOR_NAMESPACE}" \
  --timeout=300s
```

### 12.5 Operator ownership boundary

The operator reconciles child resources from an existing `ModelService`:

```text
Deployment
Service
ServiceAccount
PersistentVolumeClaim
PodDisruptionBudget
NetworkPolicy
HTTPRoute
```

It does not recreate:

```text
the kind cluster
the operator installation
the namespace containing the ModelService
the parent ModelService after it is deleted
```

A cluster rebuild therefore requires a bootstrap or GitOps layer to restore the operator and parent custom resources before normal reconciliation resumes.

---

## 13. Sample ModelService Prerequisites

The sample file is:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

Important desired-state values:

```yaml
apiVersion: platform.anselem.dev/v1alpha1
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  image: nginxinc/nginx-unprivileged:1.31-alpine
  replicas: 2
  port: 8080

  exposure:
    enabled: true
    hostname: fraud-model.local
    pathPrefix: /
    gatewayName: shared-gateway
    gatewayNamespace: gateway-system
    gatewaySectionName: fraud-model-https
    gatewayDataPlaneNamespace: envoy-gateway-system

  security:
    runAsNonRoot: true
    runAsUser: 101
    runAsGroup: 101
    fsGroup: 101
    readOnlyRootFilesystem: true
    automountServiceAccountToken: false

  storage:
    enabled: true
    size: 1Gi
    mountPath: /models
```

### 13.1 Create and label the namespace when absent

```bash
kubectl create namespace ai-platform \
  --dry-run=client \
  -o yaml |
kubectl apply -f -
```

```bash
kubectl label namespace ai-platform \
  shared-gateway-access=true \
  app.kubernetes.io/part-of=ai-platform \
  --overwrite
```

### 13.2 Apply the sample only after the operator is ready

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

### 13.3 Verify reconciliation

```bash
kubectl get \
  modelservice,deployment,pod,service,serviceaccount,pvc,pdb,networkpolicy,httproute \
  -n ai-platform
```

The workload route should attach to:

```text
fraud-model-https
```

not the plain HTTP listener.

---

## 14. Secret and Local Artifact Prerequisites

The repository uses two classes of local, non-committed files.

### 14.1 Kustomize secret input files

```text
config/platform/keycloak/.secrets/postgres.env
config/platform/keycloak/.secrets/bootstrap-admin.env
config/platform/keycloak/.secrets/service-client.env
config/platform/keycloak/.secrets/test-users.env
```

### 14.2 Runtime and validation artifacts

```text
.local/keycloak/bootstrap-admin-password
.local/keycloak/ai-platform-service-client-secret
.local/keycloak/auth-ai-platform-root-ca.crt
.local/keycloak/fraud-model-root-ca.crt
.local/keycloak/tokens/
.local/keycloak/test-users/
.local/keycloak/venv/
```

### 14.3 Required `.gitignore` behavior

The repository must ignore at least:

```gitignore
config/platform/keycloak/.secrets/
.local/keycloak/
```

Verify:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/postgres.env \
  config/platform/keycloak/.secrets/bootstrap-admin.env \
  config/platform/keycloak/.secrets/service-client.env \
  config/platform/keycloak/.secrets/test-users.env \
  .local/keycloak/tokens/service-access-token.jwt
```

Confirm none are tracked:

```bash
git ls-files |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/keycloak|\.jwt$|token-response' &&
{
  echo 'ERROR: sensitive material is tracked' >&2
  exit 1
} ||
echo 'PASS: no sensitive local material is tracked'
```

Never commit:

```text
passwords
client secrets
access tokens
refresh tokens
private keys
Vault administrative tokens
Kubernetes ServiceAccount tokens
```

---

## 15. TLS Trust Files

The validation commands use trusted CA files rather than disabling verification.

### 15.1 Keycloak CA file

```text
.local/keycloak/auth-ai-platform-root-ca.crt
```

### 15.2 ModelService CA file

```text
.local/keycloak/fraud-model-root-ca.crt
```

### 15.3 Validate both files

```bash
for ca_file in \
  .local/keycloak/auth-ai-platform-root-ca.crt \
  .local/keycloak/fraud-model-root-ca.crt
do
  test -s "${ca_file}" || {
    echo "ERROR: CA file is missing: ${ca_file}" >&2
    exit 1
  }

  echo "Checking ${ca_file}"
  openssl x509 \
    -in "${ca_file}" \
    -noout \
    -subject \
    -issuer \
    -dates
done
```

Expected CA identity:

```text
AI Platform ModelService Root CA
```

Do not use `curl -k` in validation. A successful request with verification disabled does not prove that the certificate chain is trusted or that the correct security configuration is active.

---

## 16. NetworkPolicy Considerations

Keycloak is protected by ingress `NetworkPolicy` resources.

Expected policies:

```text
keycloak-ingress
keycloak-postgres-ingress
```

### 16.1 Verify policies

```bash
kubectl get networkpolicy \
  -n keycloak
```

### 16.2 Keycloak application ingress

Port `8080` is restricted to the shared-Gateway Envoy data-plane identity. Test Pods used to verify the internal JWKS endpoint must carry the expected Envoy labels:

```text
app.kubernetes.io/component=proxy
app.kubernetes.io/name=envoy
gateway.envoyproxy.io/owning-gateway-name=shared-gateway
gateway.envoyproxy.io/owning-gateway-namespace=gateway-system
```

### 16.3 PostgreSQL ingress

Port `5432` is allowed from the Keycloak workload and should not be generally exposed to other namespaces.

### 16.4 ModelService policy

The sample enables:

```yaml
networkPolicy:
  enabled: true
  allowSameNamespaceIngress: true
  allowDNSEgress: true
```

The generated policy must also permit traffic from the intended Envoy Gateway data plane according to the operator's exposure design.

---

## 17. Browser and SSH Requirements for PKCE

Human authentication uses Authorization Code with PKCE and a loopback callback:

```text
http://127.0.0.1:18080/callback
```

When the PKCE helper runs on the Ansible VM and the browser runs on a desktop, create an SSH local-forward from the desktop:

```bash
ssh \
  -L 18080:127.0.0.1:18080 \
  ansible@192.168.0.58
```

Keep the tunnel open while completing browser login.

The desktop must also:

```text
resolve auth.ai-platform.local to the Gateway address;
trust the AI Platform ModelService Root CA;
reach the Gateway over HTTPS.
```

Direct password grant remains disabled. Do not enable it merely to simplify testing.

---

## 18. Pre-flight Validation Script

Create an optional combined pre-flight script at:

```text
infrastructure/keycloak/scripts/validate-oidc-prerequisites.sh
```

```bash
cat > infrastructure/keycloak/scripts/validate-oidc-prerequisites.sh <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

EXPECTED_CONTEXT="${EXPECTED_CONTEXT:-kind-ai-platform-policy}"
EXPECTED_KIND_CLUSTER="${EXPECTED_KIND_CLUSTER:-ai-platform-policy}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"

required_commands=(
  kubectl
  kind
  docker
  jq
  curl
  openssl
  git
  make
  go
  python3
)

for command_name in "${required_commands[@]}"; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: required command is missing: ${command_name}" >&2
    exit 1
  }
done

echo 'PASS: required local commands are available.'

current_context="$(kubectl config current-context)"

[[ "${current_context}" == "${EXPECTED_CONTEXT}" ]] || {
  echo "ERROR: current context is ${current_context}; expected ${EXPECTED_CONTEXT}." >&2
  exit 1
}

echo "PASS: kubectl context is ${current_context}."

kind get clusters |
grep -qx "${EXPECTED_KIND_CLUSTER}" || {
  echo "ERROR: kind cluster ${EXPECTED_KIND_CLUSTER} was not found." >&2
  exit 1
}

echo "PASS: kind cluster ${EXPECTED_KIND_CLUSTER} exists."

kubectl get crd modelservices.platform.anselem.dev >/dev/null
kubectl get crd gateways.gateway.networking.k8s.io >/dev/null
kubectl get crd httproutes.gateway.networking.k8s.io >/dev/null
kubectl get crd securitypolicies.gateway.envoyproxy.io >/dev/null
kubectl get crd certificates.cert-manager.io >/dev/null

echo 'PASS: required CRDs are installed.'

kubectl rollout status \
  deployment/envoy-gateway \
  -n envoy-gateway-system \
  --timeout=180s

kubectl rollout status \
  deployment/cert-manager \
  -n cert-manager \
  --timeout=180s

kubectl wait \
  --for=condition=Programmed \
  "gateway/${GATEWAY_NAME}" \
  -n "${GATEWAY_NAMESPACE}" \
  --timeout=180s

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    -n "${GATEWAY_NAMESPACE}" \
    -o jsonpath='{.status.addresses[0].value}'
)"

[[ -n "${gateway_ip}" ]] || {
  echo 'ERROR: shared Gateway has no address.' >&2
  exit 1
}

echo "PASS: shared Gateway is Programmed at ${gateway_ip}."

for namespace_name in keycloak ai-platform; do
  if kubectl get namespace "${namespace_name}" >/dev/null 2>&1; then
    label_value="$(
      kubectl get namespace "${namespace_name}" \
        -o jsonpath='{.metadata.labels.shared-gateway-access}'
    )"

    [[ "${label_value}" == 'true' ]] || {
      echo "ERROR: namespace ${namespace_name} lacks shared-gateway-access=true." >&2
      exit 1
    }

    echo "PASS: namespace ${namespace_name} may attach routes to the shared Gateway."
  else
    echo "INFO: namespace ${namespace_name} does not exist yet."
  fi
done

echo
echo 'PASS: OIDC/JWT platform prerequisites validated.'
SCRIPT
```

Make it executable and check its syntax:

```bash
chmod +x \
  infrastructure/keycloak/scripts/validate-oidc-prerequisites.sh

bash -n \
  infrastructure/keycloak/scripts/validate-oidc-prerequisites.sh
```

Run:

```bash
infrastructure/keycloak/scripts/validate-oidc-prerequisites.sh
```

This script is intended as an early environment check. Later phase-specific validation scripts perform deeper checks.

---

## 19. Pre-flight Checklist

Complete this checklist before proceeding to Keycloak installation:

```text
[ ] Repository is available at /mnt/data/ai-platform-operator
[ ] kubectl context is kind-ai-platform-policy
[ ] kind cluster ai-platform-policy exists
[ ] Kubernetes API is reachable
[ ] Required command-line tools are installed
[ ] Gateway API CRDs are installed
[ ] Envoy Gateway controller is Ready
[ ] SecurityPolicy CRD is installed
[ ] shared-gateway is Programmed
[ ] Gateway has a LoadBalancer address
[ ] cert-manager is Ready
[ ] MetalLB is running
[ ] Calico is running
[ ] local-path storage is available
[ ] Vault is reachable and unsealed
[ ] Vault CA file is available on the jumpbox
[ ] pki_modelservice/ exists
[ ] kubernetes-kind/ auth mount exists
[ ] ModelService CRD is installed
[ ] Operator source repository builds successfully
[ ] Keycloak and ai-platform namespaces can use the shared Gateway
[ ] Private hostnames can be resolved for CLI and browser tests
[ ] Local secret and token paths are excluded from Git
[ ] No TLS verification bypass is required
```

---

## 20. Common Prerequisite Failures

### Wrong Kubernetes context

Symptoms:

```text
expected resources are missing
namespaces have unexpected ages
Gateway or operator cannot be found
```

Check:

```bash
kubectl config current-context
kubectl config get-contexts
```

### Recreated kind cluster

Symptoms:

```text
Keycloak or ModelService resources are absent
operator Deployment is absent
only recently reinstalled platform components exist
```

A recreated kind cluster contains none of the previous in-cluster state. Reapply bootstrap manifests, operator installation, and parent custom resources.

### Gateway has no address

Check:

```bash
kubectl describe gateway shared-gateway \
  -n gateway-system

kubectl get service \
  -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=shared-gateway \
  -o wide

kubectl get pods \
  -n metallb-system
```

### SecurityPolicy API missing

Check:

```bash
kubectl get crd \
  securitypolicies.gateway.envoyproxy.io
```

Install or repair the Envoy Gateway version and CRDs that support the policy fields used in this project.

### Vault is sealed

Check:

```bash
vault status
```

Unseal Vault before certificate issuance or renewal can succeed.

### Private hostname does not resolve

Use `curl --resolve` for CLI validation. For browser or Python-based validation, configure hosts-file or internal DNS resolution.

### TLS verification fails

Check:

```text
correct CA file
correct hostname
correct Gateway listener
certificate SAN
certificate validity period
SNI value
```

Do not hide the problem with `-k`.

### HTTPRoute is not accepted

Check:

```text
namespace label shared-gateway-access=true
parent Gateway name and namespace
listener sectionName
listener allowedRoutes configuration
backend Service and port
HTTPRoute status conditions
```

---

## 21. Completion Criteria

This phase is complete when:

```text
[✓] Correct Kubernetes context selected
[✓] kind cluster reachable
[✓] Required local tools available
[✓] Required cluster components healthy
[✓] Gateway API and SecurityPolicy CRDs installed
[✓] shared-gateway Programmed with a reachable address
[✓] cert-manager healthy
[✓] Vault reachable, trusted, and unsealed
[✓] Vault PKI and kind authentication mounts available
[✓] ModelService CRD available
[✓] Namespace Gateway-access labels understood and verified
[✓] Private hostname resolution strategy prepared
[✓] Local secret and token paths excluded from Git
[✓] TLS verification will use trusted CA files
```

The next document is:

```text
02-keycloak-installation.md
```

It covers the declarative PostgreSQL and Keycloak installation, local secret generation, Kustomize configuration, workload rollout, storage, health checks, and installation validation.
