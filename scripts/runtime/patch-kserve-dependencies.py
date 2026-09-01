from pathlib import Path


SCRIPT = Path(__file__).resolve()
REPO_ROOT = SCRIPT.parents[2]

ROOT = (
    REPO_ROOT
    / ".runtime-src"
    / "kserve"
    / "python"
)

kserve = ROOT / "kserve" / "pyproject.toml"
storage = ROOT / "storage" / "pyproject.toml"


def replace(path: Path, old: str, new: str) -> None:
    if not path.exists():
        raise SystemExit(
            f"Expected file not found: {path}"
        )

    text = path.read_text()

    if old not in text:
        raise SystemExit(
            f"Expected dependency not found in {path}: {old}"
        )

    path.write_text(
        text.replace(old, new)
    )


# ------------------------------------------------------------
# Preserve the known-working FastAPI version.
#
# KServe v0.19.0 declares only a minimum. Allowing an
# unrestricted lock refresh selected unrelated FastAPI
# releases during remediation.
# ------------------------------------------------------------

replace(
    kserve,
    '"fastapi>=0.115.3"',
    '"fastapi==0.136.1"',
)


# ------------------------------------------------------------
# KServe runtime security floors
# ------------------------------------------------------------

replace(
    kserve,
    '"starlette==0.49.1"',
    '"starlette>=1.3.1"',
)

replace(
    kserve,
    '"urllib3>=2.6.0"',
    '"urllib3>=2.7.0"',
)

replace(
    kserve,
    '"aiohttp>=3.13.3"',
    '"aiohttp>=3.14.3"',
)

replace(
    kserve,
    '"cryptography>=46.0.5"',
    '"cryptography>=50.0.0"',
)

replace(
    kserve,
    '"python-multipart>=0.0.22"',
    '"python-multipart>=0.0.30"',
)

replace(
    kserve,
    '"pyjwt>=2.12.0"',
    '"pyjwt>=2.13.0"',
)

replace(
    kserve,
    '"pyasn1>=0.6.3"',
    '"pyasn1>=0.6.4"',
)


# ------------------------------------------------------------
# KServe storage security floors
# ------------------------------------------------------------

replace(
    storage,
    '"aiohttp<4.0.0,>=3.10.0"',
    '"aiohttp<4.0.0,>=3.14.3"',
)

replace(
    storage,
    '"dulwich>=0.21.0"',
    '"dulwich>=1.2.5"',
)

replace(
    storage,
    '"cryptography>=46.0.5"',
    '"cryptography>=50.0.0"',
)

replace(
    storage,
    '"pyjwt>=2.12.0"',
    '"pyjwt>=2.13.0"',
)

replace(
    storage,
    '"pyasn1>=0.6.3"',
    '"pyasn1>=0.6.4"',
)




# ------------------------------------------------------------
# KServe storage transitive security floors
#
# These packages are pulled transitively by the cloud-storage
# SDKs. Declare explicit floors so a fresh lock cannot resolve
# vulnerable versions.
# ------------------------------------------------------------

replace(
    storage,
    '    "azure-core>=1.38.0"\n]',
    '    "azure-core>=1.38.0",\n'
    '    "urllib3>=2.7.0",\n'
    '    "protobuf>=6.33.5"\n'
    ']',
)

print(
    "Patched KServe runtime dependency policy in:",
    ROOT,
)
