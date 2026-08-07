# Gateway JWT Role Matrix Validation

## Purpose

This validation confirms that Envoy Gateway enforces:

- JWT authentication
- role-based authorization
- HTTP method restrictions

## Refresh tokens

Generate a fresh machine token:

```bash
infrastructure/keycloak/scripts/get-machine-token.sh
```

For human users, run the PKCE login helper:

```bash
infrastructure/keycloak/scripts/pkce-login.py
```

## Log out before switching users

To avoid Keycloak reusing the previous browser session, open:

```text
https://auth.ai-platform.local/realms/ai-platform/protocol/openid-connect/logout
```

Then run the PKCE login helper again and sign in as the next user.

A private/incognito browser window can also be used.

## Save separate user tokens

After logging in as the viewer:

```bash
cp \
  .local/keycloak/tokens/user-access-token.jwt \
  .local/keycloak/tokens/viewer-access-token.jwt
```

After logging in as the administrator:

```bash
cp \
  .local/keycloak/tokens/user-access-token.jwt \
  .local/keycloak/tokens/admin-access-token.jwt
```

Restrict permissions:

```bash
chmod 600 \
  .local/keycloak/tokens/viewer-access-token.jwt \
  .local/keycloak/tokens/admin-access-token.jwt \
  .local/keycloak/tokens/service-access-token.jwt
```

## Run the automated matrix

```bash
infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-gateway-role-matrix.sh

or 

infrastructure/keycloak/scripts/get-machine-token.sh &&
infrastructure/keycloak/scripts/validate-oidc-end-to-end.sh
```

## Expected results

| Identity | Method | Expected status | Meaning |
|---|---|---:|---|
| missing-token | GET | 401 | Authentication failed |
| invalid-token | GET | 401 | Authentication failed |
| model-viewer | GET | 200 | Allowed |
| model-viewer | POST | 403 | Denied by Gateway |
| model-viewer | DELETE | 403 | Denied by Gateway |
| model-deployer | GET | 200 | Allowed |
| model-deployer | POST | 405 | Gateway allowed it; Nginx rejected the method |
| model-deployer | DELETE | 403 | Denied by Gateway |
| platform-admin | GET | 200 | Allowed |
| platform-admin | POST | 405 | Gateway allowed it; Nginx rejected the method |
| platform-admin | DELETE | 405 | Gateway allowed it; Nginx rejected the method |

## How to interpret status codes

```text
401 = JWT authentication failed
403 = JWT was valid, but Gateway authorization denied the request
405 = Gateway authorized the request, but the Nginx backend does not support that HTTP method
200 = Gateway and backend both accepted the request
```

## Successful validation

A successful run ends with:

```text
PASS: Gateway authentication and role-authorization matrix validated.
```
