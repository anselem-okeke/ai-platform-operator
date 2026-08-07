#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
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
        jwks_client = PyJWKClient(
            args.jwks_url,
            ssl_context=__import__("ssl").create_default_context(
                cafile=args.ca_file
            ),
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
        print(f"ERROR: Unable to retrieve or process JWKS: {exc}", file=sys.stderr)
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
