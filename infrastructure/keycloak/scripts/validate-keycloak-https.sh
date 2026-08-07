#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
KEYCLOAK_NAMESPACE="${KEYCLOAK_NAMESPACE:-keycloak}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-auth.ai-platform.local}"
CERTIFICATE_NAME="${CERTIFICATE_NAME:-auth-ai-platform-local}"
TLS_SECRET_NAME="${TLS_SECRET_NAME:-auth-ai-platform-local-tls}"
ISSUER_NAME="${ISSUER_NAME:-vault-keycloak-issuer}"
CA_FILE="${CA_FILE:-.local/keycloak/auth-ai-platform-root-ca.crt}"
TIMEOUT="${TIMEOUT:-180s}"

for command_name in kubectl curl openssl jq base64; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "ERROR: Required command not found: ${command_name}" >&2
    exit 1
  fi
done

echo "Checking Vault Keycloak Issuer..."

kubectl wait \
  --for=condition=Ready \
  "issuer/${ISSUER_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking Keycloak Certificate..."

kubectl wait \
  --for=condition=Ready \
  "certificate/${CERTIFICATE_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking shared Gateway..."

kubectl wait \
  --for=condition=Programmed \
  "gateway/${GATEWAY_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking Keycloak routes..."

for route_name in keycloak keycloak-http-redirect; do
  accepted="$(
    kubectl get httproute "${route_name}" \
      --namespace "${KEYCLOAK_NAMESPACE}" \
      --output jsonpath='{.status.parents[0].conditions[?(@.type=="Accepted")].status}'
  )"

  resolved_refs="$(
    kubectl get httproute "${route_name}" \
      --namespace "${KEYCLOAK_NAMESPACE}" \
      --output jsonpath='{.status.parents[0].conditions[?(@.type=="ResolvedRefs")].status}'
  )"

  if [[ "${accepted}" != "True" ]]; then
    echo "ERROR: HTTPRoute/${route_name} is not Accepted." >&2
    exit 1
  fi

  if [[ "${resolved_refs}" != "True" ]]; then
    echo "ERROR: HTTPRoute/${route_name} has unresolved references." >&2
    exit 1
  fi
done

echo "Checking Keycloak workload..."

kubectl rollout status \
  deployment/keycloak \
  --namespace "${KEYCLOAK_NAMESPACE}" \
  --timeout="${TIMEOUT}"

echo "Checking PostgreSQL workload..."

kubectl rollout status \
  statefulset/keycloak-postgres \
  --namespace "${KEYCLOAK_NAMESPACE}" \
  --timeout="${TIMEOUT}"

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

if [[ -z "${gateway_ip}" ]]; then
  echo "ERROR: Gateway has no assigned address." >&2
  exit 1
fi

echo "Gateway address: ${gateway_ip}"
echo "Keycloak hostname: ${KEYCLOAK_HOSTNAME}"

mkdir -p "$(dirname "${CA_FILE}")"
chmod 700 "$(dirname "${CA_FILE}")"

kubectl get secret "${TLS_SECRET_NAME}" \
  --namespace "${GATEWAY_NAMESPACE}" \
  --output jsonpath='{.data.ca\.crt}' |
base64 --decode \
  > "${CA_FILE}"

chmod 644 "${CA_FILE}"

if ! openssl x509 \
  -in "${CA_FILE}" \
  -noout \
  -subject \
  -issuer \
  >/dev/null
then
  echo "ERROR: Exported CA file is not a valid certificate." >&2
  exit 1
fi

echo "Checking HTTP-to-HTTPS redirect..."

redirect_headers="$(
  curl \
    --silent \
    --show-error \
    --head \
    --resolve "${KEYCLOAK_HOSTNAME}:80:${gateway_ip}" \
    "http://${KEYCLOAK_HOSTNAME}/"
)"

http_status="$(
  printf '%s\n' "${redirect_headers}" |
  awk 'NR == 1 {print $2}'
)"

redirect_location="$(
  printf '%s\n' "${redirect_headers}" |
  awk '
    BEGIN {IGNORECASE=1}
    /^location:/ {
      sub(/\r$/, "", $2)
      print $2
      exit
    }
  '
)"

expected_location="https://${KEYCLOAK_HOSTNAME}/"

if [[ "${http_status}" != "301" ]]; then
  echo "ERROR: HTTP returned ${http_status}, expected 301." >&2
  printf '%s\n' "${redirect_headers}" >&2
  exit 1
fi

if [[ "${redirect_location}" != "${expected_location}" ]]; then
  echo "ERROR: Redirect location is '${redirect_location}'." >&2
  echo "Expected: ${expected_location}" >&2
  exit 1
fi

echo "Checking trusted HTTPS..."

https_status="$(
  curl \
    --silent \
    --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --cacert "${CA_FILE}" \
    --resolve "${KEYCLOAK_HOSTNAME}:443:${gateway_ip}" \
    "https://${KEYCLOAK_HOSTNAME}/"
)"

case "${https_status}" in
  200|302|303)
    ;;
  *)
    echo "ERROR: HTTPS returned ${https_status}." >&2
    exit 1
    ;;
esac

echo "Checking TLS certificate hostname..."

served_certificate="$(
  mktemp
)"

trap 'rm -f "${served_certificate}"' EXIT

openssl s_client \
  -connect "${gateway_ip}:443" \
  -servername "${KEYCLOAK_HOSTNAME}" \
  -CAfile "${CA_FILE}" \
  -verify_return_error \
  </dev/null \
  2>/dev/null |
openssl x509 \
  > "${served_certificate}"

if ! openssl x509 \
  -in "${served_certificate}" \
  -noout \
  -checkhost "${KEYCLOAK_HOSTNAME}" \
  >/dev/null
then
  echo "ERROR: Served certificate does not match ${KEYCLOAK_HOSTNAME}." >&2
  exit 1
fi

echo "Checking OIDC discovery..."

discovery_document="$(
  curl \
    --silent \
    --show-error \
    --cacert "${CA_FILE}" \
    --resolve "${KEYCLOAK_HOSTNAME}:443:${gateway_ip}" \
    "https://${KEYCLOAK_HOSTNAME}/realms/master/.well-known/openid-configuration"
)"

issuer="$(
  printf '%s' "${discovery_document}" |
  jq -r '.issuer'
)"

authorization_endpoint="$(
  printf '%s' "${discovery_document}" |
  jq -r '.authorization_endpoint'
)"

token_endpoint="$(
  printf '%s' "${discovery_document}" |
  jq -r '.token_endpoint'
)"

jwks_uri="$(
  printf '%s' "${discovery_document}" |
  jq -r '.jwks_uri'
)"

expected_issuer="https://${KEYCLOAK_HOSTNAME}/realms/master"

if [[ "${issuer}" != "${expected_issuer}" ]]; then
  echo "ERROR: OIDC issuer is '${issuer}'." >&2
  echo "Expected: ${expected_issuer}" >&2
  exit 1
fi

for endpoint in \
  "${authorization_endpoint}" \
  "${token_endpoint}" \
  "${jwks_uri}"
do
  if [[ "${endpoint}" != "https://${KEYCLOAK_HOSTNAME}/"* ]]; then
    echo "ERROR: OIDC endpoint uses an unexpected external URL: ${endpoint}" >&2
    exit 1
  fi
done

echo
echo "PASS: Vault Keycloak Issuer is ready."
echo "PASS: Keycloak certificate is ready."
echo "PASS: Gateway is Programmed."
echo "PASS: Keycloak routes are Accepted and Resolved."
echo "PASS: HTTP redirects to HTTPS."
echo "PASS: Trusted HTTPS reaches Keycloak."
echo "PASS: Served certificate matches ${KEYCLOAK_HOSTNAME}."
echo "PASS: OIDC discovery publishes the correct external issuer."
