#!/usr/bin/env bash
set -euo pipefail

TOKEN_FILE="${1:-.local/keycloak/tokens/service-access-token.jwt}"

EXPECTED_ISSUER="${EXPECTED_ISSUER:-https://auth.ai-platform.local/realms/ai-platform}"
EXPECTED_AUDIENCE="${EXPECTED_AUDIENCE:-ai-platform-gateway}"
EXPECTED_CLIENT="${EXPECTED_CLIENT:-ai-platform-service}"
EXPECTED_USERNAME="${EXPECTED_USERNAME:-service-account-ai-platform-service}"

if [[ ! -s "${TOKEN_FILE}" ]]; then
  echo "ERROR: Token file does not exist: ${TOKEN_FILE}" >&2
  exit 1
fi

decode_payload() {
  local token
  local payload
  local remainder

  token="$(cat "${TOKEN_FILE}")"
  payload="$(cut -d. -f2 <<<"${token}")"

  payload="${payload//-/+}"
  payload="${payload//_//}"

  remainder=$(( ${#payload} % 4 ))

  case "${remainder}" in
    0)
      ;;
    2)
      payload="${payload}=="
      ;;
    3)
      payload="${payload}="
      ;;
    *)
      echo "ERROR: Invalid JWT payload encoding." >&2
      exit 1
      ;;
  esac

  printf '%s' "${payload}" |
  base64 --decode
}

payload="$(decode_payload)"

issuer="$(jq -r '.iss' <<<"${payload}")"
authorized_party="$(jq -r '.azp' <<<"${payload}")"
username="$(jq -r '.preferred_username' <<<"${payload}")"
expiry="$(jq -r '.exp' <<<"${payload}")"
issued_at="$(jq -r '.iat' <<<"${payload}")"
now="$(date +%s)"

audience_valid="$(
  jq \
    --arg expected "${EXPECTED_AUDIENCE}" \
    '
      if (.aud | type) == "array" then
        .aud | index($expected) != null
      else
        .aud == $expected
      end
    ' <<<"${payload}"
)"

role_deployer="$(
  jq \
    '.realm_access.roles | index("model-deployer") != null' \
    <<<"${payload}"
)"

role_viewer="$(
  jq \
    '.realm_access.roles | index("model-viewer") != null' \
    <<<"${payload}"
)"

[[ "${issuer}" == "${EXPECTED_ISSUER}" ]] || {
  echo "ERROR: Unexpected issuer: ${issuer}" >&2
  exit 1
}

[[ "${audience_valid}" == "true" ]] || {
  echo "ERROR: Required audience is missing." >&2
  exit 1
}

[[ "${authorized_party}" == "${EXPECTED_CLIENT}" ]] || {
  echo "ERROR: Unexpected authorized party: ${authorized_party}" >&2
  exit 1
}

[[ "${username}" == "${EXPECTED_USERNAME}" ]] || {
  echo "ERROR: Unexpected service-account username: ${username}" >&2
  exit 1
}

[[ "${role_deployer}" == "true" ]] || {
  echo "ERROR: model-deployer role is missing." >&2
  exit 1
}

[[ "${role_viewer}" == "true" ]] || {
  echo "ERROR: inherited model-viewer role is missing." >&2
  exit 1
}

if ! [[ "${expiry}" =~ ^[0-9]+$ ]]; then
  echo "ERROR: Token does not contain a valid numeric exp claim." >&2
  exit 1
fi

if ! [[ "${issued_at}" =~ ^[0-9]+$ ]]; then
  echo "ERROR: Token does not contain a valid numeric iat claim." >&2
  exit 1
fi

if (( expiry <= now )); then
  echo "ERROR: Token has expired." >&2
  echo "       Issued at:    $(date -u -d "@${issued_at}" '+%Y-%m-%dT%H:%M:%SZ')" >&2
  echo "       Expired at:   $(date -u -d "@${expiry}" '+%Y-%m-%dT%H:%M:%SZ')" >&2
  echo "       Token lifetime: $((expiry - issued_at)) seconds" >&2
  exit 1
fi

remaining_seconds=$((expiry - now))

echo "PASS: Machine token issuer is correct."
echo "PASS: Machine token audience is correct."
echo "PASS: Authorized client is correct."
echo "PASS: Service-account identity is correct."
echo "PASS: model-deployer role is present."
echo "PASS: inherited model-viewer role is present."
echo "PASS: Machine token has not expired."
echo "INFO: Machine token expires in ${remaining_seconds} seconds."
