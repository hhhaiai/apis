#!/usr/bin/env bash
set -euo pipefail

# Aggregate verification for the local agent gateway work.
#
# This runs the non-negotiable local gates and reports the remaining real-model
# gate explicitly instead of hiding it behind green mock tests.
#
# Usage:
#   bash scripts/verify_agent_gateway.sh
#   CCGW_VERIFY_REQUIRE_REAL=true bash scripts/verify_agent_gateway.sh

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAMAGOTCHI_DIR="${TAMAGOTCHI_DIR:-/Users/sanbo/code/claude-code-tamagotchi}"
REQUIRE_REAL="${CCGW_VERIFY_REQUIRE_REAL:-false}"
JSON_UPSTREAM_PORT="${CCGW_VERIFY_JSON_UPSTREAM_PORT:-$((19600 + RANDOM % 300))}"
JSON_UPSTREAM_PID=""
REAL_GATE_PASSED=false

cleanup() {
  stop_json_upstream
}

stop_json_upstream() {
  if [[ -n "${JSON_UPSTREAM_PID}" ]]; then
    kill "${JSON_UPSTREAM_PID}" >/dev/null 2>&1 || true
    wait "${JSON_UPSTREAM_PID}" >/dev/null 2>&1 || true
    JSON_UPSTREAM_PID=""
  fi
}
trap cleanup EXIT

section() {
  printf '\n== %s ==\n' "$1"
}

run() {
  echo "+ $*"
  "$@"
}

start_json_upstream() {
  python3 "${ROOT_DIR}/scripts/json_openai_mock.py" --port "${JSON_UPSTREAM_PORT}" \
    >/tmp/ccgw-verify-json-upstream-${JSON_UPSTREAM_PORT}.stdout 2>&1 &
  JSON_UPSTREAM_PID="$!"
  for _ in {1..50}; do
    if curl -fsS "http://127.0.0.1:${JSON_UPSTREAM_PORT}/healthz" >/dev/null 2>&1; then
      echo "JSON test upstream: http://127.0.0.1:${JSON_UPSTREAM_PORT}"
      return
    fi
    sleep 0.2
  done
  cat "/tmp/ccgw-verify-json-upstream-${JSON_UPSTREAM_PORT}.stdout" >&2 || true
  echo "failed to start JSON test upstream" >&2
  exit 1
}

section "apis static and unit gates"
cd "${ROOT_DIR}"
run bash -n scripts/smoke_agent_clients.sh scripts/smoke_real_models.sh scripts/verify_agent_gateway.sh
run python3 -B -m py_compile scripts/json_openai_mock.py scripts/discover_openai_upstreams.py
run go test ./...

section "tamagotchi type and build gates"
if [[ ! -d "${TAMAGOTCHI_DIR}" ]]; then
  echo "FAIL: TAMAGOTCHI_DIR not found: ${TAMAGOTCHI_DIR}" >&2
  exit 1
fi
(
  cd "${TAMAGOTCHI_DIR}"
  run bunx tsc --noEmit
  run bun run build
)

section "agent client compatibility smoke"
run bash "${ROOT_DIR}/scripts/smoke_agent_clients.sh"

section "JSON contract smoke"
run env CCGW_SMOKE_JSON_UPSTREAM=true bash "${ROOT_DIR}/scripts/smoke_agent_clients.sh"

section "strict model-quality smoke script plumbing"
start_json_upstream
run env \
  CCGW_REAL_SMOKE_UPSTREAM_URL="http://127.0.0.1:${JSON_UPSTREAM_PORT}" \
  CCGW_REAL_SMOKE_UPSTREAM_API_KEY=free \
  CCGW_REAL_SMOKE_MODEL=x \
  bash "${ROOT_DIR}/scripts/smoke_real_models.sh"
stop_json_upstream

section "actual real-model gate"
if CCGW_REAL_SMOKE_PRINT_DISCOVERY=false bash "${ROOT_DIR}/scripts/smoke_real_models.sh"; then
  echo "PASS real_model_gate"
  REAL_GATE_PASSED=true
else
  status=$?
  echo
  echo "Discovering local OpenAI-compatible upstream candidates..."
  python3 "${ROOT_DIR}/scripts/discover_openai_upstreams.py" --timeout 1 || true
  if [[ "${REQUIRE_REAL}" == "true" ]]; then
    echo "FAIL real_model_gate is required and did not pass" >&2
    exit "${status}"
  fi
  echo "PENDING real_model_gate: real local model endpoint is not ready; rerun with CCGW_VERIFY_REQUIRE_REAL=true when it must be enforced."
fi

section "summary"
echo "PASS local gateway, agent clients, JSON contract, Tamagotchi type/build, and strict smoke plumbing."
if [[ "${REAL_GATE_PASSED}" == "true" ]]; then
  echo "PASS actual real-model gate."
else
  echo "PENDING actual real-model quality gate unless scripts/smoke_real_models.sh passed above."
fi
