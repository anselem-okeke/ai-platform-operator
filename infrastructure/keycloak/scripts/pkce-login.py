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
