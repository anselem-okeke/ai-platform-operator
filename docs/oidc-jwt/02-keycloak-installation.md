# OIDC/JWT Security for the AI Platform Operator

## Phase 2 — Keycloak Installation

## 1. Purpose

This document installs Keycloak and its PostgreSQL database inside the Kubernetes cluster as the OpenID Connect identity provider for the AI Platform.

At the end of this phase, the cluster will contain:

```text
Namespace/keycloak
StatefulSet/keycloak-postgres
PersistentVolumeClaim for PostgreSQL
Deployment/keycloak
Service/keycloak
Secret/keycloak-postgres-credentials
Secret/keycloak-bootstrap-admin
NetworkPolicy/keycloak-ingress
NetworkPolicy/keycloak-postgres-ingress
```

This phase installs Keycloak internally. External HTTPS exposure, Vault PKI, cert-manager, the Gateway HTTPS listener, and the Keycloak `HTTPRoute` are documented in:

```text
03-vault-pki-and-keycloak-https.md
```

The final external identity-provider URL will be:

```text
https://auth.ai-platform.local
```

---

## 2. Architecture

```text
Administrator
  ↓ kubectl apply -k
Kubernetes API
  ↓
Namespace/keycloak
  ├── StatefulSet/keycloak-postgres
  │     └── PVC/keycloak-postgres-data
  │
  ├── Deployment/keycloak
  │     ├── application port 8080
  │     └── management port 9000
  │
  ├── Service/keycloak
  ├── Service/keycloak-postgres
  ├── generated credential Secrets
  └── NetworkPolicies
```

Keycloak stores its persistent configuration in PostgreSQL.

Keycloak will later run behind Envoy Gateway:

```text
Client
  ↓ HTTPS
Envoy Gateway
  ↓ HTTP inside the cluster
Service/keycloak:8080
  ↓
Keycloak Pod
```

TLS is intentionally terminated at Envoy Gateway. Keycloak receives HTTP internally, but generates externally correct HTTPS URLs through its hostname and proxy-header settings.

---

## 3. Environment Used

```text
Repository:            /mnt/data/ai-platform-operator
Kubernetes context:    kind-ai-platform-policy
Keycloak namespace:    keycloak
Keycloak image:        quay.io/keycloak/keycloak:26.7.0
PostgreSQL image:      postgres:17.6-alpine
PostgreSQL storage:    5Gi
Keycloak app port:     8080
Keycloak management:   9000
External hostname:     auth.ai-platform.local
External URL:          https://auth.ai-platform.local
```

Run all commands from:

```bash
cd /mnt/data/ai-platform-operator
```

Confirm the active context:

```bash
kubectl config current-context
```

Expected:

```text
kind-ai-platform-policy
```

---

## 4. Files Created in This Phase

```text
config/platform/keycloak/
├── namespace.yaml
├── postgres.yaml
├── keycloak.yaml
├── networkpolicy.yaml
├── kustomization.yaml
└── .secrets/
    ├── postgres.env
    └── bootstrap-admin.env

infrastructure/keycloak/
├── scripts/
│   └── validate-keycloak-installation.sh
└── variables.env.example
```

The `.secrets` files contain real credentials and must never be committed.

---

## 5. Create the Directory Structure

```bash
mkdir -p \
  config/platform/keycloak/.secrets \
  infrastructure/keycloak/scripts

chmod 700 config/platform/keycloak/.secrets
```

---

## 6. Protect Local Secret Files from Git

Ensure `.gitignore` contains:

```gitignore
config/platform/keycloak/.secrets/
.local/keycloak/
```

Apply idempotently:

```bash
grep -qxF 'config/platform/keycloak/.secrets/' .gitignore ||
printf '%s\n' 'config/platform/keycloak/.secrets/' >> .gitignore

grep -qxF '.local/keycloak/' .gitignore ||
printf '%s\n' '.local/keycloak/' >> .gitignore
```

Verify:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets \
  .local/keycloak
```

---

## 7. Generate PostgreSQL Credentials

Generate a strong PostgreSQL password:

```bash
POSTGRES_PASSWORD="$(
  openssl rand -base64 48 |
  tr -d '\n'
)"
```

Create:

```text
config/platform/keycloak/.secrets/postgres.env
```

```bash
cat > config/platform/keycloak/.secrets/postgres.env <<EOF
POSTGRES_DB=keycloak
POSTGRES_USER=keycloak
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
EOF

chmod 600 config/platform/keycloak/.secrets/postgres.env
unset POSTGRES_PASSWORD
```

Do not print the generated password.

---

## 8. Generate the Bootstrap Administrator Credentials

The bootstrap administrator is used only to perform the initial Keycloak administration and configuration.

Generate a password:

```bash
KC_BOOTSTRAP_ADMIN_PASSWORD="$(
  openssl rand -base64 48 |
  tr -d '\n'
)"
```

Create:

```text
config/platform/keycloak/.secrets/bootstrap-admin.env
```

```bash
cat > config/platform/keycloak/.secrets/bootstrap-admin.env <<EOF
KC_BOOTSTRAP_ADMIN_USERNAME=platform-admin
KC_BOOTSTRAP_ADMIN_PASSWORD=${KC_BOOTSTRAP_ADMIN_PASSWORD}
EOF

chmod 600 config/platform/keycloak/.secrets/bootstrap-admin.env
```

Store a restricted local copy for later administration scripts:

```bash
mkdir -p .local/keycloak
chmod 700 .local/keycloak

printf '%s\n' "${KC_BOOTSTRAP_ADMIN_PASSWORD}" \
  > .local/keycloak/bootstrap-admin-password

chmod 600 .local/keycloak/bootstrap-admin-password
unset KC_BOOTSTRAP_ADMIN_PASSWORD
```

---

## 9. Create the Keycloak Namespace Manifest

Create:

```text
config/platform/keycloak/namespace.yaml
```

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: keycloak
  labels:
    app.kubernetes.io/name: keycloak
    app.kubernetes.io/part-of: ai-platform
    shared-gateway-access: "true"
```

The label:

```text
shared-gateway-access=true
```

allows `HTTPRoute` resources in the namespace to attach to the shared Gateway when the Gateway listener uses namespace selection.

---

## 10. Create the PostgreSQL Manifest

Create:

```text
config/platform/keycloak/postgres.yaml
```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: keycloak-postgres
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak-postgres
    app.kubernetes.io/component: database
    app.kubernetes.io/part-of: ai-platform
spec:
  clusterIP: None
  selector:
    app.kubernetes.io/name: keycloak-postgres
  ports:
    - name: postgres
      port: 5432
      targetPort: postgres
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: keycloak-postgres
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak-postgres
    app.kubernetes.io/component: database
    app.kubernetes.io/part-of: ai-platform
spec:
  serviceName: keycloak-postgres
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: keycloak-postgres
  template:
    metadata:
      labels:
        app.kubernetes.io/name: keycloak-postgres
        app.kubernetes.io/component: database
        app.kubernetes.io/part-of: ai-platform
    spec:
      automountServiceAccountToken: false
      securityContext:
        fsGroup: 999
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: postgres
          image: postgres:17.6-alpine
          imagePullPolicy: IfNotPresent
          ports:
            - name: postgres
              containerPort: 5432
          envFrom:
            - secretRef:
                name: keycloak-postgres-credentials
          readinessProbe:
            exec:
              command:
                - /bin/sh
                - -ec
                - pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}"
            initialDelaySeconds: 5
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 6
          livenessProbe:
            exec:
              command:
                - /bin/sh
                - -ec
                - pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}"
            initialDelaySeconds: 30
            periodSeconds: 20
            timeoutSeconds: 5
            failureThreshold: 6
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
            runAsNonRoot: true
            runAsUser: 999
            runAsGroup: 999
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
              subPath: postgres
  volumeClaimTemplates:
    - metadata:
        name: data
        labels:
          app.kubernetes.io/name: keycloak-postgres
          app.kubernetes.io/component: database
          app.kubernetes.io/part-of: ai-platform
      spec:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 5Gi
```

### 10.1 Why a StatefulSet is used

PostgreSQL requires persistent identity and storage. The StatefulSet provides:

- a stable Pod identity;
- a stable PVC;
- controlled restart behavior;
- persistence across Pod recreation.

### 10.2 Development storage assumption

The cluster uses the local-path storage provisioner. This is suitable for the development kind cluster but is not a production high-availability database design.

---

## 11. Create the Keycloak Manifest

Create:

```text
config/platform/keycloak/keycloak.yaml
```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: keycloak
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak
    app.kubernetes.io/component: identity-provider
    app.kubernetes.io/part-of: ai-platform
spec:
  selector:
    app.kubernetes.io/name: keycloak
  ports:
    - name: http
      port: 8080
      targetPort: http
    - name: management
      port: 9000
      targetPort: management
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
  namespace: keycloak
  labels:
    app.kubernetes.io/name: keycloak
    app.kubernetes.io/component: identity-provider
    app.kubernetes.io/part-of: ai-platform
spec:
  replicas: 1
  strategy:
    type: RollingUpdate
  selector:
    matchLabels:
      app.kubernetes.io/name: keycloak
  template:
    metadata:
      labels:
        app.kubernetes.io/name: keycloak
        app.kubernetes.io/component: identity-provider
        app.kubernetes.io/part-of: ai-platform
    spec:
      automountServiceAccountToken: false
      securityContext:
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      initContainers:
        - name: wait-for-postgres
          image: postgres:17.6-alpine
          command:
            - /bin/sh
            - -ec
            - |
              until pg_isready \
                -h keycloak-postgres \
                -p 5432 \
                -U "${POSTGRES_USER}" \
                -d "${POSTGRES_DB}";
              do
                echo "Waiting for PostgreSQL..."
                sleep 3
              done
          envFrom:
            - secretRef:
                name: keycloak-postgres-credentials
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
            runAsNonRoot: true
            runAsUser: 1000
            runAsGroup: 1000
      containers:
        - name: keycloak
          image: quay.io/keycloak/keycloak:26.7.0
          imagePullPolicy: IfNotPresent
          args:
            - start
          ports:
            - name: http
              containerPort: 8080
            - name: management
              containerPort: 9000
          env:
            - name: KC_DB
              value: postgres
            - name: KC_DB_URL
              value: jdbc:postgresql://keycloak-postgres:5432/keycloak
            - name: KC_DB_USERNAME
              valueFrom:
                secretKeyRef:
                  name: keycloak-postgres-credentials
                  key: POSTGRES_USER
            - name: KC_DB_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: keycloak-postgres-credentials
                  key: POSTGRES_PASSWORD
            - name: KC_BOOTSTRAP_ADMIN_USERNAME
              valueFrom:
                secretKeyRef:
                  name: keycloak-bootstrap-admin
                  key: KC_BOOTSTRAP_ADMIN_USERNAME
            - name: KC_BOOTSTRAP_ADMIN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: keycloak-bootstrap-admin
                  key: KC_BOOTSTRAP_ADMIN_PASSWORD
            - name: KC_HOSTNAME
              value: https://auth.ai-platform.local
            - name: KC_HTTP_ENABLED
              value: "true"
            - name: KC_PROXY_HEADERS
              value: xforwarded
            - name: KC_HEALTH_ENABLED
              value: "true"
            - name: KC_METRICS_ENABLED
              value: "true"
          startupProbe:
            httpGet:
              path: /health/started
              port: management
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 30
          readinessProbe:
            httpGet:
              path: /health/ready
              port: management
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 12
          livenessProbe:
            httpGet:
              path: /health/live
              port: management
            initialDelaySeconds: 30
            periodSeconds: 20
            timeoutSeconds: 5
            failureThreshold: 6
          resources:
            requests:
              cpu: 250m
              memory: 512Mi
            limits:
              cpu: "1"
              memory: 1Gi
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
            runAsNonRoot: true
            runAsUser: 1000
            runAsGroup: 1000
```

---

## 12. Key Keycloak Configuration Explained

### 12.1 `KC_HOSTNAME`

```text
KC_HOSTNAME=https://auth.ai-platform.local
```

This sets the public identity of the Keycloak server.

It ensures OIDC discovery, authorization, token, logout, and JWKS-related URLs use the external HTTPS hostname rather than an internal Kubernetes Service name.

### 12.2 `KC_HTTP_ENABLED`

```text
KC_HTTP_ENABLED=true
```

Keycloak receives HTTP traffic from Envoy after TLS termination.

This does not mean users access Keycloak over unencrypted HTTP. The external path is still HTTPS:

```text
Client HTTPS
  ↓
Envoy terminates TLS
  ↓
Internal HTTP to Keycloak:8080
```

### 12.3 `KC_PROXY_HEADERS`

```text
KC_PROXY_HEADERS=xforwarded
```

Keycloak trusts the `X-Forwarded-*` headers supplied by Envoy Gateway so it can determine the original external protocol, host, and port.

This setting must only be used when traffic is restricted to the trusted reverse proxy path.

### 12.4 Health and metrics

```text
KC_HEALTH_ENABLED=true
KC_METRICS_ENABLED=true
```

Keycloak publishes health and metrics on management port `9000`.

The application port remains `8080`.

---

## 13. Create the NetworkPolicies

Create:

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

### 13.1 Security behavior

The policies implement:

```text
Envoy data-plane Pods
  → may access Keycloak port 8080

Approved management test Pods
  → may access Keycloak management port 9000

Keycloak Pods
  → may access PostgreSQL port 5432

Other ingress
  → denied for selected Pods
```

### 13.2 JWKS validation later

When testing Keycloak JWKS from `envoy-gateway-system`, the test Pod must carry the exact Envoy data-plane labels allowed by this policy.

---

## 14. Create the Kustomization

Create:

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

### 14.1 Why the name suffix hash is disabled

The Deployment and StatefulSet reference fixed Secret names:

```text
keycloak-postgres-credentials
keycloak-bootstrap-admin
```

Disabling the Kustomize suffix preserves those names.

For production, a secret-management operator such as External Secrets Operator may replace the local `secretGenerator` workflow.

---

## 15. Create the Example Environment File

Create:

```text
infrastructure/keycloak/variables.env.example
```

```dotenv
# PostgreSQL configuration
POSTGRES_DB=keycloak
POSTGRES_USER=keycloak
POSTGRES_PASSWORD=replace-with-generated-password

# Temporary Keycloak bootstrap administrator
KC_BOOTSTRAP_ADMIN_USERNAME=platform-admin
KC_BOOTSTRAP_ADMIN_PASSWORD=replace-with-generated-password
```

This file is safe to commit because it contains placeholders only.

---

## 16. Validate the Rendered Configuration

Render without applying:

```bash
kubectl kustomize config/platform/keycloak \
  > /tmp/keycloak-rendered.yaml
```

Review the resource kinds and names:

```bash
grep -E '^kind:|^  name:|^  namespace:' \
  /tmp/keycloak-rendered.yaml
```

Run client-side validation:

```bash
kubectl apply \
  --dry-run=client \
  -f /tmp/keycloak-rendered.yaml
```

Run server-side validation:

```bash
kubectl apply \
  --dry-run=server \
  -k config/platform/keycloak
```

Remove the temporary rendered file securely because it contains generated Secret values:

```bash
shred -u /tmp/keycloak-rendered.yaml 2>/dev/null ||
rm -f /tmp/keycloak-rendered.yaml
```

Do not store rendered Kustomize output containing Secret data in Git.

---

## 17. Install Keycloak

Apply the complete Kustomization:

```bash
kubectl apply -k config/platform/keycloak
```

Expected resources include:

```text
namespace/keycloak
secret/keycloak-postgres-credentials
secret/keycloak-bootstrap-admin
service/keycloak-postgres
statefulset.apps/keycloak-postgres
service/keycloak
deployment.apps/keycloak
networkpolicy.networking.k8s.io/keycloak-ingress
networkpolicy.networking.k8s.io/keycloak-postgres-ingress
```

---

## 18. Wait for PostgreSQL

```bash
kubectl rollout status \
  statefulset/keycloak-postgres \
  -n keycloak \
  --timeout=300s
```

Verify the Pod:

```bash
kubectl get pod \
  -n keycloak \
  -l app.kubernetes.io/name=keycloak-postgres \
  -o wide
```

Verify the PVC:

```bash
kubectl get pvc -n keycloak
```

Expected:

```text
STATUS: Bound
CAPACITY: 5Gi
```

---

## 19. Wait for Keycloak

```bash
kubectl rollout status \
  deployment/keycloak \
  -n keycloak \
  --timeout=300s
```

Verify:

```bash
kubectl get deployment,pod,service \
  -n keycloak \
  -o wide
```

Expected:

```text
Deployment/keycloak            1/1 available
StatefulSet/keycloak-postgres  1/1 ready
```

---

## 20. Inspect Logs

### 20.1 PostgreSQL logs

```bash
kubectl logs \
  -n keycloak \
  statefulset/keycloak-postgres \
  --tail=100
```

### 20.2 Keycloak logs

```bash
kubectl logs \
  -n keycloak \
  deployment/keycloak \
  -c keycloak \
  --tail=150
```

Look for successful startup and absence of persistent database, hostname, or proxy configuration errors.

---

## 21. Validate Internal Health

The management port is protected by NetworkPolicy. Create a temporary labeled Pod in the `keycloak` namespace:

```bash
kubectl delete pod keycloak-health-test \
  -n keycloak \
  --ignore-not-found
```

```bash
kubectl run keycloak-health-test \
  -n keycloak \
  --image=curlimages/curl:8.16.0 \
  --restart=Never \
  --labels='keycloak-management-client=true' \
  --command -- \
  curl \
    --silent \
    --show-error \
    --fail \
    http://keycloak:9000/health/ready
```

Wait:

```bash
kubectl wait \
  --for=jsonpath='{.status.phase}'=Succeeded \
  pod/keycloak-health-test \
  -n keycloak \
  --timeout=90s
```

Inspect:

```bash
kubectl logs keycloak-health-test -n keycloak | jq .
```

Expected status:

```json
{
  "status": "UP"
}
```

Clean up:

```bash
kubectl delete pod keycloak-health-test -n keycloak
```

---

## 22. Create the Installation Validation Script

Create:

```text
infrastructure/keycloak/scripts/validate-keycloak-installation.sh
```

```bash
#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-keycloak}"
TIMEOUT="${TIMEOUT:-300s}"

for command_name in kubectl jq; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command is missing: ${command_name}" >&2
    exit 1
  }
done

echo "Checking namespace..."
kubectl get namespace "${NAMESPACE}" >/dev/null

echo "Checking PostgreSQL Secret..."
kubectl get secret keycloak-postgres-credentials \
  --namespace "${NAMESPACE}" >/dev/null

echo "Checking bootstrap administrator Secret..."
kubectl get secret keycloak-bootstrap-admin \
  --namespace "${NAMESPACE}" >/dev/null

echo "Checking PostgreSQL rollout..."
kubectl rollout status \
  statefulset/keycloak-postgres \
  --namespace "${NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking Keycloak rollout..."
kubectl rollout status \
  deployment/keycloak \
  --namespace "${NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking PostgreSQL PVC..."

pvc_phase="$(
  kubectl get pvc \
    --namespace "${NAMESPACE}" \
    --selector app.kubernetes.io/name=keycloak-postgres \
    --output json |
  jq -r '.items[0].status.phase // ""'
)"

if [[ "${pvc_phase}" != "Bound" ]]; then
  echo "ERROR: PostgreSQL PVC is ${pvc_phase:-missing}; expected Bound." >&2
  exit 1
fi

echo "Checking Services..."
kubectl get service keycloak \
  --namespace "${NAMESPACE}" >/dev/null

kubectl get service keycloak-postgres \
  --namespace "${NAMESPACE}" >/dev/null

echo "Checking NetworkPolicies..."
kubectl get networkpolicy keycloak-ingress \
  --namespace "${NAMESPACE}" >/dev/null

kubectl get networkpolicy keycloak-postgres-ingress \
  --namespace "${NAMESPACE}" >/dev/null

echo "Checking secure ServiceAccount-token defaults..."

keycloak_automount="$(
  kubectl get deployment keycloak \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.spec.template.spec.automountServiceAccountToken}'
)"

postgres_automount="$(
  kubectl get statefulset keycloak-postgres \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.spec.template.spec.automountServiceAccountToken}'
)"

[[ "${keycloak_automount}" == "false" ]] || {
  echo "ERROR: Keycloak Pod token automount is not false." >&2
  exit 1
}

[[ "${postgres_automount}" == "false" ]] || {
  echo "ERROR: PostgreSQL Pod token automount is not false." >&2
  exit 1
}

echo
echo "PASS: Keycloak namespace exists."
echo "PASS: Required Secrets exist."
echo "PASS: PostgreSQL is ready."
echo "PASS: Keycloak is ready."
echo "PASS: PostgreSQL PVC is Bound."
echo "PASS: Required Services exist."
echo "PASS: NetworkPolicies exist."
echo "PASS: Kubernetes API token automount is disabled."
echo "PASS: Keycloak installation validation completed."
```

Make executable and validate syntax:

```bash
chmod +x \
  infrastructure/keycloak/scripts/validate-keycloak-installation.sh

bash -n \
  infrastructure/keycloak/scripts/validate-keycloak-installation.sh
```

Run:

```bash
infrastructure/keycloak/scripts/validate-keycloak-installation.sh
```

---

## 23. Validate Secret Safety

Confirm the secret files are ignored:

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/postgres.env \
  config/platform/keycloak/.secrets/bootstrap-admin.env \
  .local/keycloak/bootstrap-admin-password
```

Confirm they are not tracked:

```bash
git ls-files \
  config/platform/keycloak/.secrets/postgres.env \
  config/platform/keycloak/.secrets/bootstrap-admin.env \
  .local/keycloak/bootstrap-admin-password
```

Expected: no output.

Inspect staged file names for accidental secret inclusion:

```bash
git diff --cached --name-only |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/|password$' &&
{
  echo "ERROR: Sensitive file is staged"
  exit 1
} || echo "PASS: No sensitive files are staged"
```

---

## 24. Files Safe to Commit

Stage only non-secret configuration:

```bash
git add \
  .gitignore \
  config/platform/keycloak/namespace.yaml \
  config/platform/keycloak/postgres.yaml \
  config/platform/keycloak/keycloak.yaml \
  config/platform/keycloak/networkpolicy.yaml \
  config/platform/keycloak/kustomization.yaml \
  infrastructure/keycloak/scripts/validate-keycloak-installation.sh \
  infrastructure/keycloak/variables.env.example
```

Review:

```bash
git diff --cached --name-only
```

The following must not appear:

```text
config/platform/keycloak/.secrets/postgres.env
config/platform/keycloak/.secrets/bootstrap-admin.env
.local/keycloak/bootstrap-admin-password
```

---

## 25. Troubleshooting

### 25.1 PostgreSQL Pod remains Pending

Check the PVC and storage class:

```bash
kubectl get pvc -n keycloak
kubectl get storageclass
kubectl describe pvc -n keycloak
```

Likely causes:

- local-path provisioner not installed;
- no default StorageClass;
- storage provisioner not ready.

### 25.2 PostgreSQL fails readiness

```bash
kubectl describe pod \
  -n keycloak \
  -l app.kubernetes.io/name=keycloak-postgres

kubectl logs \
  -n keycloak \
  statefulset/keycloak-postgres
```

Check:

- Secret keys exist;
- database name and username match;
- the data directory permissions are valid;
- the PVC is writable.

### 25.3 Keycloak waits indefinitely for PostgreSQL

Inspect init-container logs:

```bash
KEYCLOAK_POD="$(
  kubectl get pod \
    -n keycloak \
    -l app.kubernetes.io/name=keycloak \
    -o jsonpath='{.items[0].metadata.name}'
)"

kubectl logs \
  -n keycloak \
  "${KEYCLOAK_POD}" \
  -c wait-for-postgres
```

Check the PostgreSQL Service and endpoints:

```bash
kubectl get service,endpoints \
  -n keycloak \
  keycloak-postgres
```

### 25.4 Keycloak reports database authentication errors

Confirm Secret keys without displaying values:

```bash
kubectl get secret keycloak-postgres-credentials \
  -n keycloak \
  -o json |
jq -r '.data | keys[]'
```

Expected:

```text
POSTGRES_DB
POSTGRES_PASSWORD
POSTGRES_USER
```

If credentials were changed after PostgreSQL initialized, the existing database volume may still contain the old database credentials. In a disposable development cluster, delete the StatefulSet PVC only when data loss is acceptable.

### 25.5 Keycloak health probes fail

Check management-port logs and Pod events:

```bash
kubectl describe pod \
  -n keycloak \
  -l app.kubernetes.io/name=keycloak

kubectl logs \
  -n keycloak \
  deployment/keycloak \
  -c keycloak
```

Confirm:

```text
KC_HEALTH_ENABLED=true
management port=9000
probe paths=/health/started, /health/ready, /health/live
```

### 25.6 Internal health-test Pod cannot connect

Check that the temporary Pod has:

```text
keycloak-management-client=true
```

The NetworkPolicy allows port `9000` only from matching Pods in the same namespace.

### 25.7 External HTTPS is not available yet

That is expected at the end of this phase.

This phase installs Keycloak internally. External access requires the next phase:

```text
03-vault-pki-and-keycloak-https.md
```

That phase configures:

- cert-manager ServiceAccount and TokenRequest RBAC;
- Vault PKI role and policy;
- Vault Kubernetes-auth role;
- cert-manager Issuer;
- Certificate and TLS Secret;
- Gateway HTTPS listener;
- Keycloak `HTTPRoute`;
- HTTP-to-HTTPS redirect.

---

## 26. Completion Criteria

This phase is complete when all checks below pass:

```text
[✓] keycloak namespace exists
[✓] PostgreSQL credentials Secret exists
[✓] Bootstrap administrator Secret exists
[✓] PostgreSQL StatefulSet is Ready
[✓] PostgreSQL PVC is Bound with 5Gi storage
[✓] Keycloak Deployment is Ready
[✓] Keycloak Service exposes ports 8080 and 9000
[✓] Keycloak uses PostgreSQL
[✓] Keycloak external hostname is auth.ai-platform.local
[✓] Keycloak HTTP is enabled behind the trusted reverse proxy
[✓] X-Forwarded proxy headers are enabled
[✓] Health and metrics endpoints are enabled
[✓] Keycloak and PostgreSQL Pod token automount is disabled
[✓] NetworkPolicies restrict Keycloak and PostgreSQL ingress
[✓] Installation validation script passes
[✓] Real credentials are excluded from Git
```

---

## 27. Resulting State

At the end of this phase:

```text
Keycloak and PostgreSQL are running inside Kubernetes.
Keycloak is reachable through its internal Service.
Persistent database storage is Bound.
Credential files remain outside Git.
External HTTPS is not yet configured.
```

Continue with:

```text
03-vault-pki-and-keycloak-https.md
```
