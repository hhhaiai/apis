#!/usr/bin/env bash
set -euo pipefail

# Sends the script envelope to an external HTTP service as-is.
# The remote service should understand ccgateway.script_adapter.v1 protocol.

: "${SCRIPT_COMPLETE_URL:?SCRIPT_COMPLETE_URL is required}"

payload="$(cat)"

target_url="${SCRIPT_COMPLETE_URL}"
if printf "%s" "$payload" | grep -q '"mode":"stream"'; then
  target_url="${SCRIPT_STREAM_URL:-$SCRIPT_COMPLETE_URL}"
fi

auth_header=()
if [[ -n "${SCRIPT_API_KEY:-}" ]]; then
  auth_header=(-H "Authorization: Bearer ${SCRIPT_API_KEY}")
fi

curl --fail --silent --show-error \
  -X POST "${target_url}" \
  -H "Content-Type: application/json" \
  "${auth_header[@]}" \
  --data-binary "$payload"
