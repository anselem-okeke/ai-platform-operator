##!/usr/bin/env bash
#set -uo pipefail
#
#NAMESPACE="${MODEL_SERVICE_NAMESPACE:-ai-platform}"
#SERVICE_ACCOUNT="${API_SERVICE_ACCOUNT:-ai-platform-api}"
#
#IDENTITY="system:serviceaccount:${NAMESPACE}:${SERVICE_ACCOUNT}"
#
#expect_yes() {
#  local description="$1"
#  shift
#
#  result="$(
#    kubectl auth can-i \
#      "$@" \
#      --as="${IDENTITY}"
#  )"
#
#  if [[ "${result}" != "yes" ]]; then
#    echo "FAIL: ${description}"
#    echo "Expected: yes"
#    echo "Actual:   ${result}"
#    exit 1
#  fi
#
#  echo "PASS: ${description}"
#}
#
#expect_no() {
#  local description="$1"
#  shift
#
#  result="$(
#    kubectl auth can-i \
#      "$@" \
#      --as="${IDENTITY}"
#  )"
#
#  if [[ "${result}" != "no" ]]; then
#    echo "FAIL: ${description}"
#    echo "Expected: no"
#    echo "Actual:   ${result}"
#    exit 1
#  fi
#
#  echo "PASS: ${description}"
#}
#
#echo "Validating Kubernetes permissions for:"
#echo "${IDENTITY}"
#echo
#
#expect_yes \
#  "can get ModelServices" \
#  get modelservices.platform.anselem.dev \
#  -n "${NAMESPACE}"
#
#expect_yes \
#  "can list ModelServices" \
#  list modelservices.platform.anselem.dev \
#  -n "${NAMESPACE}"
#
#expect_yes \
#  "can create ModelServices" \
#  create modelservices.platform.anselem.dev \
#  -n "${NAMESPACE}"
#
#expect_yes \
#  "can update ModelServices" \
#  update modelservices.platform.anselem.dev \
#  -n "${NAMESPACE}"
#
#expect_yes \
#  "can patch ModelServices" \
#  patch modelservices.platform.anselem.dev \
#  -n "${NAMESPACE}"
#
#expect_yes \
#  "can delete ModelServices" \
#  delete modelservices.platform.anselem.dev \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot update ModelService status" \
#  update modelservices.platform.anselem.dev/status \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot read Secrets" \
#  get secrets \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot create Deployments" \
#  create deployments.apps \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot create Services" \
#  create services \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot create HTTPRoutes" \
#  create httproutes.gateway.networking.k8s.io \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot create Roles" \
#  create roles.rbac.authorization.k8s.io \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot create RoleBindings" \
#  create rolebindings.rbac.authorization.k8s.io \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot request ServiceAccount tokens" \
#  create serviceaccounts/token \
#  -n "${NAMESPACE}"
#
#expect_no \
#  "cannot list Nodes" \
#  list nodes
#
#expect_no \
#  "cannot manage ModelServices in default namespace" \
#  list modelservices.platform.anselem.dev \
#  -n default
#
#echo
#echo "PASS: AI Platform API RBAC validation complete."

#!/usr/bin/env bash
set -uo pipefail

NAMESPACE="${MODEL_SERVICE_NAMESPACE:-ai-platform}"
SERVICE_ACCOUNT="${API_SERVICE_ACCOUNT:-ai-platform-api}"

IDENTITY="system:serviceaccount:${NAMESPACE}:${SERVICE_ACCOUNT}"

FAILURES=0

expect_yes() {
  local description="$1"
  shift

  local result

  result="$(
    kubectl auth can-i \
      "$@" \
      --as="${IDENTITY}"
  )"

  if [[ "${result}" != "yes" ]]; then
    echo "FAIL: ${description}"
    echo "Expected: yes"
    echo "Actual:   ${result}"
    echo
    FAILURES=$((FAILURES + 1))
    return
  fi

  echo "PASS: ${description}"
}

expect_no() {
  local description="$1"
  shift

  local result

  result="$(
    kubectl auth can-i \
      "$@" \
      --as="${IDENTITY}"
  )"

  if [[ "${result}" != "no" ]]; then
    echo "FAIL: ${description}"
    echo "Expected: no"
    echo "Actual:   ${result}"
    echo
    FAILURES=$((FAILURES + 1))
    return
  fi

  echo "PASS: ${description}"
}

echo "Validating Kubernetes permissions for:"
echo "${IDENTITY}"
echo

expect_yes \
  "can get ModelServices" \
  get modelservices.platform.anselem.dev \
  -n "${NAMESPACE}"

expect_yes \
  "can list ModelServices" \
  list modelservices.platform.anselem.dev \
  -n "${NAMESPACE}"

expect_yes \
  "can create ModelServices" \
  create modelservices.platform.anselem.dev \
  -n "${NAMESPACE}"

expect_yes \
  "can update ModelServices" \
  update modelservices.platform.anselem.dev \
  -n "${NAMESPACE}"

expect_yes \
  "can patch ModelServices" \
  patch modelservices.platform.anselem.dev \
  -n "${NAMESPACE}"

expect_yes \
  "can delete ModelServices" \
  delete modelservices.platform.anselem.dev \
  -n "${NAMESPACE}"

expect_no \
  "cannot update ModelService status" \
  update modelservices.platform.anselem.dev \
  --subresource=status \
  -n "${NAMESPACE}"

expect_no \
  "cannot read Secrets" \
  get secrets \
  -n "${NAMESPACE}"

expect_no \
  "cannot create Deployments" \
  create deployments.apps \
  -n "${NAMESPACE}"

expect_no \
  "cannot create Services" \
  create services \
  -n "${NAMESPACE}"

expect_no \
  "cannot create HTTPRoutes" \
  create httproutes.gateway.networking.k8s.io \
  -n "${NAMESPACE}"

expect_no \
  "cannot create Roles" \
  create roles.rbac.authorization.k8s.io \
  -n "${NAMESPACE}"

expect_no \
  "cannot create RoleBindings" \
  create rolebindings.rbac.authorization.k8s.io \
  -n "${NAMESPACE}"

expect_no \
  "cannot request ServiceAccount tokens" \
  create serviceaccounts/token \
  -n "${NAMESPACE}"

expect_no \
  "cannot list Nodes" \
  list nodes

expect_no \
  "cannot manage ModelServices in default namespace" \
  list modelservices.platform.anselem.dev \
  -n default

echo

if (( FAILURES > 0 )); then
  echo "FAIL: AI Platform API RBAC validation completed with ${FAILURES} failure(s)."
  exit 1
fi

echo "PASS: AI Platform API RBAC validation complete."

