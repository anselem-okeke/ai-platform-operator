# Vault PKI integration

1. Apply ServiceAccount and RBAC.
2. Export and copy the kind cluster CA to the jumpbox.
3. Configure the Vault PKI engine and dedicated Kubernetes auth mount.
4. Copy the public Vault HTTPS CA to `config/platform/vault/files/vault-server-ca.crt`.
5. Run `kubectl apply -k config/platform/vault`.
6. Wait for `issuer/vault-issuer` to become Ready.
7. Apply `fraud-model-vault-certificate.yaml`.

Never commit tokens, unseal keys, private keys, ServiceAccount JWTs, or `variables.env`.

## Validation

Validate the initial Vault PKI integration:

```bash
infrastructure/vault/scripts/validate-vault-pki-integration.sh
