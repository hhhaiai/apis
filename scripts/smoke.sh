#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ANTHROPIC_VERSION="${ANTHROPIC_VERSION:-2023-06-01}"

echo "== healthz =="
curl -sS "$BASE_URL/healthz"
echo

echo "== /v1/messages =="
curl -sS -X POST "$BASE_URL/v1/messages" \
  -H "anthropic-version: $ANTHROPIC_VERSION" \
  -H "content-type: application/json" \
  -d '{
    "model":"claude-test",
    "max_tokens":128,
    "messages":[{"role":"user","content":"hello from smoke"}]
  }'
echo

echo "== /v1/chat/completions =="
curl -sS -X POST "$BASE_URL/v1/chat/completions" \
  -H "content-type: application/json" \
  -d '{
    "model":"claude-test",
    "messages":[{"role":"user","content":"hello openai smoke"}],
    "max_tokens":128
  }'
echo

echo "== /v1/responses =="
curl -sS -X POST "$BASE_URL/v1/responses" \
  -H "content-type: application/json" \
  -d '{
    "model":"claude-test",
    "input":"hello responses smoke",
    "max_output_tokens":128
  }'
echo

