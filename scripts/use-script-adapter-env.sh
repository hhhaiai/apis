#!/usr/bin/env bash
set -euo pipefail

cat <<'EOF'
# python bridge target (replace with your local upstream)
export OPENAI_BASE_URL='http://127.0.0.1:8000'
export OPENAI_API_KEY='sk-local'
export OPENAI_MODEL='qwen-max'

export UPSTREAM_ADAPTERS_JSON='[
  {
    "name":"local-script-bridge",
    "kind":"script",
    "command":"python3",
    "args":["scripts/script-adapters/python_openai_bridge.py"],
    "model":"qwen-max",
    "timeout_ms":120000
  }
]'

export UPSTREAM_DEFAULT_ROUTE='local-script-bridge'
export UPSTREAM_MODEL_ROUTES_JSON='{
  "*":["local-script-bridge"]
}'
EOF
