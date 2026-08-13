#!/usr/bin/env bash
set -euo pipefail

cd /mnt/data/ai-platform-operator

API_URL="${API_URL:-http://127.0.0.1:18081}"
TOKEN_FILE="${TOKEN_FILE:-.local/keycloak/tokens/service-access-token.jwt}"

echo "Getting fresh machine token..."
infrastructure/keycloak/scripts/get-machine-token.sh

TOKEN="$(
  cat "${TOKEN_FILE}"
)"

cleanup() {
  unset TOKEN
}

trap cleanup EXIT

echo
echo "1/5 Generating successful 200 traffic..."

for i in $(seq 1 100); do
  curl \
    -sS \
    -H "Authorization: Bearer ${TOKEN}" \
    "${API_URL}/api/v1/model-services" \
    >/dev/null

  curl \
    -sS \
    -H "Authorization: Bearer ${TOKEN}" \
    "${API_URL}/api/v1/model-services/fraud-model" \
    >/dev/null

  sleep 0.1
done

echo
echo "2/5 Generating 401 traffic..."

for i in $(seq 1 20); do
  curl \
    -sS \
    "${API_URL}/api/v1/model-services" \
    >/dev/null
done

echo
echo "3/5 Generating 404 traffic..."

for i in $(seq 1 20); do
  curl \
    -sS \
    -H "Authorization: Bearer ${TOKEN}" \
    "${API_URL}/api/v1/model-services/does-not-exist" \
    >/dev/null
done

echo
echo "4/5 Generating 405 traffic..."

for i in $(seq 1 20); do
  curl \
    -sS \
    -X DELETE \
    -H "Authorization: Bearer ${TOKEN}" \
    "${API_URL}/api/v1/model-services" \
    >/dev/null
done

echo
echo "5/5 Generating concurrent traffic for in-flight requests..."

for round in $(seq 1 40); do
  seq 1 50 | xargs -P50 -I{} \
    curl \
      -sS \
      -H "Authorization: Bearer ${TOKEN}" \
      "${API_URL}/api/v1/model-services" \
      -o /dev/null

  sleep 0.2
done

echo
echo "Traffic generation complete."
echo
echo "Expected dashboard signals:"
echo "  HTTP 200"
echo "  HTTP 401"
echo "  HTTP 404"
echo "  HTTP 405"
echo "  Request Rate by Route"
echo "  p95 Latency by Route"
echo "  Requests In Flight may briefly rise above 0"
echo
echo "Wait at least one Prometheus scrape interval, then refresh Grafana."
