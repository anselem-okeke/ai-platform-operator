# Keycloak Python Virtual Environment Setup

Run these commands from `/mnt/data/ai-platform-operator`.

## 1. Install virtual-environment support

```bash
sudo apt update
sudo apt install -y python3.12-venv
```

## 2. Create the environment

`/mnt/data` does not support the symlinks normally created by `venv`, so pre-create `lib64` and use `--copies`:

```bash
mkdir -p .local/keycloak/venv/lib64
python3 -m venv --copies .local/keycloak/venv
```

## 3. Install PyJWT

```bash
.local/keycloak/venv/bin/python -m pip install \
  --disable-pip-version-check \
  'PyJWT[crypto]>=2.10,<3'
```

## 4. Verify

```bash
.local/keycloak/venv/bin/python -c \
  "import jwt; print(jwt.__version__)"
```

Optional activation:

```bash
source .local/keycloak/venv/bin/activate
```
