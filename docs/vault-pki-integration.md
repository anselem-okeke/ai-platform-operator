# Vault PKI integration

The AI Platform operator does not call Vault directly. cert-manager authenticates
to Vault, requests a certificate, and manages the TLS Secret used by the shared
Gateway. This keeps the operator provider-neutral.
