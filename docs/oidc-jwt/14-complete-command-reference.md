# Complete Command Reference

## Repository

```bash
cd /mnt/data/ai-platform-operator
```

---

## Cluster Context

```bash
kubectl config current-context
kind get clusters
kubectl cluster-info
kubectl get nodes -o wide
kubectl get pods -A
```

---

## Operator

```bash
make generate
make manifests
make test
make install
make deploy
```

```bash
kubectl rollout status \
  deployment/ai-platform-operator-controller-manager \
  -n ai-platform-operator-system \
  --timeout=180s
```

```bash
kubectl logs \
  -n ai-platform-operator-system \
  deployment/ai-platform-operator-controller-manager \
  -c manager \
  --tail=200
```

---

## ModelService

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml
```

```bash
kubectl get modelservice fraud-model \
  -n ai-platform \
  -o yaml
```

```bash
kubectl rollout status \
  deployment/fraud-model \
  -n ai-platform \
  --timeout=180s
```

```bash
kubectl get \
  deployment,service,serviceaccount,pvc,pdb,networkpolicy,httproute \
  -n ai-platform
```

---

## Shared Gateway

```bash
kubectl apply \
  -f config/platform/shared-gateway.yaml
```

```bash
kubectl wait \
  --for=condition=Programmed \
  gateway/shared-gateway \
  -n gateway-system \
  --timeout=180s
```

```bash
kubectl get gateway shared-gateway \
  -n gateway-system \
  -o yaml
```

---

## Gateway Address

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

---

## Keycloak Installation

```bash
kubectl apply \
  -k config/platform/keycloak
```

```bash
kubectl rollout status \
  statefulset/keycloak-postgres \
  -n keycloak \
  --timeout=180s
```

```bash
kubectl rollout status \
  deployment/keycloak \
  -n keycloak \
  --timeout=180s
```

```bash
infrastructure/keycloak/scripts/validate-keycloak-installation.sh
```

---

## Keycloak HTTPS

```bash
kubectl get certificate,issuer \
  -A
```

```bash
kubectl get httproute \
  -n keycloak
```

```bash
infrastructure/keycloak/scripts/validate-keycloak-https.sh
```

---

## OIDC Discovery

```bash
curl \
  --silent \
  --show-error \
  --fail \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/realms/ai-platform/.well-known/openid-configuration |
jq .
```

---

## Configure Realm and Clients

```bash
infrastructure/keycloak/scripts/configure-keycloak-realm-clients.sh
```

---

## Configure Roles and Users

```bash
set -a
source infrastructure/keycloak/config/authorization.env
source config/platform/keycloak/.secrets/test-users.env
set +a
```

```bash
infrastructure/keycloak/scripts/configure-keycloak-roles-users.sh
```

```bash
unset VIEWER_PASSWORD
unset DEPLOYER_PASSWORD
unset ADMIN_PASSWORD
```

---

## Machine Token

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/service-access-token.jwt
```

```bash
infrastructure/keycloak/scripts/validate-machine-token.sh
```

---

## JWT Signature Validation

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

## PKCE Login

```bash
ssh \
  -L 18080:127.0.0.1:18080 \
  ansible@192.168.0.58
```

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

---

## Fraud Model HTTPS

```bash
kubectl apply \
  -f config/platform/authentication/fraud-model-http-redirect.yaml
```

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o yaml
```

```bash
kubectl get httproute fraud-model-http-redirect \
  -n ai-platform \
  -o yaml
```

```bash
openssl s_client \
  -connect "${GATEWAY_IP}:443" \
  -servername fraud-model.local \
  -CAfile .local/keycloak/fraud-model-root-ca.crt \
  -verify_return_error \
  </dev/null
```

```bash
curl \
  --cacert .local/keycloak/fraud-model-root-ca.crt \
  --resolve "fraud-model.local:443:${GATEWAY_IP}" \
  https://fraud-model.local/
```

---

## HTTP Redirect

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
301
```

---

## JWT SecurityPolicy

```bash
kubectl apply \
  -f config/platform/authentication/fraud-model-jwt-securitypolicy.yaml
```

```bash
kubectl get securitypolicy fraud-model-jwt-authentication \
  -n ai-platform \
  -o yaml
```

```bash
infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh
```

---

## Role Authorization

```bash
infrastructure/keycloak/scripts/validate-gateway-role-authorization.sh
```

```bash
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh
```

---

## Kubernetes Security

```bash
kubectl explain \
  modelservice.spec.security.automountServiceAccountToken
```

```bash
kubectl get serviceaccount fraud-model \
  -n ai-platform \
  -o jsonpath='{.automountServiceAccountToken}{"\n"}'
```

```bash
kubectl get deployment fraud-model \
  -n ai-platform \
  -o jsonpath='{.spec.template.spec.automountServiceAccountToken}{"\n"}'
```

```bash
infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh
```

---

## Operator RBAC Checks

```bash
OPERATOR_SA="system:serviceaccount:ai-platform-operator-system:ai-platform-operator-controller-manager"
```

```bash
kubectl auth can-i create modelservices.platform.anselem.dev \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

```bash
kubectl auth can-i delete modelservices.platform.anselem.dev \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

```bash
kubectl auth can-i update modelservices.platform.anselem.dev/status \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

```bash
kubectl auth can-i create serviceaccounts/token \
  --as="${OPERATOR_SA}" \
  -n ai-platform
```

---

## Workload RBAC Checks

```bash
WORKLOAD_SA="system:serviceaccount:ai-platform:fraud-model"
```

```bash
kubectl auth can-i list pods \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

```bash
kubectl auth can-i get secrets \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

```bash
kubectl auth can-i create serviceaccounts/token \
  --as="${WORKLOAD_SA}" \
  -n ai-platform
```

---

## End-to-End Validation

```bash
infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

---

## Route Status

```bash
kubectl get httproute \
  -A
```

```bash
kubectl get httproute fraud-model \
  -n ai-platform \
  -o jsonpath='{range .status.parents[*].conditions[*]}{.type}{"="}{.status}{" reason="}{.reason}{"\n"}{end}'
```

---

## SecurityPolicy Status

```bash
kubectl get securitypolicy fraud-model-jwt-authentication \
  -n ai-platform \
  -o json |
jq '
  .status.ancestors[]?.conditions[]? |
  {
    type,
    status,
    reason,
    message
  }
'
```

---

## JWKS Check

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

---

## Git Safety

```bash
git status --short
git diff --name-only
git diff
```

```bash
git check-ignore -v \
  config/platform/keycloak/.secrets/test-users.env \
  .local/keycloak/tokens/service-access-token.jwt
```

```bash
git ls-files |
grep -E \
  '(^|/)\.secrets/|(^|/)\.local/keycloak/|\.jwt$|token-response|password'
```

```bash
git diff --cached --name-status
git diff --cached --stat
git diff --cached
```

---

## Script Validation

```bash
find infrastructure/keycloak/scripts \
  -type f \
  -name '*.sh' \
  -print0 |
while IFS= read -r -d '' script; do
  bash -n "${script}"
done
```

```bash
find infrastructure/keycloak/scripts \
  -type f \
  -name '*.py' \
  -print0 |
while IFS= read -r -d '' script; do
  python3 -m py_compile "${script}"
done
```

```bash
find infrastructure/keycloak \
  -type d \
  -name '__pycache__' \
  -prune \
  -exec rm -rf {} +
```

---

## Go Validation

```bash
gofmt -w \
  api/v1alpha1/modelservice_types.go \
  internal/controller/modelservice_controller.go \
  internal/controller/modelservice_controller_test.go
```

```bash
make generate
make manifests
make test
```

---

## Commit

```bash
git add \
  api \
  internal/controller \
  config/crd \
  config/rbac \
  config/platform \
  config/samples \
  infrastructure/keycloak \
  docs/oidc-jwt \
  .gitignore
```

```bash
git commit \
  -m "feat: add Keycloak OIDC and gateway authorization"
```

```bash
git show \
  --stat \
  --oneline \
  HEAD
```

```bash
git push -u origin "$(git branch --show-current)"
```

---

## Complete Validation Sequence

```bash
infrastructure/keycloak/scripts/validate-keycloak-installation.sh

infrastructure/keycloak/scripts/validate-keycloak-https.sh

infrastructure/keycloak/scripts/get-machine-token.sh

infrastructure/keycloak/scripts/validate-machine-token.sh

infrastructure/keycloak/scripts/validate-gateway-jwt-authentication.sh

infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh

infrastructure/keycloak/scripts/validate-kubernetes-permissions.sh

infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```
