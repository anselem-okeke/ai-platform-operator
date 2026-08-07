#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-gateway-system}"
CERTIFICATE_NAME="${CERTIFICATE_NAME:-fraud-model-local}"
SECRET_NAME="${SECRET_NAME:-fraud-model-local-tls}"
ISSUER_NAME="${ISSUER_NAME:-vault-issuer}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
TIMEOUT="${TIMEOUT:-180s}"

command -v kubectl >/dev/null 2>&1 || {
  echo "ERROR: kubectl is required." >&2
  exit 1
}

secret_certificate() {
  kubectl get secret "${SECRET_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.data.tls\.crt}' |
    base64 --decode
}

secret_private_key() {
  kubectl get secret "${SECRET_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.data.tls\.key}' |
    base64 --decode
}

certificate_serial() {
  secret_certificate |
    openssl x509 -noout -serial |
    cut -d= -f2
}

certificate_issuer() {
  secret_certificate |
    openssl x509 -noout -issuer |
    sed 's/^issuer=//'
}

private_key_fingerprint() {
  secret_private_key |
    openssl pkey -pubout 2>/dev/null |
    openssl sha256 |
    awk '{print $2}'
}

echo "Checking Vault Issuer..."

kubectl wait \
  --for=condition=Ready \
  "issuer/${ISSUER_NAME}" \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

echo "Checking current Certificate..."

kubectl wait \
  --for=condition=Ready \
  "certificate/${CERTIFICATE_NAME}" \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

CURRENT_ISSUER="$(
  kubectl get certificate "${CERTIFICATE_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.spec.issuerRef.name}'
)"

if [[ "${CURRENT_ISSUER}" != "${ISSUER_NAME}" ]]; then
  echo "ERROR: Certificate uses '${CURRENT_ISSUER}', expected '${ISSUER_NAME}'." >&2
  exit 1
fi

OLD_SERIAL="$(certificate_serial)"
OLD_KEY_FINGERPRINT="$(private_key_fingerprint)"
OLD_REQUEST_COUNT="$(
  kubectl get certificaterequests \
    --namespace "${NAMESPACE}" \
    --no-headers 2>/dev/null |
    wc -l
)"

echo "Before renewal:"
printf '  Serial:          %s\n' "${OLD_SERIAL}"
printf '  Key fingerprint: %s\n' "${OLD_KEY_FINGERPRINT}"
printf '  Requests:        %s\n' "${OLD_REQUEST_COUNT}"

if command -v cmctl >/dev/null 2>&1; then
  echo "Requesting renewal with cmctl..."
  cmctl renew "${CERTIFICATE_NAME}" \
    --namespace "${NAMESPACE}"
else
  echo "ERROR: cmctl is required for an explicit renewal request." >&2
  echo "Install cmctl, then rerun this script." >&2
  exit 1
fi

echo "Waiting for a new certificate serial..."

DEADLINE=$((SECONDS + 180))

while true; do
  NEW_SERIAL="$(certificate_serial)"

  if [[ "${NEW_SERIAL}" != "${OLD_SERIAL}" ]]; then
    break
  fi

  if (( SECONDS >= DEADLINE )); then
    echo "ERROR: Certificate serial did not change within 180 seconds." >&2
    exit 1
  fi

  sleep 5
done

kubectl wait \
  --for=condition=Ready \
  "certificate/${CERTIFICATE_NAME}" \
  --namespace "${NAMESPACE}" \
  --timeout "${TIMEOUT}"

NEW_SERIAL="$(certificate_serial)"
NEW_KEY_FINGERPRINT="$(private_key_fingerprint)"
NEW_ISSUER="$(certificate_issuer)"
NEW_REQUEST_COUNT="$(
  kubectl get certificaterequests \
    --namespace "${NAMESPACE}" \
    --no-headers 2>/dev/null |
    wc -l
)"

echo "After renewal:"
printf '  Serial:          %s\n' "${NEW_SERIAL}"
printf '  Key fingerprint: %s\n' "${NEW_KEY_FINGERPRINT}"
printf '  Certificate CA:  %s\n' "${NEW_ISSUER}"
printf '  Requests:        %s\n' "${NEW_REQUEST_COUNT}"

if [[ "${OLD_SERIAL}" == "${NEW_SERIAL}" ]]; then
  echo "ERROR: Certificate serial did not rotate." >&2
  exit 1
fi

if [[ "${OLD_KEY_FINGERPRINT}" == "${NEW_KEY_FINGERPRINT}" ]]; then
  echo "ERROR: Private key did not rotate." >&2
  exit 1
fi

if [[ "${NEW_ISSUER}" != *"AI Platform ModelService Root CA"* ]]; then
  echo "ERROR: Renewed certificate was not signed by the expected Vault CA." >&2
  exit 1
fi

PROGRAMMED="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${NAMESPACE}" \
    --output jsonpath='{.status.conditions[?(@.type=="Programmed")].status}'
)"

if [[ "${PROGRAMMED}" != "True" ]]; then
  echo "ERROR: Gateway is not Programmed after certificate rotation." >&2
  exit 1
fi

echo
echo "PASS: Vault certificate renewal succeeded."
echo "PASS: Certificate serial changed."
echo "PASS: Private key rotated."
echo "PASS: Renewed certificate was signed by Vault."
echo "PASS: Gateway remained Programmed."
