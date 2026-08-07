#!/usr/bin/env bash
set -euo pipefail

TOKEN="${1:-}"

if [[ -z "${TOKEN}" ]]; then
  echo "Usage: $0 <jwt-or-token-file>" >&2
  exit 1
fi

if [[ -f "${TOKEN}" ]]; then
  TOKEN="$(cat "${TOKEN}")"
fi

decode_segment() {
  local segment="$1"
  local remainder

  segment="${segment//-/+}"
  segment="${segment//_//}"

  remainder=$(( ${#segment} % 4 ))

  case "${remainder}" in
    0)
      ;;
    2)
      segment="${segment}=="
      ;;
    3)
      segment="${segment}="
      ;;
    *)
      echo "ERROR: Invalid base64url segment." >&2
      return 1
      ;;
  esac

  printf '%s' "${segment}" |
  base64 --decode
}

IFS='.' read -r header payload signature <<<"${TOKEN}"

if [[ -z "${header}" || -z "${payload}" || -z "${signature}" ]]; then
  echo "ERROR: Input is not a three-part JWT." >&2
  exit 1
fi

echo "Header:"
decode_segment "${header}" |
jq .

echo
echo "Payload:"
decode_segment "${payload}" |
jq .
