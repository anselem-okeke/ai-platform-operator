#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-keycloak}"
TIMEOUT="${TIMEOUT:-300s}"

echo "Checking PostgreSQL StatefulSet..."

kubectl rollout status \
  statefulset/keycloak-postgres \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

echo "Checking Keycloak Deployment..."

kubectl rollout status \
  deployment/keycloak \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

echo "Checking PostgreSQL PVC..."

PVC_PHASE="$(
  kubectl get pvc data-keycloak-postgres-0 \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.status.phase}'
)"

if [[ "${PVC_PHASE}" != "Bound" ]]; then
  echo "ERROR: PostgreSQL PVC is ${PVC_PHASE}, expected Bound." >&2
  exit 1
fi

echo "Checking required Secrets..."

for secret in \
  keycloak-postgres-credentials \
  keycloak-bootstrap-admin
do
  kubectl get secret "${secret}" \
    --namespace "${NAMESPACE}" \
    >/dev/null
done

echo "Checking Keycloak readiness..."

READY_REPLICAS="$(
  kubectl get deployment keycloak \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.status.readyReplicas}'
)"

if [[ "${READY_REPLICAS:-0}" -lt 1 ]]; then
  echo "ERROR: Keycloak has no ready replicas." >&2
  exit 1
fi

echo
kubectl get \
  deployment,statefulset,pod,service,pvc \
  --namespace "${NAMESPACE}"

echo
echo "PASS: PostgreSQL is ready."
echo "PASS: PostgreSQL storage is Bound."
echo "PASS: Keycloak is ready."
echo "PASS: Required runtime Secrets exist."
