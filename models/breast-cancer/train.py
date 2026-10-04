#!/usr/bin/env python3

import argparse
import hashlib
import json
import platform
from importlib import metadata
from pathlib import Path

import joblib
import numpy as np
from sklearn.datasets import load_breast_cancer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import accuracy_score
from sklearn.model_selection import train_test_split
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler


RANDOM_STATE = 42
MIN_TEST_ACCURACY = 0.95
MODEL_NAME = "breast-cancer"
MODEL_FORMAT = "sklearn"
RUNTIME = "kserve-sklearnserver"
ARTIFACT_NAME = "model.joblib"
MANIFEST_NAME = "manifest.json"
CHECKSUMS_NAME = "checksums.sha256"
SAMPLE_INPUT = [[
    17.99, 10.38, 122.8, 1001.0, 0.1184, 0.2776,
    0.3001, 0.1471, 0.2419, 0.07871, 1.095, 0.9053,
    8.589, 153.4, 0.006399, 0.04904, 0.05373, 0.01587,
    0.03003, 0.006193, 25.38, 17.33, 184.6, 2019.0,
    0.1622, 0.6656, 0.7119, 0.2654, 0.4601, 0.1189,
]]
EXPECTED_SAMPLE_PREDICTION = [0]


def sha256(path: Path) -> str:
    digest = hashlib.sha256()

    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)

    return digest.hexdigest()


def package_version(name: str) -> str:
    return metadata.version(name)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Train and package the Breast cancer sklearn model"
    )
    parser.add_argument(
        "--output-dir",
        required=True,
        type=Path,
    )
    parser.add_argument(
        "--model-version",
        required=True,
    )
    parser.add_argument(
        "--source-revision",
        required=True,
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()

    if not args.model_version.strip():
        raise SystemExit("ERROR: model version must not be empty")

    if not args.source_revision.strip():
        raise SystemExit("ERROR: source revision must not be empty")

    output_dir = args.output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    output_paths = {
        ARTIFACT_NAME: output_dir / ARTIFACT_NAME,
        MANIFEST_NAME: output_dir / MANIFEST_NAME,
        CHECKSUMS_NAME: output_dir / CHECKSUMS_NAME,
    }

    existing = [
        name
        for name, path in output_paths.items()
        if path.exists()
    ]

    if existing:
        raise SystemExit(
            "ERROR: refusing to overwrite existing outputs: "
            + ", ".join(sorted(existing))
        )

    dataset = load_breast_cancer()

    features = np.asarray(dataset.data)
    targets = np.asarray(dataset.target)

    (
        train_features,
        test_features,
        train_targets,
        test_targets,
    ) = train_test_split(
        features,
        targets,
        test_size=0.2,
        random_state=RANDOM_STATE,
        stratify=targets,
    )

    model = Pipeline(
        steps=[
            ("scaler", StandardScaler()),
            (
                "classifier",
                LogisticRegression(
                    max_iter=500,
                    random_state=RANDOM_STATE,
                ),
            ),
        ]
    )

    model.fit(train_features, train_targets)

    test_predictions = model.predict(test_features)
    accuracy = float(
        accuracy_score(test_targets, test_predictions)
    )

    if accuracy < MIN_TEST_ACCURACY:
        raise SystemExit(
            f"ERROR: test accuracy {accuracy:.6f} is below "
            f"the demo acceptance floor {MIN_TEST_ACCURACY:.2f}"
        )

    sample_prediction = [
        int(value)
        for value in model.predict(SAMPLE_INPUT).tolist()
    ]

    if sample_prediction != EXPECTED_SAMPLE_PREDICTION:
        raise SystemExit(
            "ERROR: unexpected sample prediction: "
            f"{sample_prediction}"
        )

    artifact_path = output_paths[ARTIFACT_NAME]

    joblib.dump(
        model,
        artifact_path,
        compress=3,
    )

    artifact_sha256 = sha256(artifact_path)

    manifest = {
        "schemaVersion": 1,
        "model": {
            "name": MODEL_NAME,
            "version": args.model_version,
            "format": MODEL_FORMAT,
            "runtime": RUNTIME,
            "artifact": ARTIFACT_NAME,
            "sha256": artifact_sha256,
        },
        "source": {
            "repository": (
                "https://github.com/anselem-okeke/"
                "ai-platform-operator"
            ),
            "revision": args.source_revision,
            "trainingScript": "models/breast-cancer/train.py",
            "dataset": "sklearn.datasets.load_breast_cancer",
        },
        "training": {
            "randomState": RANDOM_STATE,
            "trainingSamples": int(train_features.shape[0]),
            "testSamples": int(test_features.shape[0]),
            "features": int(features.shape[1]),
            "classes": [
                str(value)
                for value in dataset.target_names.tolist()
            ],
            "testAccuracy": accuracy,
            "minimumTestAccuracy": MIN_TEST_ACCURACY,
        },
        "compatibility": {
            "python": platform.python_version(),
            "numpy": package_version("numpy"),
            "joblib": package_version("joblib"),
            "scikitLearn": package_version(
                "scikit-learn"
            ),
        },
        "validation": {
            "sampleInput": SAMPLE_INPUT,
            "expectedPrediction": (
                EXPECTED_SAMPLE_PREDICTION
            ),
        },
    }

    manifest_path = output_paths[MANIFEST_NAME]

    manifest_path.write_text(
        json.dumps(
            manifest,
            indent=2,
            sort_keys=True,
        )
        + "\n"
    )

    manifest_sha256 = sha256(manifest_path)

    output_paths[CHECKSUMS_NAME].write_text(
        f"{artifact_sha256}  {ARTIFACT_NAME}\n"
        f"{manifest_sha256}  {MANIFEST_NAME}\n"
    )

    print(
        json.dumps(
            {
                "artifact": str(artifact_path),
                "artifactSha256": artifact_sha256,
                "manifest": str(manifest_path),
                "modelVersion": args.model_version,
                "samplePrediction": sample_prediction,
                "testAccuracy": accuracy,
            },
            indent=2,
            sort_keys=True,
        )
    )
    print("PASS: Breast cancer model artifact created")


if __name__ == "__main__":
    main()
