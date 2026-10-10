#!/usr/bin/env bash
set -euo pipefail

# End-to-end local compatibility smoke for the agent clients this gateway targets:
# - Anthropic/Claude Code style /v1/messages with x-api-key
# - OpenAI/Codex Responses /v1/responses
# - Groq SDK alias /openai/v1/chat/completions
# - Optional real Claude Code, Codex, and Claude Code Tamagotchi reference projects
#
# Defaults start this repository's gateway with mock upstreams on a temporary port.
# Override CCGW_SMOKE_BASE_URL to test an already-running gateway.
# Set CCGW_SMOKE_JSON_UPSTREAM=true to also start a deterministic OpenAI-compatible
# JSON upstream, proving clients that require JSON (Tamagotchi observations) do
# not merely fall back after parse failure.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOKEN="${CCGW_SMOKE_TOKEN:-admin123456}"
MODEL="${CCGW_SMOKE_MODEL:-x}"
PORT="${CCGW_SMOKE_PORT:-$((18100 + RANDOM % 700))}"
BASE_URL="${CCGW_SMOKE_BASE_URL:-http://127.0.0.1:${PORT}}"
RUN_LOG="${CCGW_SMOKE_RUN_LOG:-/tmp/ccgw-agent-smoke-${PORT}.log}"
GATEWAY_PID=""
JSON_UPSTREAM_PID=""
JSON_UPSTREAM_PORT="${CCGW_SMOKE_JSON_UPSTREAM_PORT:-$((19100 + RANDOM % 700))}"

CLAUDE_CODE_CLI="${CLAUDE_CODE_CLI:-/Users/sanbo/code/claude-code-source/cli.js}"
TAMAGOTCHI_DIR="${TAMAGOTCHI_DIR:-/Users/sanbo/code/claude-code-tamagotchi}"
CODEX_CLI="${CODEX_CLI:-$(command -v codex || true)}"

run_maybe_timeout() {
  if command -v timeout >/dev/null 2>&1; then
    timeout "$@"
  else
    shift
    "$@"
  fi
}

cleanup() {
  if [[ -n "${GATEWAY_PID}" ]]; then
    kill "${GATEWAY_PID}" >/dev/null 2>&1 || true
    wait "${GATEWAY_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${JSON_UPSTREAM_PID}" ]]; then
    kill "${JSON_UPSTREAM_PID}" >/dev/null 2>&1 || true
    wait "${JSON_UPSTREAM_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

start_json_upstream_if_needed() {
  if [[ "${CCGW_SMOKE_JSON_UPSTREAM:-false}" != "true" ]]; then
    return
  fi
  if [[ -n "${CCGW_SMOKE_BASE_URL:-}" ]]; then
    echo "WARN json upstream mode is ignored when CCGW_SMOKE_BASE_URL points at an existing gateway" >&2
    return
  fi

  python3 "${ROOT_DIR}/scripts/json_openai_mock.py" --port "${JSON_UPSTREAM_PORT}" \
    >/tmp/ccgw-json-upstream-${JSON_UPSTREAM_PORT}.stdout 2>&1 &
  JSON_UPSTREAM_PID="$!"

  for _ in {1..50}; do
    if curl -fsS "http://127.0.0.1:${JSON_UPSTREAM_PORT}/healthz" >/dev/null 2>&1; then
      export UPSTREAM_ADAPTERS_JSON="[{\"name\":\"json-smoke\",\"kind\":\"openai\",\"base_url\":\"http://127.0.0.1:${JSON_UPSTREAM_PORT}\",\"api_key\":\"local\",\"model\":\"${MODEL}\",\"supports_tools\":false}]"
      export UPSTREAM_DEFAULT_ROUTE="json-smoke"
      export UPSTREAM_MODEL_ROUTES_JSON="{\"*\": [\"json-smoke\"]}"
      export ENABLE_RESPONSE_JUDGE="false"
      echo "JSON upstream: http://127.0.0.1:${JSON_UPSTREAM_PORT}"
      return
    fi
    sleep 0.2
  done

  cat "/tmp/ccgw-json-upstream-${JSON_UPSTREAM_PORT}.stdout" >&2 || true
  echo "json upstream failed to start" >&2
  exit 1
}

start_gateway_if_needed() {
  if [[ -n "${CCGW_SMOKE_BASE_URL:-}" ]]; then
    return
  fi

  local bin="/tmp/ccgw-agent-smoke-${PORT}"
  (cd "${ROOT_DIR}" && go build -o "${bin}" ./cmd/cc-gateway)
  start_json_upstream_if_needed
  PORT="${PORT}" ADMIN_TOKEN="${TOKEN}" RUN_LOG_PATH="${RUN_LOG}" "${bin}" >/tmp/ccgw-agent-smoke-${PORT}.stdout 2>&1 &
  GATEWAY_PID="$!"

  for _ in {1..50}; do
    if curl -fsS "${BASE_URL}/healthz" >/dev/null 2>&1; then
      return
    fi
    sleep 0.2
  done

  cat "/tmp/ccgw-agent-smoke-${PORT}.stdout" >&2 || true
  echo "gateway failed to start at ${BASE_URL}" >&2
  exit 1
}

assert_status_200() {
  local name="$1"
  local path="$2"
  local body="$3"
  local headers_file="/tmp/ccgw-smoke-${PORT}-${name}.headers"
  local body_file="/tmp/ccgw-smoke-${PORT}-${name}.body"

  curl -fsS -D "${headers_file}" "${BASE_URL}${path}" \
    -H "authorization: Bearer ${TOKEN}" \
    -H "x-api-key: ${TOKEN}" \
    -H "anthropic-version: 2023-06-01" \
    -H "content-type: application/json" \
    --data-raw "${body}" >"${body_file}"
  echo "PASS ${name}: ${path}"
}

assert_get_200() {
  local name="$1"
  local path="$2"
  local body_file="/tmp/ccgw-smoke-${PORT}-${name}.body"

  curl -fsS "${BASE_URL}${path}" \
    -H "authorization: Bearer ${TOKEN}" \
    -H "x-api-key: ${TOKEN}" >"${body_file}"
  echo "PASS ${name}: ${path}"
}

start_gateway_if_needed

echo "Gateway: ${BASE_URL}"
curl -fsS "${BASE_URL}/healthz" >/dev/null
echo "PASS healthz"

assert_get_200 "openai_models" "/v1/models"
assert_get_200 "groq_sdk_models_alias" "/openai/v1/models"
assert_get_200 "openai_model_retrieve" "/v1/models/${MODEL}"
assert_get_200 "groq_sdk_model_retrieve_alias" "/openai/v1/models/${MODEL}"

assert_status_200 "anthropic_messages" "/v1/messages" "{
  \"model\":\"${MODEL}\",
  \"max_tokens\":64,
  \"stream\":false,
  \"messages\":[{\"role\":\"user\",\"content\":\"agent smoke: messages\"}]
}"

assert_status_200 "openai_chat" "/v1/chat/completions" "{
  \"model\":\"${MODEL}\",
  \"max_tokens\":64,
  \"messages\":[{\"role\":\"user\",\"content\":\"agent smoke: chat completions\"}]
}"

assert_status_200 "groq_sdk_alias" "/openai/v1/chat/completions" "{
  \"model\":\"${MODEL}\",
  \"max_tokens\":64,
  \"messages\":[{\"role\":\"user\",\"content\":\"agent smoke: groq alias\"}]
}"

assert_status_200 "responses" "/v1/responses" "{
  \"model\":\"${MODEL}\",
  \"max_output_tokens\":64,
  \"stream\":false,
  \"input\":[{\"role\":\"user\",\"content\":\"agent smoke: responses\"}]
}"

if [[ -f "${CLAUDE_CODE_CLI}" ]]; then
  HOME="/tmp/claude-ccgw-smoke-home-${PORT}" \
  ANTHROPIC_BASE_URL="${BASE_URL}" \
  ANTHROPIC_API_KEY="${TOKEN}" \
  run_maybe_timeout 35s node "${CLAUDE_CODE_CLI}" \
    --bare --no-session-persistence --disable-slash-commands --tools '' \
    --model "${MODEL}" -p 'Reply exactly OK.' >/tmp/ccgw-claude-smoke-${PORT}.out
  echo "PASS claude_code_cli"
else
  echo "SKIP claude_code_cli: ${CLAUDE_CODE_CLI} not found"
fi

if [[ -n "${CODEX_CLI}" ]] && [[ -x "${CODEX_CLI}" ]]; then
  mkdir -p "/tmp/codex-ccgw-smoke-home-${PORT}"
  NO_PROXY=127.0.0.1,localhost \
  no_proxy=127.0.0.1,localhost \
  CODEX_HOME="/tmp/codex-ccgw-smoke-home-${PORT}" \
  CCGATEWAY_API_KEY="${TOKEN}" \
  run_maybe_timeout 45s "${CODEX_CLI}" exec --json --ephemeral --ignore-rules --skip-git-repo-check -C /tmp \
    -c 'model_provider="ccgateway"' \
    -c "model=\"${MODEL}\"" \
    -c "model_providers.ccgateway={ name = \"CC Gateway\", base_url = \"${BASE_URL}/v1\", env_key = \"CCGATEWAY_API_KEY\", wire_api = \"responses\", stream_max_retries = 0 }" \
    'Reply exactly OK. Do not run commands.' </dev/null | tee "/tmp/ccgw-codex-smoke-${PORT}.jsonl" >/dev/null
  grep -q '"turn.completed"' "/tmp/ccgw-codex-smoke-${PORT}.jsonl"
  echo "PASS codex_cli"
else
  echo "SKIP codex_cli: set CODEX_CLI to a built Codex CLI binary or install codex in PATH"
fi

if [[ -d "${TAMAGOTCHI_DIR}" ]] && command -v bun >/dev/null 2>&1 && [[ -d "${TAMAGOTCHI_DIR}/node_modules/groq-sdk" ]]; then
  (
    cd "${TAMAGOTCHI_DIR}"
    GROQ_API_KEY="${TOKEN}" \
    GROQ_BASE_URL="${BASE_URL}" \
    GROQ_MODEL="${MODEL}" \
    bun -e 'import Groq from "groq-sdk";
const client = new Groq({ apiKey: process.env.GROQ_API_KEY, baseURL: process.env.GROQ_BASE_URL });
const completion = await client.chat.completions.create({
  model: process.env.GROQ_MODEL || "x",
  max_tokens: 64,
  messages: [{ role: "user", content: "agent smoke: groq sdk client" }]
});
if (!completion.choices?.[0]?.message) throw new Error("missing Groq SDK message");
console.log("PASS groq_sdk_client");'
  )
else
  echo "SKIP groq_sdk_client: set TAMAGOTCHI_DIR with installed node_modules/groq-sdk and bun"
fi

if [[ -d "${TAMAGOTCHI_DIR}" ]] && command -v bun >/dev/null 2>&1 && [[ -d "${TAMAGOTCHI_DIR}/node_modules/groq-sdk" ]]; then
  (
    cd "${TAMAGOTCHI_DIR}"
    PET_GROQ_API_KEY="${TOKEN}" \
    PET_GROQ_BASE_URL="${BASE_URL}/v1" \
    PET_GROQ_MODEL="${MODEL}" \
    PET_GROQ_TIMEOUT=30000 \
    bun -e 'import { GroqClient } from "./src/llm/GroqClient.ts";
const client = new GroqClient(process.env.PET_GROQ_API_KEY, process.env.PET_GROQ_MODEL || "x", Number(process.env.PET_GROQ_TIMEOUT || 30000), 1, undefined, process.env.PET_GROQ_BASE_URL);
const result = await client.analyzeUserMessage("agent smoke: tamagotchi", []);
if (process.env.CCGW_SMOKE_JSON_UPSTREAM === "true" && result.summary !== "local gateway json smoke") {
  throw new Error(`expected parsed JSON summary from local upstream, got ${JSON.stringify(result)}`);
}
console.log("PASS tamagotchi_client");'
  )
else
  echo "SKIP tamagotchi_client: set TAMAGOTCHI_DIR with installed node_modules/groq-sdk and bun"
fi

if [[ -f "${RUN_LOG}" ]]; then
  echo "Run log: ${RUN_LOG}"
  tail -n 8 "${RUN_LOG}"
fi
