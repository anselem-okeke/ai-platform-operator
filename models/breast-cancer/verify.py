#!/usr/bin/env python3

import argparse
import hashlib
import json
from pathlib import Path

import joblib


MODEL_NAME = "breast-cancer"
MODEL_FORMAT = "sklearn"
RUNTIME = "kserve-sklearnserver"
ARTIFACT_NAME = "model.joblib"
MANIFEST_NAME = "manifest.json"
CHECKSUMS_NAME = "checksums.sha256"
REQUIRED_FILES = {
    ARTIFACT_NAME,
    MANIFEST_NAME,
    CHECKSUMS_NAME,
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()

    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)

    return digest.hexdigest()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Verify the packaged Breast cancer model artifact"
    )
    parser.add_argument(
        "--artifact-dir",
        required=True,
        type=Path,
    )
    parser.add_argument(
        "--expected-model-version",
        required=True,
    )
    return parser.parse_args()


def read_checksums(path: Path) -> dict[str, str]:
    checksums: dict[str, str] = {}

    for number, raw_line in enumerate(
        path.read_text().splitlines(),
        start=1,
    ):
        line = raw_line.strip()

        if not line:
            continue

        parts = line.split(maxsplit=1)

        if len(parts) != 2:
            raise SystemExit(
                "ERROR: invalid checksum line "
                f"{number}: {raw_line!r}"
            )

        digest, filename = parts
        filename = filename.lstrip("*").strip()

        if (
            len(digest) != 64
            or any(
                character not in "0123456789abcdef"
                for character in digest
            )
        ):
            raise SystemExit(
                "ERROR: invalid SHA-256 digest on line "
                f"{number}"
            )

        if filename in checksums:
            raise SystemExit(
                "ERROR: duplicate checksum entry: "
                f"{filename}"
            )

        checksums[filename] = digest

    return checksums


def main() -> None:
    args = parse_args()

    artifact_dir = args.artifact_dir.resolve()

    if not artifact_dir.is_dir():
        raise SystemExit(
            f"ERROR: artifact directory not found: "
            f"{artifact_dir}"
        )

    actual_files = {
        path.name
        for path in artifact_dir.iterdir()
        if path.is_file()
    }

    if actual_files != REQUIRED_FILES:
        raise SystemExit(
            "ERROR: unexpected artifact file set: "
            f"expected={sorted(REQUIRED_FILES)} "
            f"actual={sorted(actual_files)}"
        )

    checksums = read_checksums(
        artifact_dir / CHECKSUMS_NAME
    )

    expected_checksum_files = {
        ARTIFACT_NAME,
        MANIFEST_NAME,
    }

    if set(checksums) != expected_checksum_files:
        raise SystemExit(
            "ERROR: checksum entries do not match "
            "the required artifact files"
        )

    for filename, expected_digest in checksums.items():
        actual_digest = sha256(
            artifact_dir / filename
        )

        if actual_digest != expected_digest:
            raise SystemExit(
                "ERROR: checksum mismatch for "
                f"{filename}: "
                f"expected={expected_digest} "
                f"actual={actual_digest}"
            )

    manifest = json.loads(
        (artifact_dir / MANIFEST_NAME).read_text()
    )

    model = manifest.get("model", {})

    expected_values = {
        "name": MODEL_NAME,
        "version": args.expected_model_version,
        "format": MODEL_FORMAT,
        "runtime": RUNTIME,
        "artifact": ARTIFACT_NAME,
    }

    for field, expected in expected_values.items():
        actual = model.get(field)

        if actual != expected:
            raise SystemExit(
                f"ERROR: manifest model.{field} mismatch: "
                f"expected={expected!r} actual={actual!r}"
            )

    artifact_digest = sha256(
        artifact_dir / ARTIFACT_NAME
    )

    if model.get("sha256") != artifact_digest:
        raise SystemExit(
            "ERROR: manifest model SHA-256 does not "
            "match model.joblib"
        )

    source = manifest.get("source", {})

    if not source.get("revision"):
        raise SystemExit(
            "ERROR: manifest source revision is missing"
        )

    validation = manifest.get("validation", {})
    sample_input = validation.get("sampleInput")
    expected_prediction = validation.get(
        "expectedPrediction"
    )

    if not isinstance(sample_input, list):
        raise SystemExit(
            "ERROR: validation sample input is missing"
        )

    if not isinstance(expected_prediction, list):
        raise SystemExit(
            "ERROR: expected prediction is missing"
        )

    loaded_model = joblib.load(
        artifact_dir / ARTIFACT_NAME
    )

    actual_prediction = [
        int(value)
        for value in loaded_model.predict(
            sample_input
        ).tolist()
    ]

    if actual_prediction != expected_prediction:
        raise SystemExit(
            "ERROR: loaded model prediction mismatch: "
            f"expected={expected_prediction} "
            f"actual={actual_prediction}"
        )

    print(
        json.dumps(
            {
                "artifactDir": str(artifact_dir),
                "artifactSha256": artifact_digest,
                "modelName": model["name"],
                "modelVersion": model["version"],
                "prediction": actual_prediction,
                "sourceRevision": source["revision"],
            },
            indent=2,
            sort_keys=True,
        )
    )
    print("PASS: Breast cancer model artifact verified")


if __name__ == "__main__":
    main()
