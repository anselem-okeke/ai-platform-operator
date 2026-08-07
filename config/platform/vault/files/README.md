Place the public Vault HTTPS CA here as `vault-server-ca.crt`.

```bash
cp .local/tls/vault-server-ca.crt   config/platform/vault/files/vault-server-ca.crt
```

Never place private keys, Vault tokens, unseal keys, or ServiceAccount JWTs here.
