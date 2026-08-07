# Fraud Model HTTPS Exposure

## Purpose

This document explains how the `fraud-model` workload was restored, exposed through the shared Envoy Gateway, secured with a Vault-issued TLS certificate, and redirected from HTTP to HTTPS.

It covers:

- restoration of the AI Platform operator and `ModelService`;
- the operator ownership boundary;
- generation of the workload resources;
- creation of a dedicated HTTPS Gateway listener;
- certificate usage for `fraud-model.local`;
- `HTTPRoute` attachment;
- HTTP-to-HTTPS redirection;
- validation of routing, TLS, SNI, and backend reachability;
- troubleshooting the failures encountered during implementation.

The final request path is:

```text
Client
  ↓
fraud-model.local
  ↓
Gateway/shared-gateway
  ↓
HTTPS listener: fraud-model-https
  ↓
Certificate: fraud-model-local-tls
  ↓
HTTPRoute/fraud-model
  ↓
Service/fraud-model
  ↓
fraud-model Pods
```

---

## Final Environment

```text
Repository:
  /mnt/data/ai-platform-operator

Kubernetes context:
  kind-ai-platform-policy

Cluster:
  ai-platform-policy

Application namespace:
  ai-platform

Gateway namespace:
  gateway-system

Gateway:
  shared-gateway

Gateway address:
  172.19.255.200

Hostname:
  fraud-model.local

HTTPS listener:
  fraud-model-https

TLS Secret:
  fraud-model-local-tls

ModelService:
  ai-platform/fraud-model
```

---

## Why the Fraud Model Route Had to Be Restored

During the implementation, the cluster no longer contained the workload resources expected for the fraud model.

The following resources were missing:

```text
Namespace/ai-platform
ModelService/fraud-model
Deployment/fraud-model
Service/fraud-model
HTTPRoute/fraud-model
ServiceAccount/fraud-model
PersistentVolumeClaim
PodDisruptionBudget
NetworkPolicy
```

The main cause was that the operator and the parent `ModelService` custom resource were not present in the active cluster.

The operator cannot reconcile child resources when either of these is missing:

```text
operator Deployment
parent ModelService resource
```

---

## Operator Ownership Boundary

The operator manages resources derived from the parent `ModelService`.

### The operator can restore

```text
Deployment
Service
HTTPRoute
PersistentVolumeClaim
PodDisruptionBudget
NetworkPolicy
ServiceAccount
status conditions
```

### The operator cannot restore by itself

```text
the Kubernetes cluster
the operator Deployment
the operator namespace
the ModelService CRD
the parent ModelService object
the shared Gateway
cert-manager
Vault configuration
the TLS certificate infrastructure
```

The recovery order therefore matters:

```text
Cluster prerequisites
  ↓
CRD
  ↓
Operator
  ↓
ModelService
  ↓
Generated child resources
```

---

## Verify the Active Cluster

Before restoring resources, confirm the current context:

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

Confirm the kind cluster:

```bash
kind get clusters
```

Expected to include:

```text
ai-platform-policy
```

Check whether the application namespace exists:

```bash
kubectl get namespace ai-platform
```

Check whether the `ModelService` CRD exists:

```bash
kubectl get crd modelservices.platform.anselem.dev
```

Check whether the operator is running:

```bash
kubectl get deployment \
  -A \
  | grep ai-platform-operator
```

Check existing `ModelService` objects:

```bash
kubectl get modelservices.platform.anselem.dev \
  -A
```

---

## Restore the Operator

From the repository root:

```bash
cd /mnt/data/ai-platform-operator
```

Generate code and manifests:

```bash
make generate
make manifests
```

Install the CRD:

```bash
make install
```

Deploy the operator:

```bash
make deploy
```

Verify the operator namespace:

```bash
kubectl get namespace ai-platform-operator-system
```

Verify the controller Deployment:

```bash
kubectl get deployment \
  -n ai-platform-operator-system
```

Wait for rollout:

```bash
kubectl rollout status \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system \
  --timeout=180s
```

Inspect the controller Pods:

```bash
kubectl get pods \
  -n ai-platform-operator-system \
  -o wide
```

Check logs:

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager \
  --tail=100
```

---

## Restore the Application Namespace

Create the namespace when missing:

```bash
kubectl create namespace ai-platform \
  --dry-run=client \
  -o yaml \
  | kubectl apply -f -
```

Verify:

```bash
kubectl get namespace ai-platform
```

---

## ModelService Definition

The restored sample uses:

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

The repository file is:

```text
config/samples/platform_v1alpha1_modelservice.yaml
```

---

## Apply the ModelService

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

Verify the parent resource:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o wide
```

Inspect the full resource:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o yaml
```

---

## Observe Reconciliation

Watch the generated resources:

```bash
kubectl get \
  deployment,service,serviceaccount,pvc,pdb,networkpolicy,httproute \
  -n ai-platform \
  -w
```

Expected resources include:

```text
Deployment/fraud-model
Service/fraud-model
ServiceAccount/fraud-model
HTTPRoute/fraud-model
PersistentVolumeClaim
PodDisruptionBudget
NetworkPolicy
```

Check controller logs during reconciliation:

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager \
  --since=10m
```

---

## Verify the Generated Deployment

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o yaml
```

Check the image:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
```

Expected:

```text
nginxinc/nginx-unprivileged:1.31-alpine
```

Check replica count:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.replicas}{"\n"}'
```

Expected:

```text
2
```

Check container port:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.containers[0].ports[0].containerPort}{"\n"}'
```

Expected:

```text
8080
```

Wait for rollout:

```bash
kubectl rollout status \
  deployment/fraud-model \
  -n ai-platform \
  --timeout=180s
```

---

## Verify the Workload Pods

```bash
kubectl get pods \
  -n ai-platform \
  -l app.kubernetes.io/name=fraud-model \
  -o wide
```

Expected:

```text
2 Running Pods
```

Check Pod readiness:

```bash
kubectl wait \
  --for=condition=Ready \
  pod \
  -n ai-platform \
  -l app.kubernetes.io/name=fraud-model \
  --timeout=180s
```

Inspect one Pod:

```bash
FRAUD_MODEL_POD="$(
  kubectl get pod \
    -n ai-platform \
    -l app.kubernetes.io/name=fraud-model \
    -o jsonpath='{.items[0].metadata.name}'
)"
```

```bash
kubectl describe pod \
  -n ai-platform \
  "${FRAUD_MODEL_POD}"
```

---

## Verify the Service

```bash
kubectl get service fraud-model \
  -n ai-platform \
  -o yaml
```

Check the Service port:

```bash
kubectl get service fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.ports[0].port}{"\n"}'
```

Check the target port:

```bash
kubectl get service fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.ports[0].targetPort}{"\n"}'
```

Expected:

```text
8080
```

Check endpoints:

```bash
kubectl get endpointslice \
  -n ai-platform \
  -l kubernetes.io/service-name=fraud-model \
  -o wide
```

The EndpointSlice should contain the ready Pod addresses.

---

## Verify the Generated HTTPRoute

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

Expected route properties:

```text
hostname:
  fraud-model.local

parent Gateway:
  gateway-system/shared-gateway

sectionName:
  fraud-model-https

backend:
  Service/fraud-model

backend port:
  8080
```

Check parent references:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .spec.parentRefs[*]}{.namespace}{"/"}{.name}{" section="}{.sectionName}{"\n"}{end}'
```

Expected:

```text
gateway-system/shared-gateway section=fraud-model-https
```

Check hostnames:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.hostnames[*]}{"\n"}'
```

Expected:

```text
fraud-model.local
```

---

## Why HTTPS Initially Failed

The first route targeted the shared Gateway, but the Gateway did not yet have a matching HTTPS listener for:

```text
fraud-model.local
```

Observed behavior:

```text
HTTP request:
  reached the workload

HTTPS request:
  connection reset or TLS handshake failure
```

The reason was:

```text
no HTTPS listener matched the requested SNI hostname
```

A Gateway listener must match:

```text
port
protocol
hostname
certificate
route attachment
```

For HTTPS, the hostname is selected during the TLS handshake through Server Name Indication.

---

## Dedicated Fraud Model HTTPS Listener

The shared Gateway was updated with a hostname-specific listener:

```yaml
listeners:
  - name: fraud-model-https
    hostname: fraud-model.local
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
        - group: ""
          kind: Secret
          name: fraud-model-local-tls
    allowedRoutes:
      namespaces:
        from: All
```

The Gateway is:

```text
gateway-system/shared-gateway
```

The listener name is:

```text
fraud-model-https
```

The TLS Secret is:

```text
gateway-system/fraud-model-local-tls
```

The exact namespace of the Secret must match the namespace of the Gateway unless a supported cross-namespace reference mechanism is configured.

---

## Update the Shared Gateway Manifest

The repository file is:

```text
config/platform/shared-gateway.yaml
```

The relevant structure should include:

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
      port: 80
      protocol: HTTP
      allowedRoutes:
        namespaces:
          from: All

    - name: keycloak-https
      hostname: auth.ai-platform.local
      port: 443
      protocol: HTTPS
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: auth-ai-platform-local-tls
      allowedRoutes:
        namespaces:
          from: All

    - name: fraud-model-https
      hostname: fraud-model.local
      port: 443
      protocol: HTTPS
      tls:
        mode: Terminate
        certificateRefs:
          - group: ""
            kind: Secret
            name: fraud-model-local-tls
      allowedRoutes:
        namespaces:
          from: All
```

Apply:

```bash
kubectl apply \
  -f config/platform/shared-gateway.yaml
```

---

## Verify the TLS Secret

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

Expected type:

```text
kubernetes.io/tls
```

Inspect metadata only:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o json |
jq '{
  name: .metadata.name,
  namespace: .metadata.namespace,
  type,
  keys: (.data | keys)
}'
```

Expected keys:

```text
tls.crt
tls.key
ca.crt
```

The presence of `ca.crt` depends on the certificate issuance configuration.

---

## Inspect the Fraud Model Certificate

Extract the certificate locally:

```bash
mkdir -p .local/keycloak
chmod 700 .local/keycloak
```

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system \
  -o jsonpath='{.data.tls\.crt}' |
base64 --decode \
  > .local/keycloak/fraud-model-server.crt
```

Restrict access:

```bash
chmod 600 \
  .local/keycloak/fraud-model-server.crt
```

Inspect:

```bash
openssl x509 \
  -in .local/keycloak/fraud-model-server.crt \
  -noout \
  -subject \
  -issuer \
  -serial \
  -dates \
  -ext subjectAltName
```

Expected:

```text
subject contains fraud-model.local
SAN contains DNS:fraud-model.local
issuer is the AI Platform ModelService Root CA
```

---

## Root CA for Client Validation

The trusted local CA file used during validation is:

```text
.local/keycloak/fraud-model-root-ca.crt
```

Verify:

```bash
test -s .local/keycloak/fraud-model-root-ca.crt &&
echo "PASS: Fraud model root CA exists"
```

Inspect:

```bash
openssl x509 \
  -in .local/keycloak/fraud-model-root-ca.crt \
  -noout \
  -subject \
  -issuer \
  -fingerprint \
  -sha256
```

Expected root common name:

```text
AI Platform ModelService Root CA
```

---

## Wait for Gateway Programming

```bash
kubectl wait \
  --for=condition=Programmed \
  gateway/shared-gateway \
  -n gateway-system \
  --timeout=180s
```

Inspect listeners:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o json |
jq '
  .status.listeners[] |
  {
    name,
    supportedKinds,
    attachedRoutes,
    conditions
  }
'
```

Confirm the listener exists:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o jsonpath='{range .status.listeners[*]}{.name}{"\n"}{end}' |
grep '^fraud-model-https$'
```

Expected:

```text
fraud-model-https
```

---

## Validate Route Attachment

Check route status:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o json |
jq '
  .status.parents[] |
  {
    parentRef,
    controllerName,
    conditions
  }
'
```

Expected conditions include:

```text
Accepted=True
ResolvedRefs=True
```

A compact check:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}{"="}{.status}{" reason="}{.reason}{"\n"}{end}'
```

---

## Resolve the Gateway Address

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

Verify:

```bash
echo "${GATEWAY_IP}"
```

Expected in the documented environment:

```text
172.19.255.200
```

---

## Validate the TLS Handshake

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -CAfile .local/keycloak/fraud-model-root-ca.crt \
  -verify_return_error \
  </dev/null
```

Expected:

```text
Verify return code: 0 (ok)
```

Inspect only the served certificate:

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -showcerts \
  </dev/null \
  2>/dev/null |
openssl x509 \
  -noout \
  -subject \
  -issuer \
  -dates \
  -ext subjectAltName
```

The served certificate must match:

```text
fraud-model.local
```

It must not serve the Keycloak certificate.

---

## Validate HTTPS Reachability

```bash
curl \
  --silent \
  --show-error \
  --fail \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  https://fraud-model.local/ \
  --output /tmp/fraud-model-response.html
```

Check the status code:

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

Expected before JWT protection is attached:

```text
200
```

Inspect the response:

```bash
head -n 20 \
  /tmp/fraud-model-response.html
```

Expected content is the default nginx page.

---

## Why SNI Validation Matters

The Gateway serves multiple HTTPS hostnames on port `443`:

```text
auth.ai-platform.local
fraud-model.local
```

The client sends the requested hostname during the TLS handshake through SNI.

Envoy uses it to select:

```text
the correct listener
the correct certificate
the correct route
```

Without:

```bash
-servername fraud-model.local
```

or:

```bash
--resolve "fraud-model.local:443:${GATEWAY_IP}"
```

the validation may test the wrong certificate or fail to match a listener.

---

# HTTP-to-HTTPS Redirect

## Redirect Architecture

```text
Client sends HTTP
  ↓
Gateway listener on port 80
  ↓
HTTPRoute/fraud-model-http-redirect
  ↓
RequestRedirect filter
  ↓
301 Location: https://fraud-model.local/
```

The redirect route is separate from the operator-generated HTTPS backend route.

---

## Redirect Manifest

Repository file:

```text
config/platform/authentication/fraud-model-http-redirect.yaml
```

Example:

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

Apply:

```bash
kubectl apply \
  -f config/platform/authentication/fraud-model-http-redirect.yaml
```

---

## Validate Redirect Route Status

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform \
  -o yaml
```

Check conditions:

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}{"="}{.status}{" reason="}{.reason}{"\n"}{end}'
```

Expected:

```text
Accepted=True
ResolvedRefs=True
```

---

## Validate HTTP Redirect

```bash
curl \
  --silent \
  --show-error \
  --output /dev/null \
  --dump-header - \
  --resolve "fraud-model.local:80:${GATEWAY_IP}" \
  http://fraud-model.local/
```

Expected:

```text
HTTP/1.1 301 Moved Permanently
Location: https://fraud-model.local/
```

Status-only validation:

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

Follow the redirect:

```bash
curl \
  --silent \
  --show-error \
  --location \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:80:${GATEWAY_IP}" \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  http://fraud-model.local/ \
  --output /tmp/fraud-model-redirected-response.html
```

---

# Validate Backend Reachability Independently

Before blaming the Gateway, validate the Service from inside the cluster.

Create a temporary curl Pod:

```bash
kubectl run fraud-model-service-check \
  -n ai-platform \
  --image=curlimages/curl:8.14.1 \
  --restart=Never \
  --rm \
  -i \
  -- \
  curl \
    --silent \
    --show-error \
    --fail \
    http://fraud-model.ai-platform.svc.cluster.local:8080/
```

Expected:

```text
nginx default page
```

This proves:

```text
Pods are healthy
Service selectors are correct
EndpointSlice is populated
backend port is correct
```

---

# Validate the ModelService Status

Inspect status:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o json |
jq '.status'
```

Check conditions:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.conditions[*]}{.type}{"="}{.status}{" reason="}{.reason}{" message="}{.message}{"\n"}{end}'
```

The exact condition names depend on the controller implementation, but the final state should indicate that the workload is reconciled and ready.

---

# Resource Ownership Validation

Check owner references on the Deployment:

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{range .metadata.ownerReferences[*]}{.apiVersion}{" "}{.kind}{" "}{.name}{" controller="}{.controller}{"\n"}{end}'
```

Expected:

```text
platform.anselem.dev/v1alpha1 ModelService fraud-model controller=true
```

Check the Service:

```bash
kubectl get service fraud-model \
  -n ai-platform \
  -o jsonpath='{range .metadata.ownerReferences[*]}{.kind}{" "}{.name}{"\n"}{end}'
```

Check the HTTPRoute:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .metadata.ownerReferences[*]}{.kind}{" "}{.name}{"\n"}{end}'
```

This confirms that the child resources are controlled by the parent `ModelService`.

---

# Reconciliation Test

Delete one generated child resource:

```bash
kubectl delete service fraud-model \
  -n ai-platform
```

Watch restoration:

```bash
kubectl get service fraud-model \
  -n ai-platform \
  -w
```

The operator should recreate the Service.

Verify:

```bash
kubectl get service fraud-model \
  -n ai-platform
```

Do not delete the parent `ModelService` during this validation unless testing full deletion behavior.

---

# Full Validation Script

Create:

```text
infrastructure/keycloak/scripts/validate-fraud-model-https.sh
```

```bash
cat > infrastructure/keycloak/scripts/validate-fraud-model-https.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

MODEL_NAMESPACE="${MODEL_NAMESPACE:-ai-platform}"
MODEL_NAME="${MODEL_NAME:-fraud-model}"
MODEL_HOSTNAME="${MODEL_HOSTNAME:-fraud-model.local}"

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
HTTPS_LISTENER="${HTTPS_LISTENER:-fraud-model-https}"

CA_FILE="${CA_FILE:-.local/keycloak/fraud-model-root-ca.crt}"

for command_name in kubectl curl openssl jq; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command missing: ${command_name}" >&2
    exit 1
  }
done

[[ -s "${CA_FILE}" ]] || {
  echo "ERROR: CA file is missing: ${CA_FILE}" >&2
  exit 1
}

kubectl get modelservice "${MODEL_NAME}" \
  --namespace "${MODEL_NAMESPACE}" \
  >/dev/null

echo "PASS: ModelService exists."

kubectl rollout status \
  "deployment/${MODEL_NAME}" \
  --namespace "${MODEL_NAMESPACE}" \
  --timeout=180s \
  >/dev/null

echo "PASS: fraud-model Deployment is ready."

service_endpoints="$(
  kubectl get endpointslice \
    --namespace "${MODEL_NAMESPACE}" \
    --selector "kubernetes.io/service-name=${MODEL_NAME}" \
    --output json |
  jq '
    [
      .items[].endpoints[]?
      | select(
          (.conditions.ready // true) == true
        )
    ]
    | length
  '
)"

if [[ "${service_endpoints}" -lt 1 ]]; then
  echo "ERROR: No ready Service endpoints found." >&2
  exit 1
fi

echo "PASS: Service has ready endpoints."

gateway_programmed="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output json |
  jq -r '
    [
      .status.conditions[]?
      | select(
          .type == "Programmed" and
          .status == "True"
        )
    ]
    | length
  '
)"

if [[ "${gateway_programmed}" -lt 1 ]]; then
  echo "ERROR: Gateway is not Programmed." >&2
  exit 1
fi

echo "PASS: Gateway is Programmed."

listener_exists="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output json |
  jq \
    --arg listener "${HTTPS_LISTENER}" \
    '
      [
        .status.listeners[]?
        | select(.name == $listener)
      ]
      | length
    '
)"

if [[ "${listener_exists}" -ne 1 ]]; then
  echo "ERROR: HTTPS listener ${HTTPS_LISTENER} not found." >&2
  exit 1
fi

echo "PASS: HTTPS listener exists."

route_accepted="$(
  kubectl get httproute "${MODEL_NAME}" \
    --namespace "${MODEL_NAMESPACE}" \
    --output json |
  jq -r '
    [
      .status.parents[]?.conditions[]?
      | select(
          .type == "Accepted" and
          .status == "True"
        )
    ]
    | length
  '
)"

if [[ "${route_accepted}" -lt 1 ]]; then
  echo "ERROR: fraud-model HTTPRoute is not accepted." >&2
  exit 1
fi

echo "PASS: HTTPS HTTPRoute is accepted."

redirect_accepted="$(
  kubectl get httproute "${MODEL_NAME}-http-redirect" \
    --namespace "${MODEL_NAMESPACE}" \
    --output json |
  jq -r '
    [
      .status.parents[]?.conditions[]?
      | select(
          .type == "Accepted" and
          .status == "True"
        )
    ]
    | length
  '
)"

if [[ "${redirect_accepted}" -lt 1 ]]; then
  echo "ERROR: Redirect HTTPRoute is not accepted." >&2
  exit 1
fi

echo "PASS: HTTP redirect route is accepted."

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

[[ -n "${gateway_ip}" ]] || {
  echo "ERROR: Gateway address is empty." >&2
  exit 1
}

http_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --resolve "${MODEL_HOSTNAME}:80:${gateway_ip}" \
    "http://${MODEL_HOSTNAME}/"
)"

if [[ "${http_status}" != "301" ]]; then
  echo "ERROR: Expected HTTP 301, got ${http_status}." >&2
  exit 1
fi

echo "PASS: HTTP redirects with status 301."

tls_output="$(
  openssl s_client \
    -connect "${gateway_ip}:443" \
    -servername "${MODEL_HOSTNAME}" \
    -CAfile "${CA_FILE}" \
    -verify_return_error \
    </dev/null \
    2>&1
)"

if ! grep -q \
  'Verify return code: 0 (ok)' \
  <<<"${tls_output}"
then
  echo "ERROR: TLS certificate verification failed." >&2
  printf '%s\n' "${tls_output}" >&2
  exit 1
fi

echo "PASS: TLS certificate is trusted."

https_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    "https://${MODEL_HOSTNAME}/"
)"

case "${https_status}" in
  200|401)
    ;;
  *)
    echo "ERROR: Unexpected HTTPS status ${https_status}." >&2
    exit 1
    ;;
esac

echo "PASS: HTTPS listener is reachable."
echo "INFO: HTTPS status: ${https_status}"
echo "PASS: Fraud model HTTPS exposure validated."
EOF
```

Make executable:

```bash
chmod +x \
  infrastructure/keycloak/scripts/validate-fraud-model-https.sh
```

Validate syntax:

```bash
bash -n \
  infrastructure/keycloak/scripts/validate-fraud-model-https.sh
```

Run:

```bash
infrastructure/keycloak/scripts/validate-fraud-model-https.sh
```

Before JWT authentication is attached, HTTPS may return:

```text
200
```

After JWT authentication is attached, an unauthenticated HTTPS request should return:

```text
401
```

Both confirm that the HTTPS listener and route are reachable.

---

# Troubleshooting

## `HTTPRoute` is missing

Check the parent resource:

```bash
kubectl get modelservice fraud-model \
  -n ai-platform
```

Check the operator:

```bash
kubectl get pods \
  -n ai-platform-operator-system
```

Check controller logs:

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager \
  --tail=200
```

---

## Operator does not recreate child resources

Possible causes:

```text
operator is not running
ModelService CR does not exist
CRD version mismatch
operator RBAC is insufficient
reconciliation error
invalid ModelService spec
```

Inspect events:

```bash
kubectl get events \
  -n ai-platform \
  --sort-by='.lastTimestamp'
```

Inspect controller logs and `ModelService` status.

---

## HTTPS connection reset

Likely cause:

```text
no matching HTTPS listener
```

Check:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o yaml
```

Confirm:

```text
listener name:
  fraud-model-https

hostname:
  fraud-model.local

port:
  443

protocol:
  HTTPS
```

---

## Gateway listener is not Programmed

Inspect conditions:

```bash
kubectl describe gateway shared-gateway \
  -n gateway-system
```

Possible causes:

```text
missing TLS Secret
invalid certificate reference
listener hostname conflict
unsupported listener configuration
GatewayClass not accepted
```

---

## `ResolvedRefs=False`

Check the referenced TLS Secret and backend Service:

```bash
kubectl get secret fraud-model-local-tls \
  -n gateway-system
```

```bash
kubectl get service fraud-model \
  -n ai-platform
```

Also inspect:

```bash
kubectl describe httproute fraud-model \
  -n ai-platform
```

---

## Wrong certificate is served

This usually indicates an SNI or listener mismatch.

Test explicitly:

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

Confirm that the client uses:

```text
fraud-model.local
```

and not only the Gateway IP.

---

## Certificate is not trusted

Use the correct CA:

```text
.local/keycloak/fraud-model-root-ca.crt
```

Do not use the Keycloak root CA unless both certificates share the same trust root.

Inspect the served issuer and compare it with the local CA.

---

## HTTP returns `200` instead of `301`

Check whether the redirect route exists:

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform
```

Confirm that it targets:

```text
sectionName: http
```

and contains:

```text
RequestRedirect
scheme: https
statusCode: 301
```

Also check for another HTTP route matching the same hostname with a conflicting rule.

---

## HTTPS returns `404`

Possible causes:

```text
hostname mismatch
route not attached
path does not match
wrong listener sectionName
backend route absent
```

Inspect:

```bash
kubectl describe httproute fraud-model \
  -n ai-platform
```

---

## HTTPS returns `503`

Possible causes:

```text
Service has no endpoints
Pods are not ready
wrong backend port
NetworkPolicy blocks traffic
```

Check:

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

---

## HTTPS returns `401`

After the JWT `SecurityPolicy` is attached, this is expected for requests without a bearer token.

It proves that:

```text
TLS works
the Gateway listener matches
the route is attached
JWT authentication is active
```

The next document explains authenticated access.

---

## HTTPS returns `403`

This means:

```text
the JWT was accepted
the caller is not authorized for the requested operation
```

Check roles and HTTP-method authorization.

---

## Backend returns `405`

This means the Gateway allowed the request, but nginx does not implement the requested method.

For example:

```text
POST allowed by authorization policy
  ↓
nginx receives POST
  ↓
nginx returns 405
```

This is not an authentication failure.

---

## Service works internally but Gateway fails

This isolates the failure to:

```text
Gateway listener
HTTPRoute
TLS certificate
SecurityPolicy
NetworkPolicy between Envoy and backend
```

Check Envoy Gateway logs and route status.

---

# Files Created or Modified

```text
config/platform/shared-gateway.yaml
config/platform/authentication/fraud-model-http-redirect.yaml
config/platform/authentication/kustomization.yaml
config/samples/platform_v1alpha1_modelservice.yaml
infrastructure/keycloak/scripts/validate-fraud-model-https.sh
.local/keycloak/fraud-model-root-ca.crt
.local/keycloak/fraud-model-server.crt
```

The local certificate copies under:

```text
.local/keycloak/
```

must remain excluded from Git.

---

# Git Safety

Stage the manifests and validation script:

```bash
git add \
  config/platform/shared-gateway.yaml \
  config/platform/authentication/fraud-model-http-redirect.yaml \
  config/platform/authentication/kustomization.yaml \
  config/samples/platform_v1alpha1_modelservice.yaml \
  infrastructure/keycloak/scripts/validate-fraud-model-https.sh
```

Confirm local certificate files are ignored:

```bash
git check-ignore -v \
  .local/keycloak/fraud-model-root-ca.crt \
  .local/keycloak/fraud-model-server.crt
```

Check staged files:

```bash
git diff --cached --name-only
```

The following must not appear:

```text
.local/keycloak/
tls.key
private keys
JWT files
token response files
```

---

# Validation Sequence

Run in this order:

```bash
kubectl config current-context
```

```bash
kubectl rollout status \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system \
  --timeout=180s
```

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

```bash
kubectl wait \
  --for=condition=Programmed \
  gateway/shared-gateway \
  -n gateway-system \
  --timeout=180s
```

```bash
kubectl get httproute \
  -n ai-platform
```

```bash
infrastructure/keycloak/scripts/validate-fraud-model-https.sh
```

---

# Completion Criteria

```text
[✓] AI Platform operator installed and running
[✓] ai-platform namespace present
[✓] ModelService/fraud-model restored
[✓] Deployment reconciled
[✓] two fraud-model Pods ready
[✓] Service created with ready endpoints
[✓] HTTPRoute/fraud-model created
[✓] route targets gateway-system/shared-gateway
[✓] route targets listener fraud-model-https
[✓] Gateway HTTPS listener created for fraud-model.local
[✓] Vault-issued TLS Secret attached
[✓] served certificate SAN contains fraud-model.local
[✓] certificate chain validates against the model root CA
[✓] SNI selects the correct certificate
[✓] HTTPS reaches the fraud-model backend
[✓] HTTP redirects to HTTPS with 301
[✓] operator recreates deleted child resources
[✓] local certificate copies excluded from Git
```

---

# Resulting Request Path

```text
HTTP client
  ↓ http://fraud-model.local
Gateway listener: http
  ↓
HTTPRoute/fraud-model-http-redirect
  ↓ 301
https://fraud-model.local
  ↓ SNI: fraud-model.local
Gateway listener: fraud-model-https
  ↓ TLS termination
Secret/fraud-model-local-tls
  ↓
HTTPRoute/fraud-model
  ↓
Service/fraud-model:8080
  ↓
fraud-model Pods
```

The next security layer attaches Envoy Gateway JWT authentication to:

```text
HTTPRoute/ai-platform/fraud-model
```

After that policy is active, unauthenticated HTTPS requests return:

```text
401 Unauthorized
```

while valid JWTs can proceed to authorization and the backend.
