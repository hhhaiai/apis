# Agent Client Compatibility Runbook

This gateway is intended to be a local bridge for Claude Code, Codex, Groq/OpenAI-compatible clients, and Claude Code Tamagotchi.

## Local smoke

Run the aggregate verification:

```bash
bash scripts/verify_agent_gateway.sh
```

This runs:

- gateway unit tests,
- script syntax checks,
- JSON mock compilation,
- Claude Code/Codex/Groq SDK/Tamagotchi smoke,
- deterministic JSON upstream smoke,
- Tamagotchi `bunx tsc --noEmit` and `bun run build`,
- strict real-model smoke plumbing against the deterministic JSON upstream,
- and finally the actual real-model gate as `PASS` or `PENDING`.

To make the actual real-model gate mandatory:

```bash
CCGW_VERIFY_REQUIRE_REAL=true bash scripts/verify_agent_gateway.sh
```

Run the full compatibility smoke with the built-in mock upstream:

```bash
bash scripts/smoke_agent_clients.sh
```

To test a Codex binary built from `/Users/sanbo/code/codex`, point the smoke at
that binary explicitly:

```bash
CODEX_CLI=/Users/sanbo/code/codex/.../codex bash scripts/smoke_agent_clients.sh
```

Expected proof lines:

- `PASS anthropic_messages`
- `PASS openai_models`
- `PASS groq_sdk_models_alias`
- `PASS openai_model_retrieve`
- `PASS groq_sdk_model_retrieve_alias`
- `PASS openai_chat`
- `PASS groq_sdk_alias`
- `PASS responses`
- `PASS claude_code_cli`
- `PASS codex_cli`
- `PASS groq_sdk_client`
- `PASS tamagotchi_client`

## JSON upstream smoke for Tamagotchi

Tamagotchi expects JSON for its AI observation parser. If real local models are not running yet, use the deterministic JSON upstream:

```bash
CCGW_SMOKE_JSON_UPSTREAM=true bash scripts/smoke_agent_clients.sh
```

This starts `scripts/json_openai_mock.py`, routes the gateway to it, and asserts that Tamagotchi parses:

```json
{"summary":"local gateway json smoke","intent":"agent smoke"}
```

This proves the local gateway request path is not only returning HTTP 200, but also supports the JSON-shaped response Tamagotchi needs.

The gateway also preserves OpenAI/Groq JSON controls for real upstreams:

- `max_completion_tokens` is accepted as an alias for `max_tokens`.
- `response_format` is stored in canonical metadata.
- OpenAI upstream adapters forward `response_format` to the selected model endpoint.

Tamagotchi additionally tolerates imperfect local-model output by extracting JSON from direct JSON, markdown fenced JSON, or the first balanced JSON object in prose.

## Existing gateway smoke

To test a running gateway:

```bash
CCGW_SMOKE_BASE_URL=http://127.0.0.1:18080 \
CCGW_SMOKE_TOKEN=admin123456 \
CCGW_SMOKE_MODEL=GLM-5 \
bash scripts/smoke_agent_clients.sh
```

## Real local model gate

The GLM local profile expects OpenAI-compatible upstreams at:

- `http://127.0.0.1:5025/v1/chat/completions` for `GLM-5`
- `http://127.0.0.1:5022/v1/chat/completions` for `GLM-4.7`

Check availability:

```bash
curl -sS http://127.0.0.1:5025/v1/models
curl -sS http://127.0.0.1:5022/v1/models
```

When those are live:

```bash
source <(bash scripts/use-glm-local-env.sh)
bash scripts/build_run_gateway.sh --no-ui --port 18080
CCGW_SMOKE_BASE_URL=http://127.0.0.1:18080 \
CCGW_SMOKE_TOKEN=admin123456 \
CCGW_SMOKE_MODEL=GLM-5 \
bash scripts/smoke_agent_clients.sh
```

For the strict real-model quality gate, run:

```bash
bash scripts/smoke_real_models.sh
```

Defaults:

- upstream URL: `http://127.0.0.1:5025`
- upstream endpoint: `/v1/chat/completions`
- model: `GLM-5`
- upstream API key: `free`

Override as needed:

```bash
CCGW_REAL_SMOKE_UPSTREAM_URL=http://127.0.0.1:5025 \
CCGW_REAL_SMOKE_UPSTREAM_API_KEY=free \
CCGW_REAL_SMOKE_MODEL=GLM-5 \
bash scripts/smoke_real_models.sh
```

For OpenAI-compatible services that expose the Groq-style alias instead of the
default path, also set:

```bash
CCGW_REAL_SMOKE_UPSTREAM_ENDPOINT=/openai/v1/chat/completions
```

This gate fails with `REAL_MODEL_NOT_READY` until the real model service answers
`/v1/chat/completions` with parseable JSON under `response_format=json_object`.
When `CCGW_REAL_SMOKE_UPSTREAM_URL` is not explicitly set, the script first tries
the default `http://127.0.0.1:5025`; if that fails, it auto-runs localhost
discovery and retries the first ready OpenAI-compatible endpoint. Disable that
fallback with `CCGW_REAL_SMOKE_AUTO_DISCOVER=false` when you want a strict fixed
URL check. Direct `smoke_real_models.sh` runs print the localhost discovery
report on failure by default; set `CCGW_REAL_SMOKE_PRINT_DISCOVERY=false` when a
caller already prints its own discovery report.

If the real model service may be running on a different localhost port, or the
model ID is not exactly `GLM-5`, run discovery first:

```bash
python3 scripts/discover_openai_upstreams.py --keys free admin123456 <your-local-key>
```

To avoid putting a local key in shell history, use env-discovery instead:

```bash
CCGW_REAL_SMOKE_DISCOVER_KEYS='<your-local-key>' \
python3 scripts/discover_openai_upstreams.py
```

Discovery only probes localhost. It tries common ports plus currently listening
ports, sends both `Authorization: Bearer` and `x-api-key`, checks `/v1/models`
and `/openai/v1/models`, then tries `/v1/chat/completions` and
`/openai/v1/chat/completions` with the requested model plus discovered model IDs.
When it finds a ready endpoint, it prints the exact `smoke_real_models.sh`
command to run. If it only finds `REQUIRES_AUTH` endpoints, it prints the exact
URL/endpoint list and a rerun template for adding the correct key.

If you already know likely model names, pass them explicitly:

```bash
python3 scripts/discover_openai_upstreams.py \
  --keys free admin123456 <your-local-key> \
  --models GLM-5 GLM-4.7 qwen-max deepseek-chat
```

## Compatibility paths

| Client | Gateway path | Notes |
|---|---|---|
| Claude Code / Anthropic SDK | `/v1/messages` | accepts `x-api-key`, `Authorization: Bearer`, or `?token=` |
| Codex | `/v1/responses` | Responses streaming lifecycle is emitted for Codex, including custom/local-shell tool history mapping and `response.failed` + `[DONE]` on upstream stream errors |
| OpenAI-compatible model listing | `/v1/models` | returns configured model IDs from routes, adapters, settings, and channel abilities |
| OpenAI-compatible model retrieve | `/v1/models/{model}` | returns a model object for SDK health checks and model validation |
| OpenAI-compatible clients | `/v1/chat/completions` | standard chat completions path |
| Groq SDK model listing | `/openai/v1/models` | Groq SDK-compatible model-list alias |
| Groq SDK model retrieve | `/openai/v1/models/{model}` | Groq SDK-compatible model-retrieve alias |
| Groq SDK | `/openai/v1/chat/completions` | Groq SDK hard-codes this suffix after `baseURL` |
| Tamagotchi local gateway mode | `/v1/chat/completions` | configured via `PET_GROQ_BASE_URL=http://127.0.0.1:18080/v1` |

## Completion checklist

- `go test ./...` passes in this repo.
- `bunx tsc --noEmit` passes in `/Users/sanbo/code/claude-code-tamagotchi`.
- `bun run build` passes in `/Users/sanbo/code/claude-code-tamagotchi`.
- `bash scripts/smoke_agent_clients.sh` passes.
- `CCGW_SMOKE_JSON_UPSTREAM=true bash scripts/smoke_agent_clients.sh` passes.
- `bash scripts/verify_agent_gateway.sh` passes with only the actual real-model gate marked pending when local model service is unavailable.
- `bash scripts/smoke_real_models.sh` passes against the actual local国产模型 endpoint.
- Real GLM ports are reachable before claiming real国产模型 quality is verified.
