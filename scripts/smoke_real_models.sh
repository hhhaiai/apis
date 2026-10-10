#!/usr/bin/env bash
set -euo pipefail

# Real upstream quality smoke for local 国产模型.
#
# This is intentionally stricter than scripts/smoke_agent_clients.sh:
# it refuses to silently pass when the real upstream model service is absent.
# It validates that a real OpenAI-compatible local model can:
# 1. answer through the gateway,
# 2. receive response_format=json_object through the gateway,
# 3. return parseable JSON,
# 4. power Claude Code Tamagotchi without falling back to the original prompt.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TOKEN="${CCGW_REAL_SMOKE_TOKEN:-admin123456}"
PORT="${CCGW_REAL_SMOKE_PORT:-$((18800 + RANDOM % 700))}"
BASE_URL="${CCGW_REAL_SMOKE_BASE_URL:-http://127.0.0.1:${PORT}}"
MODEL="${CCGW_REAL_SMOKE_MODEL:-GLM-5}"
UPSTREAM_NAME="${CCGW_REAL_SMOKE_UPSTREAM_NAME:-glm-real}"
UPSTREAM_URL="${CCGW_REAL_SMOKE_UPSTREAM_URL:-http://127.0.0.1:5025}"
UPSTREAM_API_KEY="${CCGW_REAL_SMOKE_UPSTREAM_API_KEY:-free}"
UPSTREAM_ENDPOINT="${CCGW_REAL_SMOKE_UPSTREAM_ENDPOINT:-/v1/chat/completions}"
UPSTREAM_URL_WAS_SET="${CCGW_REAL_SMOKE_UPSTREAM_URL:-}"
RUN_LOG="${CCGW_REAL_SMOKE_RUN_LOG:-/tmp/ccgw-real-model-smoke-${PORT}.log}"
TAMAGOTCHI_DIR="${TAMAGOTCHI_DIR:-/Users/sanbo/code/claude-code-tamagotchi}"
GATEWAY_PID=""

cleanup() {
  if [[ -n "${GATEWAY_PID}" ]]; then
    kill "${GATEWAY_PID}" >/dev/null 2>&1 || true
    wait "${GATEWAY_PID}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

json_chat_payload() {
  python3 - "$MODEL" <<'PY'
import json, sys
model = sys.argv[1]
print(json.dumps({
  "model": model,
  "max_tokens": 256,
  "response_format": {"type": "json_object"},
  "messages": [
    {
      "role": "system",
      "content": "Return JSON only. No markdown. No prose."
    },
    {
      "role": "user",
      "content": "Return exactly a JSON object with keys summary and intent. summary must be real_model_smoke. intent must be gateway_quality_check."
    }
  ]
}, ensure_ascii=False))
PY
}

assert_openai_json_response() {
  local url="$1"
  local token="$2"
  local label="$3"
  local endpoint="${4:-/v1/chat/completions}"
  local outfile="/tmp/ccgw-real-${PORT}-${label}.json"

  if ! curl -fsS --max-time "${CCGW_REAL_SMOKE_TIMEOUT:-60}" "${url}${endpoint}" \
    -H "authorization: Bearer ${token}" \
    -H "x-api-key: ${token}" \
    -H "content-type: application/json" \
    --data-raw "$(json_chat_payload)" >"${outfile}"; then
    return 1
  fi

  python3 - "${outfile}" "${label}" <<'PY'
import json, sys
path, label = sys.argv[1], sys.argv[2]
raw = json.load(open(path))
content = raw["choices"][0]["message"].get("content") or ""
try:
    parsed = json.loads(content)
except Exception as exc:
    raise SystemExit(f"{label}: model content is not JSON: {content[:500]!r}; error={exc}")
if not isinstance(parsed, dict):
    raise SystemExit(f"{label}: parsed content is not an object: {parsed!r}")
missing = [k for k in ("summary", "intent") if k not in parsed]
if missing:
    raise SystemExit(f"{label}: missing keys {missing}; parsed={parsed!r}")
print(f"PASS {label}: parsed JSON content {parsed}")
PY
}

probe_upstream() {
  echo "Checking real upstream ${UPSTREAM_URL}${UPSTREAM_ENDPOINT} ..."
  if ! assert_openai_json_response "${UPSTREAM_URL}" "${UPSTREAM_API_KEY}" "direct_real_upstream_json" "${UPSTREAM_ENDPOINT}"; then
    if [[ -z "${UPSTREAM_URL_WAS_SET}" && "${CCGW_REAL_SMOKE_AUTO_DISCOVER:-true}" == "true" ]]; then
      echo "Default real upstream is not ready; trying localhost auto-discovery ..." >&2
      local discovered
      local discover_keys=("${UPSTREAM_API_KEY}" admin123456 free)
      if [[ -n "${CCGW_REAL_SMOKE_DISCOVER_KEYS:-}" ]]; then
        # shellcheck disable=SC2206
        discover_keys+=(${CCGW_REAL_SMOKE_DISCOVER_KEYS})
      fi
      if discovered="$(python3 "${ROOT_DIR}/scripts/discover_openai_upstreams.py" \
        --format shell \
        --timeout "${CCGW_REAL_SMOKE_DISCOVER_TIMEOUT:-1}" \
        --keys "${discover_keys[@]}" \
        --models "${MODEL}" GLM-5 GLM-4.7 qwen-max deepseek-chat 2>/dev/null)"; then
        eval "${discovered}"
        UPSTREAM_URL="${CCGW_REAL_SMOKE_UPSTREAM_URL}"
        UPSTREAM_API_KEY="${CCGW_REAL_SMOKE_UPSTREAM_API_KEY}"
        MODEL="${CCGW_REAL_SMOKE_MODEL}"
        UPSTREAM_ENDPOINT="${CCGW_REAL_SMOKE_UPSTREAM_ENDPOINT:-/v1/chat/completions}"
        echo "Auto-discovered real upstream ${UPSTREAM_URL}${UPSTREAM_ENDPOINT} model ${MODEL}; retrying ..."
        assert_openai_json_response "${UPSTREAM_URL}" "${UPSTREAM_API_KEY}" "direct_real_upstream_json" "${UPSTREAM_ENDPOINT}"
        return
      fi
    fi
    if [[ "${CCGW_REAL_SMOKE_PRINT_DISCOVERY:-true}" == "true" && "${CCGW_REAL_SMOKE_AUTO_DISCOVER:-true}" == "true" ]]; then
      echo "Discovery details for local OpenAI-compatible candidates:" >&2
      local report_keys=("${UPSTREAM_API_KEY}" admin123456 free)
      if [[ -n "${CCGW_REAL_SMOKE_DISCOVER_KEYS:-}" ]]; then
        # shellcheck disable=SC2206
        report_keys+=(${CCGW_REAL_SMOKE_DISCOVER_KEYS})
      fi
      python3 "${ROOT_DIR}/scripts/discover_openai_upstreams.py" \
        --timeout "${CCGW_REAL_SMOKE_DISCOVER_TIMEOUT:-1}" \
        --keys "${report_keys[@]}" \
        --models "${MODEL}" GLM-5 GLM-4.7 qwen-max deepseek-chat >&2 || true
    fi
    cat >&2 <<EOF
REAL_MODEL_NOT_READY: ${UPSTREAM_URL} did not pass OpenAI-compatible JSON smoke.

Start your real local model service first, or point this script at it:

  CCGW_REAL_SMOKE_UPSTREAM_URL=http://127.0.0.1:5025 \\
  CCGW_REAL_SMOKE_UPSTREAM_API_KEY=free \\
  CCGW_REAL_SMOKE_MODEL=GLM-5 \\
  bash scripts/smoke_real_models.sh

This is the remaining hard gate before claiming real国产模型 quality is verified.
EOF
    exit 2
  fi
}

start_gateway_if_needed() {
  if [[ -n "${CCGW_REAL_SMOKE_BASE_URL:-}" ]]; then
    return
  fi

  local bin="/tmp/ccgw-real-model-smoke-${PORT}"
  (cd "${ROOT_DIR}" && go build -o "${bin}" ./cmd/cc-gateway)

  export UPSTREAM_ADAPTERS_JSON="$(
    python3 - "${UPSTREAM_NAME}" "${UPSTREAM_URL}" "${UPSTREAM_API_KEY}" "${MODEL}" "${UPSTREAM_ENDPOINT}" <<'PY'
import json, sys
name, base_url, api_key, model, endpoint = sys.argv[1:]
print(json.dumps([{
    "name": name,
    "kind": "openai",
    "base_url": base_url,
    "api_key": api_key,
    "model": model,
    "endpoint": endpoint,
    "supports_tools": False,
}], ensure_ascii=False))
PY
  )"
  export UPSTREAM_DEFAULT_ROUTE="${UPSTREAM_NAME}"
  export UPSTREAM_MODEL_ROUTES_JSON="{\"*\": [\"${UPSTREAM_NAME}\"], \"${MODEL}\": [\"${UPSTREAM_NAME}\"]}"
  export ENABLE_RESPONSE_JUDGE="false"

  PORT="${PORT}" ADMIN_TOKEN="${TOKEN}" RUN_LOG_PATH="${RUN_LOG}" "${bin}" >/tmp/ccgw-real-model-smoke-${PORT}.stdout 2>&1 &
  GATEWAY_PID="$!"

  for _ in {1..60}; do
    if curl -fsS "${BASE_URL}/healthz" >/dev/null 2>&1; then
      return
    fi
    sleep 0.2
  done

  cat "/tmp/ccgw-real-model-smoke-${PORT}.stdout" >&2 || true
  echo "gateway failed to start at ${BASE_URL}" >&2
  exit 1
}

probe_tamagotchi() {
  if [[ ! -d "${TAMAGOTCHI_DIR}" ]] || ! command -v bun >/dev/null 2>&1; then
    echo "SKIP tamagotchi_real_model: missing ${TAMAGOTCHI_DIR} or bun"
    return
  fi

  (
    cd "${TAMAGOTCHI_DIR}"
    PET_GROQ_API_KEY="${TOKEN}" \
    PET_GROQ_BASE_URL="${BASE_URL}/v1" \
    PET_GROQ_MODEL="${MODEL}" \
    PET_GROQ_TIMEOUT="${CCGW_REAL_SMOKE_TIMEOUT_MS:-60000}" \
    bun -e 'import { GroqClient } from "./src/llm/GroqClient.ts";
const prompt = "real model Tamagotchi smoke";
const client = new GroqClient(process.env.PET_GROQ_API_KEY, process.env.PET_GROQ_MODEL || "GLM-5", Number(process.env.PET_GROQ_TIMEOUT || 60000), 1, undefined, process.env.PET_GROQ_BASE_URL);
const result = await client.analyzeUserMessage(prompt, []);
if (!result.summary || result.summary === prompt) {
  throw new Error(`Tamagotchi fell back instead of parsing model JSON: ${JSON.stringify(result)}`);
}
console.log("PASS tamagotchi_real_model:", JSON.stringify(result));'
  )
}

probe_upstream
start_gateway_if_needed
echo "Gateway: ${BASE_URL}"
assert_openai_json_response "${BASE_URL}" "${TOKEN}" "gateway_real_upstream_json"
probe_tamagotchi

if [[ -f "${RUN_LOG}" ]]; then
  echo "Run log: ${RUN_LOG}"
  tail -n 8 "${RUN_LOG}"
fi
