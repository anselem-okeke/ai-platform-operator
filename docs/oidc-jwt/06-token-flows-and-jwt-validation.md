# Token Flows and JWT Validation

## Purpose

This document describes how human and machine identities obtain JSON Web Tokens from Keycloak and how those tokens are decoded, inspected, and cryptographically validated.

It covers:

- machine-to-machine authentication with the client-credentials grant;
- human authentication with Authorization Code and PKCE;
- JWT structure and claims;
- audience, issuer, role, and expiry validation;
- JWKS retrieval and signature validation;
- safe local token handling;
- reusable validation scripts;
- troubleshooting common token failures.

The implementation uses:

```text
Realm: ai-platform
Issuer: https://auth.ai-platform.local/realms/ai-platform
Audience: ai-platform-gateway
Public client: ai-platform-cli
Confidential client: ai-platform-service
```

---

## Token Architecture

### Machine identity flow

```text
ai-platform-service
  ↓ client_id + client_secret
Keycloak token endpoint
  ↓
Client-credentials access token
  ↓
service-account-ai-platform-service
```

### Human identity flow

```text
Browser
  ↓ Authorization Code + PKCE
ai-platform-cli
  ↓
Keycloak login
  ↓
Loopback callback
  ↓
Authorization code
  ↓ code_verifier
Keycloak token endpoint
  ↓
User access token
```

---

## Why Two Token Flows Are Used

### Machine access

The confidential client:

```text
ai-platform-service
```

uses the OAuth 2.0 client-credentials grant.

It is appropriate for:

```text
automation
CI/CD
platform services
non-interactive workloads
```

The resulting token represents:

```text
service-account-ai-platform-service
```

### Human access

The public client:

```text
ai-platform-cli
```

uses Authorization Code with PKCE.

It is appropriate for:

```text
interactive user login
browser-based authentication
CLI tools that cannot safely store a client secret
```

The direct password grant remains disabled.

---

## Local Token Directory

Create a protected local directory:

```bash
cd /mnt/data/ai-platform-operator

mkdir -p .local/keycloak/tokens
chmod 700 .local/keycloak/tokens
```

All token files are stored under:

```text
.local/keycloak/
```

This directory must remain excluded from Git.

Verify:

```bash
git check-ignore -v .local/keycloak/
```

---

## Resolve the Gateway Address

```bash
GATEWAY_IP="$(
  kubectl get gateway shared-gateway \
    -n gateway-system \
    -o jsonpath='{.status.addresses[0].value}'
)"
```

Verify:

```bash
echo "${GATEWAY_IP}"
```

Expected in the documented environment:

```text
172.19.255.200
```

---

# Machine Token Flow

## Load the Service Client Credentials

The client credentials are stored in the Kubernetes Secret:

```text
keycloak/ai-platform-service-client-credentials
```

Load them without printing the secret:

```bash
SERVICE_CLIENT_ID="$(
  kubectl get secret ai-platform-service-client-credentials \
    -n keycloak \
    -o jsonpath='{.data.CLIENT_ID}' |
  base64 --decode
)"

SERVICE_CLIENT_SECRET="$(
  kubectl get secret ai-platform-service-client-credentials \
    -n keycloak \
    -o jsonpath='{.data.CLIENT_SECRET}' |
  base64 --decode
)"
```

Confirm only that the values exist:

```bash
test -n "${SERVICE_CLIENT_ID}" &&
echo "PASS: service client ID loaded"

test -n "${SERVICE_CLIENT_SECRET}" &&
echo "PASS: service client secret loaded"
```

Do not print the secret.

---

## Request a Machine Access Token

```bash
curl \
  --silent \
  --show-error \
  --fail-with-body \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  --request POST \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode "client_id=${SERVICE_CLIENT_ID}" \
  --data-urlencode "client_secret=${SERVICE_CLIENT_SECRET}" \
  https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/token \
  > .local/keycloak/tokens/service-token-response.json
```

Restrict access:

```bash
chmod 600 \
  .local/keycloak/tokens/service-token-response.json
```

---

## Check the Token Response

Inspect only non-sensitive metadata:

```bash
jq '{
  token_type,
  expires_in,
  scope,
  access_token_present: (.access_token | type == "string"),
  refresh_token_present: (.refresh_token | type == "string")
}' .local/keycloak/tokens/service-token-response.json
```

Expected structure:

```json
{
  "token_type": "Bearer",
  "expires_in": 300,
  "access_token_present": true,
  "refresh_token_present": false
}
```

The observed machine token lifetime was approximately:

```text
300 seconds
```

---

## Extract the Access Token

```bash
jq -r '.access_token' \
  .local/keycloak/tokens/service-token-response.json \
  > .local/keycloak/tokens/service-access-token.jwt
```

```bash
chmod 600 \
  .local/keycloak/tokens/service-access-token.jwt
```

Verify that the token has three JWT segments:

```bash
awk -F. '
  NF == 3 {
    print "PASS: machine access token has JWT structure"
    exit 0
  }

  {
    print "ERROR: token is not a three-part JWT" > "/dev/stderr"
    exit 1
  }
' .local/keycloak/tokens/service-access-token.jwt
```

---

## Reusable Machine Token Script

Create:

```text
infrastructure/keycloak/scripts/get-machine-token.sh
```

```bash
cat > infrastructure/keycloak/scripts/get-machine-token.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-gateway-system}"
GATEWAY_NAME="${GATEWAY_NAME:-shared-gateway}"
KEYCLOAK_NAMESPACE="${KEYCLOAK_NAMESPACE:-keycloak}"
KEYCLOAK_HOSTNAME="${KEYCLOAK_HOSTNAME:-auth.ai-platform.local}"
REALM="${REALM:-ai-platform}"

CA_FILE="${CA_FILE:-.local/keycloak/auth-ai-platform-root-ca.crt}"
TOKEN_DIRECTORY="${TOKEN_DIRECTORY:-.local/keycloak/tokens}"
TOKEN_RESPONSE_FILE="${TOKEN_RESPONSE_FILE:-${TOKEN_DIRECTORY}/service-token-response.json}"
TOKEN_FILE="${TOKEN_FILE:-${TOKEN_DIRECTORY}/service-access-token.jwt}"

for command_name in kubectl curl jq base64; do
  command -v "${command_name}" >/dev/null 2>&1 || {
    echo "ERROR: Required command missing: ${command_name}" >&2
    exit 1
  }
done

[[ -s "${CA_FILE}" ]] || {
  echo "ERROR: CA file is missing: ${CA_FILE}" >&2
  exit 1
}

gateway_ip="$(
  kubectl get gateway "${GATEWAY_NAME}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --output jsonpath='{.status.addresses[0].value}'
)"

[[ -n "${gateway_ip}" ]] || {
  echo "ERROR: Gateway address is empty." >&2
  exit 1
}

client_id="$(
  kubectl get secret ai-platform-service-client-credentials \
    --namespace "${KEYCLOAK_NAMESPACE}" \
    --output jsonpath='{.data.CLIENT_ID}' |
  base64 --decode
)"

client_secret="$(
  kubectl get secret ai-platform-service-client-credentials \
    --namespace "${KEYCLOAK_NAMESPACE}" \
    --output jsonpath='{.data.CLIENT_SECRET}' |
  base64 --decode
)"

[[ -n "${client_id}" ]] || {
  echo "ERROR: Client ID is empty." >&2
  exit 1
}

[[ -n "${client_secret}" ]] || {
  echo "ERROR: Client secret is empty." >&2
  exit 1
}

mkdir -p "${TOKEN_DIRECTORY}"
chmod 700 "${TOKEN_DIRECTORY}"

curl \
  --silent \
  --show-error \
  --fail-with-body \
  --cacert "${CA_FILE}" \
  --resolve "${KEYCLOAK_HOSTNAME}:443:${gateway_ip}" \
  --request POST \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode "client_id=${client_id}" \
  --data-urlencode "client_secret=${client_secret}" \
  "https://${KEYCLOAK_HOSTNAME}/realms/${REALM}/protocol/openid-connect/token" \
  > "${TOKEN_RESPONSE_FILE}"

chmod 600 "${TOKEN_RESPONSE_FILE}"

if jq -e '.error' "${TOKEN_RESPONSE_FILE}" >/dev/null 2>&1; then
  jq '{
    error,
    error_description
  }' "${TOKEN_RESPONSE_FILE}" >&2
  exit 1
fi

jq -r '.access_token' \
  "${TOKEN_RESPONSE_FILE}" \
  > "${TOKEN_FILE}"

chmod 600 "${TOKEN_FILE}"

token_type="$(jq -r '.token_type' "${TOKEN_RESPONSE_FILE}")"
expires_in="$(jq -r '.expires_in' "${TOKEN_RESPONSE_FILE}")"

unset client_secret

echo "PASS: Machine access token obtained."
echo "INFO: Client: ${client_id}"
echo "INFO: Token type: ${token_type}"
echo "INFO: Expires in: ${expires_in} seconds"
echo "INFO: Token written to: ${TOKEN_FILE}"
EOF
```

Make it executable:

```bash
chmod +x \
  infrastructure/keycloak/scripts/get-machine-token.sh

bash -n \
  infrastructure/keycloak/scripts/get-machine-token.sh
```

Run it:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

---

# JWT Structure

A JWT contains three dot-separated segments:

```text
header.payload.signature
```

### Header

Usually contains:

```json
{
  "alg": "RS256",
  "kid": "signing-key-id",
  "typ": "JWT"
}
```

### Payload

Contains claims such as:

```text
iss
sub
aud
azp
preferred_username
realm_access.roles
iat
exp
```

### Signature

Proves that the token was signed by the expected Keycloak signing key.

---

## JWT Decoder

Create:

```text
infrastructure/keycloak/scripts/decode-jwt.sh
```

```bash
cat > infrastructure/keycloak/scripts/decode-jwt.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

TOKEN="${1:-}"

if [[ -z "${TOKEN}" ]]; then
  echo "Usage: $0 <jwt-or-token-file>" >&2
  exit 1
fi

if [[ -f "${TOKEN}" ]]; then
  TOKEN="$(cat "${TOKEN}")"
fi

decode_segment() {
  local segment="$1"
  local remainder

  segment="${segment//-/+}"
  segment="${segment//_//}"

  remainder=$(( ${#segment} % 4 ))

  case "${remainder}" in
    0)
      ;;
    2)
      segment="${segment}=="
      ;;
    3)
      segment="${segment}="
      ;;
    *)
      echo "ERROR: Invalid base64url segment." >&2
      return 1
      ;;
  esac

  printf '%s' "${segment}" |
  base64 --decode
}

IFS='.' read -r header payload signature <<<"${TOKEN}"

if [[ -z "${header}" || -z "${payload}" || -z "${signature}" ]]; then
  echo "ERROR: Input is not a three-part JWT." >&2
  exit 1
fi

echo "Header:"
decode_segment "${header}" |
jq .

echo
echo "Payload:"
decode_segment "${payload}" |
jq .
EOF
```

Make it executable:

```bash
chmod +x \
  infrastructure/keycloak/scripts/decode-jwt.sh

bash -n \
  infrastructure/keycloak/scripts/decode-jwt.sh
```

Run:

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/service-access-token.jwt
```

---

## Important Distinction: Decoding Is Not Validation

Decoding only reveals the JSON content.

It does not prove:

```text
who signed the token
whether the token was modified
whether the token is expired
whether the issuer is trusted
whether the audience is correct
```

Cryptographic validation must be performed separately.

---

## Expected Machine Token Claims

Inspect selected claims:

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/service-access-token.jwt |
sed -n '/^Payload:/,$p' |
tail -n +2 |
jq '{
  iss,
  sub,
  aud,
  azp,
  typ,
  preferred_username,
  realm_roles: .realm_access.roles,
  scope,
  iat,
  exp
}'
```

Expected important values:

```text
iss:
  https://auth.ai-platform.local/realms/ai-platform

aud:
  contains ai-platform-gateway

azp:
  ai-platform-service

preferred_username:
  service-account-ai-platform-service

realm_access.roles:
  contains model-deployer
  contains model-viewer
```

The role:

```text
model-viewer
```

appears because `model-deployer` is composite.

---

## Machine Claim Validation Script

Create:

```text
infrastructure/keycloak/scripts/validate-machine-token.sh
```

```bash
cat > infrastructure/keycloak/scripts/validate-machine-token.sh <<'EOF'
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

[[ "${expiry}" -gt "${now}" ]] || {
  echo "ERROR: Token has expired." >&2
  exit 1
}

echo "PASS: Machine token issuer is correct."
echo "PASS: Machine token audience is correct."
echo "PASS: Authorized client is correct."
echo "PASS: Service-account identity is correct."
echo "PASS: model-deployer role is present."
echo "PASS: inherited model-viewer role is present."
echo "PASS: Machine token has not expired."
EOF
```

Run:

```bash
chmod +x \
  infrastructure/keycloak/scripts/validate-machine-token.sh

bash -n \
  infrastructure/keycloak/scripts/validate-machine-token.sh

infrastructure/keycloak/scripts/validate-machine-token.sh
```

---

# JWKS and Signature Validation

## Download the Realm JWKS

```bash
curl \
  --silent \
  --show-error \
  --fail \
  --cacert .local/keycloak/auth-ai-platform-root-ca.crt \
  --resolve "auth.ai-platform.local:443:${GATEWAY_IP}" \
  https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  > .local/keycloak/ai-platform-jwks.json
```

Restrict access:

```bash
chmod 600 \
  .local/keycloak/ai-platform-jwks.json
```

Inspect public-key metadata:

```bash
jq '{
  key_count: (.keys | length),
  keys: [
    .keys[] |
    {
      kid,
      kty,
      alg,
      use
    }
  ]
}' .local/keycloak/ai-platform-jwks.json
```

---

## Compare the JWT Key ID with JWKS

Extract the token key ID:

```bash
TOKEN_KID="$(
  infrastructure/keycloak/scripts/decode-jwt.sh \
    .local/keycloak/tokens/service-access-token.jwt |
  sed -n '/^Header:/,/^Payload:/p' |
  sed '1d;$d' |
  jq -r '.kid'
)"
```

```bash
echo "Token kid: ${TOKEN_KID}"
```

Find the matching public key:

```bash
jq \
  --arg kid "${TOKEN_KID}" \
  '.keys[] | select(.kid == $kid) | {kid, kty, alg, use}' \
  .local/keycloak/ai-platform-jwks.json
```

Expected: one matching signing key.

---

## Python Validation Environment

Create a local virtual environment:

```bash
python3 -m venv .local/keycloak/venv
```

Install PyJWT with cryptographic support:

```bash
.local/keycloak/venv/bin/pip install \
  --disable-pip-version-check \
  'PyJWT[crypto]>=2.10,<3'
```

---

## JWT Signature Validation Script

Create:

```text
infrastructure/keycloak/scripts/validate-jwt-signature.py
```

```python
#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import ssl
import sys
from pathlib import Path

import jwt
from jwt import PyJWKClient
from jwt.exceptions import PyJWTError


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate a JWT against an OIDC JWKS endpoint."
    )
    parser.add_argument("--token-file", required=True)
    parser.add_argument("--jwks-url", required=True)
    parser.add_argument("--issuer", required=True)
    parser.add_argument("--audience", required=True)
    parser.add_argument("--ca-file", required=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()

    token = Path(args.token_file).read_text(encoding="utf-8").strip()

    if not token:
        print("ERROR: Token file is empty.", file=sys.stderr)
        return 1

    try:
        ssl_context = ssl.create_default_context(cafile=args.ca_file)

        jwks_client = PyJWKClient(
            args.jwks_url,
            ssl_context=ssl_context,
        )

        signing_key = jwks_client.get_signing_key_from_jwt(token)

        claims = jwt.decode(
            token,
            signing_key.key,
            algorithms=["RS256", "ES256"],
            audience=args.audience,
            issuer=args.issuer,
            options={
                "require": ["exp", "iat", "iss", "aud"],
            },
        )
    except PyJWTError as exc:
        print(f"ERROR: JWT validation failed: {exc}", file=sys.stderr)
        return 1
    except Exception as exc:
        print(
            f"ERROR: Unable to retrieve or process JWKS: {exc}",
            file=sys.stderr,
        )
        return 1

    safe_claims = {
        "iss": claims.get("iss"),
        "sub": claims.get("sub"),
        "aud": claims.get("aud"),
        "azp": claims.get("azp"),
        "preferred_username": claims.get("preferred_username"),
        "realm_roles": claims.get("realm_access", {}).get("roles", []),
        "iat": claims.get("iat"),
        "exp": claims.get("exp"),
    }

    print(json.dumps(safe_claims, indent=2, sort_keys=True))
    print("PASS: JWT signature, issuer, audience and expiry are valid.")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

Save it as:

```text
infrastructure/keycloak/scripts/validate-jwt-signature.py
```

Make executable:

```bash
chmod +x \
  infrastructure/keycloak/scripts/validate-jwt-signature.py
```

---

## Local Hostname Resolution for Python

The Python JWKS client must resolve:

```text
auth.ai-platform.local
```

Add the Gateway mapping to `/etc/hosts` when lab DNS is unavailable:

```bash
grep -qE \
  '^[[:space:]]*172\.19\.255\.200[[:space:]]+auth\.ai-platform\.local([[:space:]]|$)' \
  /etc/hosts ||
echo "${GATEWAY_IP} auth.ai-platform.local" |
sudo tee -a /etc/hosts
```

Use the current Gateway IP rather than hardcoding when it differs.

---

## Validate the Machine Token Signature

```bash
.local/keycloak/venv/bin/python \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  --token-file .local/keycloak/tokens/service-access-token.jwt \
  --jwks-url https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  --issuer https://auth.ai-platform.local/realms/ai-platform \
  --audience ai-platform-gateway \
  --ca-file .local/keycloak/auth-ai-platform-root-ca.crt
```

Expected ending:

```text
PASS: JWT signature, issuer, audience and expiry are valid.
```

---

# Human Authorization Code and PKCE Flow

## Why PKCE Is Required

The `ai-platform-cli` client is public.

It cannot safely store a client secret.

PKCE protects the authorization code by requiring the token exchange to include the original:

```text
code_verifier
```

Keycloak validates that it matches the earlier:

```text
code_challenge
```

The client configuration uses:

```text
publicClient=true
standardFlowEnabled=true
directAccessGrantsEnabled=false
PKCE=S256
```

---

## PKCE Login Helper

Create:

```text
infrastructure/keycloak/scripts/pkce-login.py
```

```python
#!/usr/bin/env python3

from __future__ import annotations

import argparse
import base64
import hashlib
import http.server
import json
import secrets
import ssl
import sys
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


def base64url(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Perform an OIDC Authorization Code + PKCE login."
    )
    parser.add_argument(
        "--issuer",
        default="https://auth.ai-platform.local/realms/ai-platform",
    )
    parser.add_argument("--client-id", default="ai-platform-cli")
    parser.add_argument(
        "--redirect-uri",
        default="http://127.0.0.1:18080/callback",
    )
    parser.add_argument(
        "--ca-file",
        default=".local/keycloak/auth-ai-platform-root-ca.crt",
    )
    parser.add_argument(
        "--output",
        default=".local/keycloak/tokens/user-token-response.json",
    )
    return parser.parse_args()


class CallbackHandler(http.server.BaseHTTPRequestHandler):
    authorization_code: str | None = None
    returned_state: str | None = None
    oauth_error: str | None = None

    def do_GET(self) -> None:
        query = urllib.parse.parse_qs(
            urllib.parse.urlparse(self.path).query
        )

        CallbackHandler.authorization_code = query.get("code", [None])[0]
        CallbackHandler.returned_state = query.get("state", [None])[0]
        CallbackHandler.oauth_error = query.get("error", [None])[0]

        body = (
            b"Authorization response received. "
            b"You may close this browser window."
        )

        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format: str, *args: Any) -> None:
        return


def main() -> int:
    args = parse_args()

    parsed_redirect = urllib.parse.urlparse(args.redirect_uri)

    if parsed_redirect.hostname not in {"127.0.0.1", "localhost"}:
        print(
            "ERROR: This helper accepts only loopback redirect URIs.",
            file=sys.stderr,
        )
        return 1

    if parsed_redirect.port is None:
        print("ERROR: Redirect URI requires an explicit port.", file=sys.stderr)
        return 1

    verifier = base64url(secrets.token_bytes(64))
    challenge = base64url(
        hashlib.sha256(verifier.encode("ascii")).digest()
    )
    state = base64url(secrets.token_bytes(32))

    authorization_endpoint = (
        f"{args.issuer}/protocol/openid-connect/auth"
    )
    token_endpoint = (
        f"{args.issuer}/protocol/openid-connect/token"
    )

    authorization_query = urllib.parse.urlencode(
        {
            "response_type": "code",
            "client_id": args.client_id,
            "redirect_uri": args.redirect_uri,
            "scope": "openid profile email",
            "state": state,
            "code_challenge": challenge,
            "code_challenge_method": "S256",
        }
    )

    authorization_url = (
        f"{authorization_endpoint}?{authorization_query}"
    )

    print()
    print("Open this URL in a browser:")
    print()
    print(authorization_url)
    print()
    print(
        "Waiting for the callback on "
        f"{parsed_redirect.hostname}:{parsed_redirect.port}..."
    )

    server = http.server.HTTPServer(
        (parsed_redirect.hostname, parsed_redirect.port),
        CallbackHandler,
    )

    server.handle_request()
    server.server_close()

    if CallbackHandler.oauth_error:
        print(
            f"ERROR: Authorization failed: "
            f"{CallbackHandler.oauth_error}",
            file=sys.stderr,
        )
        return 1

    if not CallbackHandler.authorization_code:
        print("ERROR: Authorization code was not returned.", file=sys.stderr)
        return 1

    if CallbackHandler.returned_state != state:
        print("ERROR: OAuth state validation failed.", file=sys.stderr)
        return 1

    token_request_body = urllib.parse.urlencode(
        {
            "grant_type": "authorization_code",
            "client_id": args.client_id,
            "code": CallbackHandler.authorization_code,
            "redirect_uri": args.redirect_uri,
            "code_verifier": verifier,
        }
    ).encode("utf-8")

    request = urllib.request.Request(
        token_endpoint,
        data=token_request_body,
        method="POST",
        headers={
            "Content-Type": "application/x-www-form-urlencoded",
        },
    )

    ssl_context = ssl.create_default_context(cafile=args.ca_file)

    try:
        with urllib.request.urlopen(
            request,
            context=ssl_context,
            timeout=30,
        ) as response:
            token_response = json.loads(
                response.read().decode("utf-8")
            )
    except Exception as exc:
        print(
            f"ERROR: Token exchange failed: {exc}",
            file=sys.stderr,
        )
        return 1

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(
        json.dumps(token_response, indent=2),
        encoding="utf-8",
    )
    output_path.chmod(0o600)

    access_token = token_response.get("access_token")

    if not access_token:
        print("ERROR: No access token was returned.", file=sys.stderr)
        return 1

    access_token_path = output_path.with_name(
        "user-access-token.jwt"
    )
    access_token_path.write_text(
        access_token,
        encoding="utf-8",
    )
    access_token_path.chmod(0o600)

    print()
    print("PASS: Authorization Code + PKCE exchange completed.")
    print(f"Token response: {output_path}")
    print(f"Access token:   {access_token_path}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

Make executable and validate:

```bash
chmod +x \
  infrastructure/keycloak/scripts/pkce-login.py

python3 -m py_compile \
  infrastructure/keycloak/scripts/pkce-login.py
```

Remove generated cache directories afterward:

```bash
find infrastructure/keycloak \
  -type d \
  -name '__pycache__' \
  -prune \
  -exec rm -rf {} +
```

---

## Create the SSH Callback Tunnel

The helper runs on the Ansible VM, while the browser normally runs on a desktop.

From the desktop:

```bash
ssh \
  -L 18080:127.0.0.1:18080 \
  ansible@192.168.0.58
```

Keep the SSH session open.

The browser must trust the AI Platform root CA and resolve:

```text
auth.ai-platform.local
```

---

## Start the PKCE Login

On the Ansible VM:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

The helper prints an authorization URL.

Open it in the browser and log in as one of:

```text
viewer-user
deployer-user
admin-user
```

Use the corresponding local password file:

```text
.local/keycloak/test-users/viewer-user-password
.local/keycloak/test-users/deployer-user-password
.local/keycloak/test-users/admin-user-password
```

Do not paste these passwords into logs, commits, or documentation output.

After authentication, Keycloak redirects to:

```text
http://127.0.0.1:18080/callback
```

The SSH tunnel forwards the callback to the helper.

---

## Inspect the User Token

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/user-access-token.jwt
```

Inspect selected claims:

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/user-access-token.jwt |
sed -n '/^Payload:/,$p' |
tail -n +2 |
jq '{
  iss,
  sub,
  aud,
  azp,
  preferred_username,
  email,
  realm_roles: .realm_access.roles,
  scope,
  iat,
  exp
}'
```

---

## Expected Human Token Claims

### `viewer-user`

```text
preferred_username:
  viewer-user

roles:
  model-viewer
```

### `deployer-user`

```text
preferred_username:
  deployer-user

roles:
  model-deployer
  model-viewer
```

### `admin-user`

```text
preferred_username:
  admin-user

roles:
  platform-admin
  model-deployer
  model-viewer
```

All user tokens should also contain:

```text
iss:
  https://auth.ai-platform.local/realms/ai-platform

aud:
  ai-platform-gateway

azp:
  ai-platform-cli
```

---

## Validate the User Token Signature

```bash
.local/keycloak/venv/bin/python \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  --token-file .local/keycloak/tokens/user-access-token.jwt \
  --jwks-url https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  --issuer https://auth.ai-platform.local/realms/ai-platform \
  --audience ai-platform-gateway \
  --ca-file .local/keycloak/auth-ai-platform-root-ca.crt
```

Expected:

```text
PASS: JWT signature, issuer, audience and expiry are valid.
```

---

## Save Tokens by Identity

The PKCE helper writes the latest user token to:

```text
.local/keycloak/tokens/user-access-token.jwt
```

After logging in as `viewer-user`:

```bash
cp \
  .local/keycloak/tokens/user-access-token.jwt \
  .local/keycloak/tokens/viewer-access-token.jwt

chmod 600 \
  .local/keycloak/tokens/viewer-access-token.jwt
```

After logging in as `deployer-user`:

```bash
cp \
  .local/keycloak/tokens/user-access-token.jwt \
  .local/keycloak/tokens/deployer-access-token.jwt

chmod 600 \
  .local/keycloak/tokens/deployer-access-token.jwt
```

After logging in as `admin-user`:

```bash
cp \
  .local/keycloak/tokens/user-access-token.jwt \
  .local/keycloak/tokens/admin-access-token.jwt

chmod 600 \
  .local/keycloak/tokens/admin-access-token.jwt
```

These files remain local and must never be committed.

---

## Verify Stored Token Identities

```bash
for token_file in \
  .local/keycloak/tokens/viewer-access-token.jwt \
  .local/keycloak/tokens/deployer-access-token.jwt \
  .local/keycloak/tokens/admin-access-token.jwt
do
  echo "Token: ${token_file}"

  infrastructure/keycloak/scripts/decode-jwt.sh \
    "${token_file}" |
  sed -n '/^Payload:/,$p' |
  tail -n +2 |
  jq '{
    preferred_username,
    roles: .realm_access.roles,
    exp
  }'
done
```

---

# Token Validation Layers

The implementation validates tokens at three levels.

## Layer 1 — Structural validation

Checks:

```text
three JWT segments
valid base64url encoding
valid JSON
```

Tools:

```text
decode-jwt.sh
```

## Layer 2 — Claim validation

Checks:

```text
issuer
audience
authorized party
username
roles
expiry
```

Tools:

```text
validate-machine-token.sh
```

## Layer 3 — Cryptographic validation

Checks:

```text
signing key selected by kid
signature validity
issuer
audience
expiry
```

Tools:

```text
validate-jwt-signature.py
```

The Gateway later performs comparable runtime validation before forwarding requests.

---

# Token Expiry

The observed access-token lifetime is:

```text
300 seconds
```

A token that worked earlier may later return:

```text
401 Unauthorized
```

This can happen even when:

```text
the token structure is valid
the signature was previously valid
the role assignments are correct
```

Always obtain a fresh token immediately before authentication or authorization tests:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

For human identities, run the PKCE flow again.

---

# Common JWT Claims

## `iss`

Issuer:

```text
https://auth.ai-platform.local/realms/ai-platform
```

The Gateway must require this exact issuer.

## `aud`

Audience:

```text
ai-platform-gateway
```

This proves that the token is intended for the AI Platform resource server.

## `azp`

Authorized party:

```text
ai-platform-service
```

or:

```text
ai-platform-cli
```

It identifies the client that requested the token.

## `sub`

Unique Keycloak subject identifier.

Do not use `sub` as a human-readable username.

## `preferred_username`

Human-readable identity such as:

```text
viewer-user
service-account-ai-platform-service
```

## `realm_access.roles`

Contains effective realm roles.

Example:

```json
{
  "realm_access": {
    "roles": [
      "model-deployer",
      "model-viewer"
    ]
  }
}
```

## `iat`

Issued-at time.

## `exp`

Expiration time.

The token must be rejected after this timestamp.

---

# Troubleshooting

## Token endpoint returns `invalid_client`

Check:

```text
client ID
client secret
client enabled state
serviceAccountsEnabled
clientAuthenticatorType
```

Verify the Secret keys:

```bash
kubectl get secret ai-platform-service-client-credentials \
  -n keycloak \
  -o json |
jq -r '.data | keys[]'
```

Do not print the decoded secret.

---

## Token endpoint returns `unauthorized_client`

Confirm the client supports the requested grant.

For machine tokens:

```text
serviceAccountsEnabled=true
```

For human PKCE:

```text
standardFlowEnabled=true
```

---

## Token has no expected audience

Check the audience mapper on:

```text
ai-platform-cli
ai-platform-service
```

The mapper must add:

```text
ai-platform-gateway
```

to the access-token audience.

---

## Token has no expected roles

Check direct role mappings:

```text
viewer-user → model-viewer
deployer-user → model-deployer
admin-user → platform-admin
service-account-ai-platform-service → model-deployer
```

Check composite hierarchy:

```text
model-deployer includes model-viewer
platform-admin includes model-deployer
```

---

## Signature validation cannot resolve Keycloak

Check:

```bash
getent hosts auth.ai-platform.local
```

When local DNS is unavailable, add the current Gateway IP to `/etc/hosts`.

---

## Signature validation reports unknown `kid`

Possible causes:

```text
Keycloak signing key rotated
JWKS file is stale
token was issued by another realm
token is corrupted
```

Download the JWKS again and compare the token header `kid`.

---

## Signature validation reports expired token

Request a fresh token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

or repeat the PKCE login.

---

## PKCE callback never arrives

Check:

```text
SSH tunnel is running
port 18080 is available
redirect URI matches the client configuration
browser redirects to 127.0.0.1:18080
helper is still waiting
```

Check whether the port is already in use:

```bash
ss -ltnp |
grep ':18080'
```

---

## Browser does not trust Keycloak HTTPS

Import or trust the CA certificate represented by:

```text
.local/keycloak/auth-ai-platform-root-ca.crt
```

Do not bypass TLS validation with `-k` in the documented validation path.

---

## Direct password grant does not work

This is expected.

The client intentionally has:

```text
directAccessGrantsEnabled=false
```

Use Authorization Code with PKCE for human users.

---

# Git Safety

Stage only scripts and non-secret configuration:

```bash
git add \
  infrastructure/keycloak/scripts/get-machine-token.sh \
  infrastructure/keycloak/scripts/decode-jwt.sh \
  infrastructure/keycloak/scripts/validate-machine-token.sh \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  infrastructure/keycloak/scripts/pkce-login.py
```

Verify no token material is staged:

```bash
git diff --cached --name-only |
grep -E \
  '\.jwt$|token-response|\.local/keycloak|service-client\.env|test-users\.env' &&
echo "ERROR: Token or credential material is staged" ||
echo "PASS: No token or credential material is staged"
```

Never commit:

```text
*.jwt
token response JSON files
client secrets
test-user passwords
private keys
.local/keycloak/
config/platform/keycloak/.secrets/
```

---

# Validation Sequence

## Machine identity

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

```bash
infrastructure/keycloak/scripts/validate-machine-token.sh \
  .local/keycloak/tokens/service-access-token.jwt
```

```bash
.local/keycloak/venv/bin/python \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  --token-file .local/keycloak/tokens/service-access-token.jwt \
  --jwks-url https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  --issuer https://auth.ai-platform.local/realms/ai-platform \
  --audience ai-platform-gateway \
  --ca-file .local/keycloak/auth-ai-platform-root-ca.crt
```

## Human identity

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

```bash
infrastructure/keycloak/scripts/decode-jwt.sh \
  .local/keycloak/tokens/user-access-token.jwt
```

```bash
.local/keycloak/venv/bin/python \
  infrastructure/keycloak/scripts/validate-jwt-signature.py \
  --token-file .local/keycloak/tokens/user-access-token.jwt \
  --jwks-url https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/certs \
  --issuer https://auth.ai-platform.local/realms/ai-platform \
  --audience ai-platform-gateway \
  --ca-file .local/keycloak/auth-ai-platform-root-ca.crt
```

---

# Completion Criteria

```text
[✓] Machine token obtained with client credentials
[✓] Machine token represents service-account-ai-platform-service
[✓] Machine token issuer is correct
[✓] Machine token audience contains ai-platform-gateway
[✓] Machine token includes model-deployer
[✓] Machine token includes inherited model-viewer
[✓] Human token obtained with Authorization Code and PKCE
[✓] Human token identifies the selected user
[✓] Human token contains the expected effective roles
[✓] Direct password grant remains disabled
[✓] JWT header and payload decoded safely
[✓] JWT signing key matched through kid
[✓] JWT signature validated through JWKS
[✓] Issuer, audience, and expiry validated
[✓] Short-lived token behavior documented
[✓] Token and credential files excluded from Git
```

---

# Resulting Token Trust Model

```text
Keycloak realm
  ↓ signs JWT
JWKS endpoint
  ↓ publishes public signing keys
Client
  ↓ sends bearer token
Validator
  ├── validates signature
  ├── validates issuer
  ├── validates audience
  ├── validates expiry
  └── reads effective roles
```

These validated claims are later consumed by Envoy Gateway for:

```text
JWT authentication
role-based authorization
HTTP-method authorization
```
