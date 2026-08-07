# AI Platform Operator: Build, Installation, Gateway, HTTPS, and JWT Validation Guide

## Purpose

This guide summarizes the complete flow discussed for the `ai-platform-operator` project:

- building the operator image;
- loading it into a Kind cluster;
- deploying the operator;
- creating the `fraud-model` workload;
- exposing it through Envoy Gateway;
- understanding HTTP versus HTTPS;
- adding redirects and JWT authentication;
- validating human and machine tokens;
- troubleshooting stale variables, expired tokens, Gateway IPs, routes, and operator logs.

---

# 1. Two different container images

## Operator image

```text
ai-platform-operator:dev
```

This image contains the compiled Go controller. It is built from the repository root `Dockerfile` and runs the operator manager.

## Model workload image

The `ModelService` defines a separate image, for example:

```yaml
spec:
  image: nginxinc/nginx-unprivileged:1.31-alpine
```

That image runs in the generated `fraud-model` Pods.

```text
ai-platform-operator:dev
  → operator/controller

nginxinc/nginx-unprivileged:1.31-alpine
  → fraud-model workload
```

The `ReplicaSet/fraud-model-<hash>` is therefore created from the model image, not from the operator Dockerfile.

---

# 2. Build and deploy the operator

```bash
export IMG="ai-platform-operator:dev"

make docker-build IMG="${IMG}"

kind load docker-image "${IMG}" \
  --name ai-platform-policy

make deploy IMG="${IMG}"
```

## Step 1: choose the image name

```bash
export IMG="ai-platform-operator:dev"
```

This sets a shell variable. No image is built yet.

## Step 2: build the image

```bash
make docker-build IMG="${IMG}"
```

The Makefile target is:

```makefile
docker-build:
	$(CONTAINER_TOOL) build -t ${IMG} .
```

It is approximately equivalent to:

```bash
docker build -t ai-platform-operator:dev .
```

Because no `-f` is supplied, Docker uses:

```text
./Dockerfile
```

The Dockerfile compiles the Go source into a manager binary and packages it into the operator image.

```text
cmd/main.go
api/
internal/controller/
go.mod
go.sum
    ↓
Dockerfile
    ↓
manager binary
    ↓
ai-platform-operator:dev
```

Verify:

```bash
docker image ls ai-platform-operator
```

## Step 3: load the image into Kind

```bash
kind load docker-image "${IMG}" \
  --name ai-platform-policy
```

Kind nodes run inside Docker containers. A host-local image is not automatically available inside those nodes.

```text
Host Docker image
    ↓ copied into
Kind node image store
```

## Step 4: deploy the operator

```bash
make deploy IMG="${IMG}"
```

This installs or updates resources such as:

```text
CustomResourceDefinition
ServiceAccount
RBAC
Operator Deployment
```

The Deployment uses:

```yaml
image: ai-platform-operator:dev
```

The manager starts and watches `ModelService` resources.

---

# 3. Installation on other clusters

## Kind

```text
build locally → load into Kind → deploy
```

## Remote or production cluster

Normally push the image to a registry:

```bash
export IMG="ghcr.io/example/ai-platform-operator:v0.1.0"

make docker-build IMG="${IMG}"
make docker-push IMG="${IMG}"
make deploy IMG="${IMG}"
```

No `kind load docker-image` is needed.

For users, a better installation method is a release manifest or Helm chart:

```bash
kubectl apply -f dist/install.yaml
```

or:

```bash
helm install ai-platform-operator <chart>
```

---

# 4. How `fraud-model` is created

After the operator is running:

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

The operator sees:

```yaml
kind: ModelService
metadata:
  name: fraud-model
  namespace: ai-platform
```

It reconciles child resources such as:

```text
Deployment/fraud-model
Service/fraud-model
ReplicaSet/fraud-model-<hash>
Pods
HTTPRoute/fraud-model
NetworkPolicy
PodDisruptionBudget
PersistentVolumeClaim
```

Flow:

```text
ModelService/fraud-model
    ↓
operator reconciler
    ↓
Deployment
    ↓
ReplicaSet
    ↓
Pods
```

---

# 5. Shared Gateway

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

The Gateway provides listeners. It does not itself create the `fraud-model` route.

The `ai-platform` namespace must be allowed:

```bash
kubectl label namespace ai-platform \
  shared-gateway-access=true \
  --overwrite
```

---

# 6. How `fraud-model` gets an HTTPRoute

The `ModelService` exposure configuration contains values such as:

```yaml
exposure:
  enabled: true
  hostname: fraud-model.local
  gatewayName: shared-gateway
  gatewayNamespace: gateway-system
  gatewaySectionName: http
```

The operator creates a route similar to:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: fraud-model
  namespace: ai-platform
spec:
  hostnames:
    - fraud-model.local
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: shared-gateway
      namespace: gateway-system
      sectionName: http
  rules:
    - backendRefs:
        - name: fraud-model
          port: 8080
```

Flow:

```text
http://fraud-model.local
    ↓
shared-gateway / http listener
    ↓
HTTPRoute/fraud-model
    ↓
Service/fraud-model:8080
    ↓
Pods
```

Confirm:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.parentRefs[*].sectionName}{"\n"}'
```

Expected:

```text
http
```

Check status:

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o json |
jq -r '
  .status.parents[].conditions[] |
  "\(.type)=\(.status) reason=\(.reason) message=\(.message)"
'
```

Expected:

```text
Accepted=True
ResolvedRefs=True
```

---

# 7. Why HTTP worked

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"

curl \
  --resolve "fraud-model.local:80:${GATEWAY_IP}" \
  http://fraud-model.local/
```

A `200` response was expected because the model route was attached to:

```yaml
sectionName: http
```

This proved that the Gateway, route, Service, and Pods were working.

---

# 8. Why HTTPS initially failed

The only HTTPS listener was:

```yaml
name: keycloak-https
hostname: auth.ai-platform.local
```

That listener does not match:

```text
fraud-model.local
```

Therefore:

```text
https://fraud-model.local
    ↓
no matching HTTPS listener
    ↓
connection reset / HTTP 000
```

This was not a JWT problem.

---

# 9. Does the model need HTTPS?

Yes, for the final secure design.

JWTs are bearer credentials. They should not be sent over plaintext HTTP.

Final flow:

```text
Client
  │ HTTPS + Bearer JWT
  ▼
Envoy Gateway
  ├── terminates TLS
  ├── validates JWT signature
  ├── checks issuer
  ├── checks audience
  ├── checks expiry
  └── later checks roles
  ▼
fraud-model Service
  ▼
Pods
```

The Pod itself can still use HTTP internally:

```text
Client → HTTPS → Envoy Gateway → HTTP → model Pod
```

---

# 10. Add HTTPS for `fraud-model.local`

You need:

1. a certificate for `fraud-model.local`;
2. an HTTPS listener;
3. the model route attached to that listener.

## Certificate example

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: fraud-model-local
  namespace: gateway-system
spec:
  secretName: fraud-model-local-tls
  dnsNames:
    - fraud-model.local
  issuerRef:
    name: vault-issuer
    kind: ClusterIssuer
```

The issuer name must match the actual cert-manager/Vault configuration.

## HTTPS listener

```yaml
- name: fraud-model-https
  hostname: fraud-model.local
  protocol: HTTPS
  port: 443
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

Multiple port-443 listeners can coexist because SNI selects the hostname:

```text
auth.ai-platform.local  → keycloak-https
fraud-model.local       → fraud-model-https
```

## Move the model route

Update the model exposure:

```yaml
exposure:
  gatewaySectionName: fraud-model-https
```

The generated route must then reference:

```yaml
sectionName: fraud-model-https
```

---

# 11. HTTP-to-HTTPS redirect

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: fraud-model-http-redirect
  namespace: ai-platform
spec:
  hostnames:
    - fraud-model.local
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: shared-gateway
      namespace: gateway-system
      sectionName: http
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      filters:
        - type: RequestRedirect
          requestRedirect:
            scheme: https
            statusCode: 301
```

This does not create HTTPS. It only redirects:

```text
http://fraud-model.local
    ↓
301 Location: https://fraud-model.local
```

The HTTPS listener and HTTPS route must already work.

Final split:

```text
HTTP listener
└── redirect route

HTTPS listener
└── backend route to fraud-model Service
```

Avoid leaving two overlapping routes on the HTTP listener for the same hostname and path.

---

# 12. JWT SecurityPolicy

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
    optional: false
    providers:
      - name: ai-platform-keycloak
        issuer: https://auth.ai-platform.local/realms/ai-platform
        audiences:
          - ai-platform-gateway
        remoteJWKS:
          uri: http://keycloak.keycloak.svc.cluster.local:8080/realms/ai-platform/protocol/openid-connect/certs
        extractFrom:
          headers:
            - name: Authorization
              valuePrefix: "Bearer "
```

Meaning:

```text
optional: false
  → token required

issuer
  → iss must match

audiences
  → aud must contain ai-platform-gateway

remoteJWKS
  → Envoy retrieves Keycloak public signing keys

Authorization: Bearer
  → token extraction contract
```

---

# 13. Kustomization

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - fraud-model-jwt-securitypolicy.yaml
```

Render:

```bash
kubectl kustomize config/platform/authentication
```

Server-side validation:

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform/authentication
```

Apply:

```bash
kubectl apply \
  -k config/platform/authentication
```

---

# 14. Expected JWT validation matrix

| Test | Expected status |
|---|---:|
| No token | `401` |
| Malformed token | `401` |
| Expired token | `401` |
| Wrong issuer | `401` |
| Wrong audience | `401` |
| Valid machine token | `200` |
| Valid human token | `200` |
| Valid token but wrong role, after authorization exists | `403` |

---

# 15. No-token validation

```bash
NO_TOKEN_STATUS="$(
  curl \
    --silent \
    --show-error \
    --output /tmp/no-token-response \
    --write-out '%{http_code}' \
    --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
    --resolve "fraud-model.local:443:${GATEWAY_IP}" \
    https://fraud-model.local/
)"

echo "No-token status: ${NO_TOKEN_STATUS}"
```

Expected:

```text
401
```

This proves HTTPS, route matching, and missing-token enforcement.

---

# 16. Machine-token validation

Generate a fresh token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

Load it:

```bash
SERVICE_ACCESS_TOKEN="$(
  tr -d '\r\n' \
    < .local/keycloak/tokens/service-access-token.jwt
)"
```

Test:

```bash
VALID_MACHINE_STATUS="$(
  curl \
    --silent \
    --show-error \
    --output /tmp/valid-machine-response \
    --write-out '%{http_code}' \
    --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
    --resolve "fraud-model.local:443:${GATEWAY_IP}" \
    --header "Authorization: Bearer ${SERVICE_ACCESS_TOKEN}" \
    https://fraud-model.local/
)"

echo "Valid machine-token status: ${VALID_MACHINE_STATUS}"
cat /tmp/valid-machine-response
```

Expected:

```text
200
```

---

# 17. Human-user token validation

Run PKCE login:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

Reload the token after login:

```bash
unset USER_ACCESS_TOKEN

USER_ACCESS_TOKEN="$(
  tr -d '\r\n' \
    < .local/keycloak/tokens/user-access-token.jwt
)"
```

Test:

```bash
VALID_USER_STATUS="$(
  curl \
    --silent \
    --show-error \
    --output /tmp/valid-user-response \
    --write-out '%{http_code}' \
    --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
    --resolve "fraud-model.local:443:${GATEWAY_IP}" \
    --header "Authorization: Bearer ${USER_ACCESS_TOKEN}" \
    https://fraud-model.local/
)"

echo "Valid user-token status: ${VALID_USER_STATUS}"
cat /tmp/valid-user-response
```

Expected:

```text
200
```

---

# 18. `USER_ACCESS_TOKEN` versus `PAYLOAD`

## `USER_ACCESS_TOKEN`

This is the complete JWT:

```text
header.payload.signature
```

It is sent to Envoy:

```bash
--header "Authorization: Bearer ${USER_ACCESS_TOKEN}"
```

This variable affects authentication.

## `PAYLOAD`

This is only the decoded middle part of the JWT. It is used to inspect claims such as:

```text
iss
aud
azp
preferred_username
iat
exp
roles
```

It is not sent to Envoy.

A request can succeed without creating a `PAYLOAD` variable.

```text
USER_ACCESS_TOKEN → used by curl
PAYLOAD           → optional troubleshooting only
```

---

# 19. Why stale shell variables caused confusion

This command takes a snapshot of the file:

```bash
USER_ACCESS_TOKEN="$(
  cat .local/keycloak/tokens/user-access-token.jwt
)"
```

If the file changes later, the shell variable does not update automatically.

Possible state:

```text
token file          → fresh token
USER_ACCESS_TOKEN   → old expired token
PAYLOAD             → decoded old token
```

Curl sends the value in `USER_ACCESS_TOKEN`, not the file itself.

Therefore an old variable can produce:

```text
401
Jwt is expired
```

---

# 20. Safe variable reset

```bash
unset USER_ACCESS_TOKEN PAYLOAD EXP
```

Then run login and reload:

```bash
infrastructure/keycloak/scripts/pkce-login.py

USER_ACCESS_TOKEN="$(
  tr -d '\r\n' \
    < .local/keycloak/tokens/user-access-token.jwt
)"
```

`PAYLOAD` remains optional.

---

# 21. Decode and inspect a token

```bash
PAYLOAD="$(
  printf '%s' "${USER_ACCESS_TOKEN}" |
  cut -d '.' -f2 |
  tr '_-' '/+' |
  awk '{
    r = length($0) % 4
    if (r == 2) print $0 "=="
    else if (r == 3) print $0 "="
    else print $0
  }' |
  base64 -d 2>/dev/null
)"
```

Inspect:

```bash
echo "${PAYLOAD}" |
jq '{
  iss,
  aud,
  azp,
  preferred_username,
  iat,
  exp,
  realm_access
}'
```

Required values include:

```text
iss = https://auth.ai-platform.local/realms/ai-platform
aud contains ai-platform-gateway
```

---

# 22. Check expiration

```bash
EXP="$(
  echo "${PAYLOAD}" |
  jq -r '.exp'
)"

NOW="$(date +%s)"

echo "Current time: ${NOW}"
echo "Token expiry: ${EXP}"
date -d "@${EXP}"
```

```bash
if (( NOW < EXP )); then
  echo "Token is valid for $((EXP - NOW)) more seconds."
else
  echo "Token expired $((NOW - EXP)) seconds ago."
fi
```

When Envoy returns:

```text
Jwt is expired
```

it proves that the request reached Envoy and the expiry check was enforced.

---

# 23. Compare token variable and file

Without printing the secret:

```bash
printf '%s' "${USER_ACCESS_TOKEN}" |
sha256sum

tr -d '\r\n' \
  < .local/keycloak/tokens/user-access-token.jwt |
sha256sum
```

The hashes should match.

---

# 24. Token file permissions

Restrict token files:

```bash
chmod 600 \
  .local/keycloak/tokens/user-access-token.jwt \
  .local/keycloak/tokens/service-access-token.jwt
```

They do not need execute permission.

---

# 25. Empty `GATEWAY_IP`

This error:

```text
curl: (49) Couldn't parse CURLOPT_RESOLVE entry 'fraud-model.local:443:'
```

means `GATEWAY_IP` was empty.

Reload:

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

Validate:

```bash
if [[ -z "${GATEWAY_IP}" ]]; then
  echo "Error: GATEWAY_IP is empty"
  exit 1
fi

echo "Using Gateway IP: ${GATEWAY_IP}"
```

Expected:

```text
Using Gateway IP: 172.19.255.200
```

This curl error occurs before HTTPS or JWT validation.

---

# 26. Gateway namespace behavior

```bash
kubectl get gateway
```

checks only the current namespace, commonly `default`.

Use:

```bash
kubectl get gateway -A
```

or:

```bash
kubectl get gateway shared-gateway \
  -n gateway-system
```

---

# 27. Operator log command troubleshooting

This error:

```text
arguments in resource/name form must have a single resource and name
```

means the deployment variable was empty, duplicated, or contained multiple names.

Check:

```bash
printf 'OPERATOR_NAMESPACE=<%s>\n' \
  "${OPERATOR_NAMESPACE:-}"

printf 'OPERATOR_DEPLOYMENT=<%s>\n' \
  "${OPERATOR_DEPLOYMENT:-}"
```

Discover:

```bash
kubectl get deployments -A |
grep -Ei 'operator|controller|manager'
```

Example:

```bash
export OPERATOR_NAMESPACE="ai-platform-operator-system"
export OPERATOR_DEPLOYMENT="ai-platform-operator-controller-manager"
```

Then:

```bash
kubectl logs \
  -n "${OPERATOR_NAMESPACE}" \
  "deployment/${OPERATOR_DEPLOYMENT}" \
  -c manager \
  --since=5m
```

---

# 28. Recommended validation order

## Operator and model

```text
1. Build operator image
2. Load into Kind
3. Deploy operator
4. Apply ModelService
5. Verify Deployment, ReplicaSet, Service, and Pods
```

## HTTP baseline

```text
6. Verify Gateway
7. Verify namespace label
8. Verify HTTPRoute sectionName=http
9. Verify Accepted=True
10. Verify ResolvedRefs=True
11. HTTP without JWT → 200
```

## HTTPS

```text
12. Issue fraud-model certificate
13. Add fraud-model-https listener
14. Move backend route to fraud-model-https
15. Verify HTTPS reaches backend
```

## JWT authentication

```text
16. Apply SecurityPolicy
17. No token → 401
18. Malformed token → 401
19. Expired token → 401
20. Wrong issuer/audience → 401
21. Valid machine token → 200
22. Valid human token → 200
```

## Redirect and authorization

```text
23. Add HTTP-to-HTTPS redirect
24. HTTP → 301
25. HTTPS remains functional
26. Add role authorization
27. Wrong role → 403
28. Correct role → 200
```

---

# 29. Reusable human-token validation block

```bash
set -euo pipefail

unset GATEWAY_IP USER_ACCESS_TOKEN PAYLOAD EXP

GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"

if [[ -z "${GATEWAY_IP}" ]]; then
  echo "Error: shared-gateway has no address"
  exit 1
fi

TOKEN_FILE=".local/keycloak/tokens/user-access-token.jwt"

if [[ ! -s "${TOKEN_FILE}" ]]; then
  echo "Error: token file missing or empty: ${TOKEN_FILE}"
  exit 1
fi

USER_ACCESS_TOKEN="$(
  tr -d '\r\n' < "${TOKEN_FILE}"
)"

VALID_USER_STATUS="$(
  curl \
    --silent \
    --show-error \
    --output /tmp/valid-user-response \
    --write-out '%{http_code}' \
    --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
    --resolve "fraud-model.local:443:${GATEWAY_IP}" \
    --header "Authorization: Bearer ${USER_ACCESS_TOKEN}" \
    https://fraud-model.local/
)"

echo "Gateway IP: ${GATEWAY_IP}"
echo "Valid user-token status: ${VALID_USER_STATUS}"
cat /tmp/valid-user-response
```

Expected:

```text
Valid user-token status: 200
```

---

# 30. Final architecture

```text
Developer source
    ↓
Dockerfile builds operator image
    ↓
Image loaded into Kind or pushed to registry
    ↓
Operator Deployment runs
    ↓
ModelService/fraud-model created
    ↓
Operator reconciles resources
    ↓
Deployment / Service / HTTPRoute
    ↓
Shared Envoy Gateway
    ├── HTTP listener
    │     └── redirect to HTTPS
    │
    └── fraud-model HTTPS listener
          ├── TLS certificate
          ├── JWT SecurityPolicy
          ├── issuer validation
          ├── audience validation
          ├── expiry validation
          └── route to fraud-model Service
```

---

# 31. Key conclusions

1. The root `Dockerfile` builds the operator, not the model workload.
2. `kind load docker-image` is a Kind development step.
3. The Gateway provides listeners; the `HTTPRoute` connects a hostname to a backend Service.
4. HTTP initially worked because the route used `sectionName: http`.
5. HTTPS initially failed because only Keycloak had a matching HTTPS listener.
6. An HTTP redirect does not create HTTPS.
7. The final model endpoint should use HTTPS because JWTs are bearer credentials.
8. `USER_ACCESS_TOKEN` is sent to Envoy.
9. `PAYLOAD` is optional and used only for inspection.
10. Updating a token file does not update an existing shell variable.
11. `Jwt is expired` is a successful negative security test.
12. Unsetting and reloading variables prevents stale-token confusion.
