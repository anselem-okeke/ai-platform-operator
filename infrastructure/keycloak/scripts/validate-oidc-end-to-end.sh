#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"

KEYCLOAK_NAMESPACE="${KEYCLOAK_NAMESPACE:-keycloak}"
WORKLOAD_NAMESPACE="${WORKLOAD_NAMESPACE:-ai-platform}"

MODEL_NAME="${MODEL_NAME:-fraud-model}"
MODEL_HOSTNAME="${MODEL_HOSTNAME:-fraud-model.local}"

SECURITY_POLICY_NAME="${SECURITY_POLICY_NAME:-fraud-model-jwt-authentication}"

MODEL_CA_FILE="${MODEL_CA_FILE:-.local/keycloak/fraud-model-root-ca.crt}"
KEYCLOAK_CA_FILE="${KEYCLOAK_CA_FILE:-.local/keycloak/auth-ai-platform-root-ca.crt}"
MACHINE_TOKEN_FILE="${MACHINE_TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"

for command_name in kubectl curl jq openssl; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command is missing: ${command_name}" >&2
    exit 1
  }
done

for required_file in \
  "${MODEL_CA_FILE}" \
  "${KEYCLOAK_CA_FILE}" \
  "${MACHINE_TOKEN_FILE}"
do
  [[ -s "${required_file}" ]] || {
    echo "ERROR: Required file is missing or empty: ${required_file}" >&2
    exit 1
  }
done

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

[[ -n "${gateway_ip}" ]] || {
  echo "ERROR: Gateway address is empty." >&2
  exit 1
}

echo "Checking Keycloak workload..."

kubectl rollout status \
  deployment/keycloak \
  --namespace "${KEYCLOAK_NAMESPACE}" \
  --timeout=180s

kubectl rollout status \
  statefulset/keycloak-postgres \
  --namespace "${KEYCLOAK_NAMESPACE}" \
  --timeout=180s

echo "Checking ModelService workload..."

kubectl rollout status \
  "deployment/${MODEL_NAME}" \
  --namespace "${WORKLOAD_NAMESPACE}" \
  --timeout=180s

echo "Checking Gateway..."

kubectl wait \
  --for=condition=Programmed \
  "gateway/${GATEWAY_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout=180s

echo "Checking SecurityPolicy..."

accepted="$(
  kubectl get securitypolicy "${SECURITY_POLICY_NAME}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --output json |
  jq -r '
    [
      .status.ancestors[]?.conditions[]?
      | select(.type == "Accepted")
      | .status
    ]
    | first // ""
  '
)"

[[ "${accepted}" == "True" ]] || {
  echo "ERROR: SecurityPolicy is not Accepted." >&2
  exit 1
}

echo "Checking workload ServiceAccount hardening..."

service_account_automount="$(
  kubectl get serviceaccount "${MODEL_NAME}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --output jsonpath='{.automountServiceAccountToken}'
)"

pod_automount="$(
  kubectl get deployment "${MODEL_NAME}" \
    --namespace "${WORKLOAD_NAMESPACE}" \
    --output jsonpath='{.spec.template.spec.automountServiceAccountToken}'
)"

[[ "${service_account_automount}" == "false" ]] || {
  echo "ERROR: ServiceAccount token automount is not disabled." >&2
  exit 1
}

[[ "${pod_automount}" == "false" ]] || {
  echo "ERROR: Pod token automount is not disabled." >&2
  exit 1
}

echo "Checking HTTP redirect..."

http_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --resolve "${MODEL_HOSTNAME}:80:${gateway_ip}" \
    "http://${MODEL_HOSTNAME}/"
)"

[[ "${http_status}" == "301" ]] || {
  echo "ERROR: HTTP returned ${http_status}; expected 301." >&2
  exit 1
}

echo "Checking trusted HTTPS certificate..."

openssl s_client \
  -connect "${gateway_ip}:443" \
  -servername "${MODEL_HOSTNAME}" \
  -CAfile "${MODEL_CA_FILE}" \
  -verify_return_error \
  </dev/null \
  >/dev/null 2>&1

echo "Checking unauthenticated request..."

no_token_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${MODEL_CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    "https://${MODEL_HOSTNAME}/"
)"

[[ "${no_token_status}" == "401" ]] || {
  echo "ERROR: Missing token returned ${no_token_status}; expected 401." >&2
  exit 1
}

echo "Checking malformed token..."

invalid_token_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${MODEL_CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    --header 'Authorization: Bearer invalid.jwt.value' \
    "https://${MODEL_HOSTNAME}/"
)"

[[ "${invalid_token_status}" == "401" ]] || {
  echo "ERROR: Malformed token returned ${invalid_token_status}; expected 401." >&2
  exit 1
}

echo "Checking machine-token request..."

machine_token="$(cat "${MACHINE_TOKEN_FILE}")"

machine_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${MODEL_CA_FILE}" \
    --resolve "${MODEL_HOSTNAME}:443:${gateway_ip}" \
    --header "Authorization: Bearer ${machine_token}" \
    "https://${MODEL_HOSTNAME}/"
)"

[[ "${machine_status}" == "200" ]] || {
  echo "ERROR: Machine token returned ${machine_status}; expected 200." >&2
  exit 1
}

echo
echo "PASS: Keycloak is available."
echo "PASS: ModelService workload is available."
echo "PASS: Gateway is Programmed."
echo "PASS: SecurityPolicy is Accepted."
echo "PASS: Workload ServiceAccount token mounting is disabled."
echo "PASS: HTTP redirects to HTTPS."
echo "PASS: TLS certificate is trusted."
echo "PASS: Missing token returns 401."
echo "PASS: Malformed token returns 401."
echo "PASS: Machine identity reaches the backend."
echo
echo "PASS: End-to-end OIDC/JWT request path validated."
