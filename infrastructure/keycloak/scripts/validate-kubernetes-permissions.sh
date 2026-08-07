#!/usr/bin/env bash
set -euo pipefail

WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE:-ai-platform}"
WORKLOAD_NAME="${WORKLOAD_NAME:-fraud-model}"

OPERATOR_NAMESPACE="${OPERATOR_NAMESPACE:-}"
OPERATOR_SERVICE_ACCOUNT="${OPERATOR_SERVICE_ACCOUNT:-}"

for command_name in kubectl jq; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command missing: ${command_name}" >&2
    exit 1
  }
done

if [[ -z "${OPERATOR_NAMESPACE}" ]]; then
  OPERATOR_NAMESPACE="$(
    kubectl get deployment \
      -A \
      -l control-plane=controller-manager \
      -o jsonpath='{.items[0].metadata.namespace}'
  )"
fi

if [[ -z "${OPERATOR_SERVICE_ACCOUNT}" ]]; then
  OPERATOR_SERVICE_ACCOUNT="$(
    kubectl get deployment \
      -n "${OPERATOR_NAMESPACE}" \
      -l control-plane=controller-manager \
      -o jsonpath='{.items[0].spec.template.spec.serviceAccountName}'
  )"
fi

[[ -n "${OPERATOR_NAMESPACE}" ]] || {
  echo "ERROR: Operator namespace was not resolved." >&2
  exit 1
}

[[ -n "${OPERATOR_SERVICE_ACCOUNT}" ]] || {
  echo "ERROR: Operator ServiceAccount was not resolved." >&2
  exit 1
}

workload_identity="system:serviceaccount:${WORKLOAD_NAMESPACE}:${WORKLOAD_NAME}"
operator_identity="system:serviceaccount:${OPERATOR_NAMESPACE}:${OPERATOR_SERVICE_ACCOUNT}"

require_yes() {
  local description="$1"
  shift

  local result
  result="$(kubectl auth can-i "$@")"

  if [[ "${result}" != "yes" ]]; then
    echo "ERROR: ${description}: expected yes, got ${result}." >&2
    exit 1
  fi

  echo "PASS: ${description}"
}

require_no() {
  local description="$1"
  shift

  local result
  result="$(kubectl auth can-i "$@")"

  if [[ "${result}" != "no" ]]; then
    echo "ERROR: ${description}: expected no, got ${result}." >&2
    exit 1
  fi

  echo "PASS: ${description}"
}

echo "Checking workload credential mounting..."

service_account_automount="$(
  kubectl get serviceaccount "${WORKLOAD_NAME}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --output jsonpath='{.automountServiceAccountToken}'
)"

pod_automount="$(
  kubectl get deployment "${WORKLOAD_NAME}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --output jsonpath='{.spec.template.spec.automountServiceAccountToken}'
)"

[[ "${service_account_automount}" == "false" ]] || {
  echo "ERROR: Workload ServiceAccount automount is not false." >&2
  exit 1
}

[[ "${pod_automount}" == "false" ]] || {
  echo "ERROR: Workload Pod automount is not false." >&2
  exit 1
}

echo "PASS: Workload ServiceAccount token automount is disabled."
echo "PASS: Workload Pod token automount is disabled."

echo
echo "Checking workload API permissions..."

for resource in \
  pods \
  services \
  secrets \
  configmaps \
  deployments.apps \
  modelservices.platform.anselem.dev
do
  require_no \
    "Workload cannot list ${resource}" \
    list "${resource}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --as "${workload_identity}"
done

require_no \
  "Workload cannot read Secrets" \
  get secrets \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${workload_identity}"

require_no \
  "Workload cannot create Pods" \
  create pods \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${workload_identity}"

require_no \
  "Workload cannot request ServiceAccount tokens" \
  create serviceaccounts/token \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${workload_identity}"

echo
echo "Checking required operator permissions..."

for resource in \
  deployments.apps \
  services \
  serviceaccounts \
  persistentvolumeclaims \
  poddisruptionbudgets.policy \
  networkpolicies.networking.k8s.io \
  httproutes.gateway.networking.k8s.io
do
  require_yes \
    "Operator can create ${resource}" \
    create "${resource}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --as "${operator_identity}"
done

require_yes \
  "Operator can update ModelService status" \
  update modelservices.platform.anselem.dev/status \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${operator_identity}"

echo
echo "Checking prohibited operator permissions..."

require_no \
  "Operator cannot read Secrets" \
  get secrets \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${operator_identity}"

require_no \
  "Operator cannot list Nodes" \
  list nodes \
  --as "${operator_identity}"

require_no \
  "Operator cannot create ClusterRoles" \
  create clusterroles.rbac.authorization.k8s.io \
  --as "${operator_identity}"

require_no \
  "Operator cannot create RoleBindings" \
  create rolebindings.rbac.authorization.k8s.io \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${operator_identity}"

require_no \
  "Operator cannot request arbitrary ServiceAccount tokens" \
  create serviceaccounts/token \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --as "${operator_identity}"

echo
echo "PASS: Kubernetes workload and operator permissions are restricted."
echo "Operator identity: ${operator_identity}"
echo "Workload identity: ${workload_identity}"
