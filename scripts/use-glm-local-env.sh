#!/usr/bin/env bash
set -euo pipefail

cat <<'EOF'
export UPSTREAM_ADAPTERS_JSON='[
  {
    "name":"glm-5",
    "kind":"openai",
    "base_url":"http://0.0.0.0:5025",
    "api_key":"free",
    "model":"GLM-5",
    "force_stream":true,
    "stream_options":{"include_usage":true}
  },
  {
    "name":"glm-47",
    "kind":"openai",
    "base_url":"http://0.0.0.0:5022",
    "api_key":"free",
    "model":"GLM-4.7",
    "force_stream":true,
    "stream_options":{"include_usage":true}
  }
]'
export UPSTREAM_DEFAULT_ROUTE='glm-5,glm-47'
export UPSTREAM_MODEL_ROUTES_JSON='{
  "GLM-5":["glm-5","glm-47"],
  "GLM-4.7":["glm-47","glm-5"],
  "*":["glm-5","glm-47"]
}'
export SCHEDULER_FAILURE_THRESHOLD='3'
export SCHEDULER_COOLDOWN='30s'
export SCHEDULER_STRICT_PROBE_GATE='false'
export PARALLEL_CANDIDATES='2'
export ENABLE_RESPONSE_JUDGE='true'
export JUDGE_MODE='heuristic'
export PROBE_ENABLED='true'
export PROBE_INTERVAL='45s'
export PROBE_TIMEOUT='8s'
export PROBE_STREAM_SMOKE='true'
export PROBE_TOOL_SMOKE='true'
export PROBE_MODELS_JSON='{
  "glm-5":["GLM-5"],
  "glm-47":["GLM-4.7"]
}'
EOF
