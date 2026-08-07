# Hardened Operator Deployment and Validation

This runbook builds, loads, deploys, and validates the hardened AI Platform Operator in the `ai-platform-policy` kind cluster.

## 1. Set variables

```bash
export IMG="ai-platform-operator:rbac-hardened"
export OPERATOR_NAMESPACE="ai-platform-operator-system"
export OPERATOR_SERVICE_ACCOUNT="ai-platform-operator-controller-manager"
export OPERATOR_DEPLOYMENT="ai-platform-operator-controller-manager"
```

## 2. Test, build and load the image

```bash
make test
make docker-build IMG="${IMG}"

kind load docker-image "${IMG}" \
  --name ai-platform-policy
```

## 3. Deploy the regenerated CRD, RBAC and operator

```bash
make deploy IMG="${IMG}"

kubectl rollout status \
  deployment/"${OPERATOR_DEPLOYMENT}" \
  --namespace "${OPERATOR_NAMESPACE}" \
  --timeout=300s
```

If the same image tag was previously deployed, restart the Deployment and wait again:

```bash
kubectl rollout restart \
  deployment/"${OPERATOR_DEPLOYMENT}" \
  --namespace "${OPERATOR_NAMESPACE}"

kubectl rollout status \
  deployment/"${OPERATOR_DEPLOYMENT}" \
  --namespace "${OPERATOR_NAMESPACE}" \
  --timeout=300s
```

## 4. Verify the new CRD field

```bash
kubectl explain \
  modelservice.spec.security.automountServiceAccountToken
```

Expected type: `boolean`.

## 5. Verify hardened RBAC

```bash
kubectl auth can-i create modelservices.platform.anselem.dev \
  --namespace ai-platform \
  --as "system:serviceaccount:${OPERATOR_NAMESPACE}:${OPERATOR_SERVICE_ACCOUNT}"

kubectl auth can-i delete modelservices.platform.anselem.dev \
  --namespace ai-platform \
  --as "system:serviceaccount:${OPERATOR_NAMESPACE}:${OPERATOR_SERVICE_ACCOUNT}"

kubectl auth can-i update modelservices.platform.anselem.dev \
  --subresource=status \
  --namespace ai-platform \
  --as "system:serviceaccount:${OPERATOR_NAMESPACE}:${OPERATOR_SERVICE_ACCOUNT}"

kubectl auth can-i create serviceaccounts \
  --subresource=token \
  --namespace ai-platform \
  --as "system:serviceaccount:${OPERATOR_NAMESPACE}:${OPERATOR_SERVICE_ACCOUNT}"
```

Expected results, in order:

```text
no
no
yes
no
```

## 6. Apply and inspect the ModelService

```bash
kubectl apply \
  -f config/samples/platform_v1alpha1_modelservice.yaml

kubectl get modelservice fraud-model \
  --namespace ai-platform \
  -o wide

kubectl describe modelservice fraud-model \
  --namespace ai-platform
```

## 7. Confirm workload token hardening

```bash
kubectl get deployment fraud-model \
  --namespace ai-platform \
  -o jsonpath='{.spec.template.spec.automountServiceAccountToken}{"\n"}'
```

Expected:

```text
false
```

## 8. Check the operator image and logs

```bash
kubectl get deployment "${OPERATOR_DEPLOYMENT}" \
  --namespace "${OPERATOR_NAMESPACE}" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="manager")].image}{"\n"}'

kubectl logs \
  --namespace "${OPERATOR_NAMESPACE}" \
  deployment/"${OPERATOR_DEPLOYMENT}" \
  -c manager \
  --since=5m | \
grep -Ei 'forbidden|unauthorized|cannot|error' || true
```

No persistent RBAC-denied errors should appear.

## Confirmed validation results

- The new CRD field was available as a boolean.
- ModelService `create` and `delete` returned `no` for the operator ServiceAccount.
- ModelService status `update` returned `yes`.
- ServiceAccount token creation returned `no`.
- The `fraud-model` sample was configured successfully.
