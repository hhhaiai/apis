# Script Adapter Examples

这个文档对应 `scripts/script-adapters/`，用于把任意 API 包装成网关可识别的标准输出。

## 协议输入

网关通过 `stdin` 输入 JSON：

```json
{
  "version": "ccgateway.script_adapter.v1",
  "mode": "complete|stream",
  "request": {
    "run_id": "optional",
    "model": "requested-model",
    "max_tokens": 1024,
    "system": "...",
    "messages": [],
    "tools": [],
    "metadata": {},
    "headers": {}
  }
}
```

## 协议输出

`complete` 模式输出单个 JSON：

```json
{
  "model": "your-model",
  "blocks": [
    {"type":"text","text":"hello"}
  ],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 10, "output_tokens": 20}
}
```

`stream` 模式可以输出 NDJSON 事件，也可以输出单行最终响应：

```json
{"response":{"model":"m","text":"hello","stop_reason":"end_turn"}}
```

如果输出的是最终响应，网关会自动合成为标准流式事件。

## 示例脚本

- `scripts/script-adapters/curl_passthrough_adapter.sh`
- `scripts/script-adapters/python_openai_bridge.py`
- `scripts/script-adapters/go_openai_bridge.go`
- `scripts/use-script-adapter-env.sh`

## 快速接入（Python 桥接）

```bash
export OPENAI_BASE_URL="http://127.0.0.1:8000/v1"
export OPENAI_API_KEY="sk-local"
export OPENAI_MODEL="your-model-name"
python3 scripts/script-adapters/python_openai_bridge.py <<'JSON'
{"version":"ccgateway.script_adapter.v1","mode":"complete","request":{"model":"x","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}}
JSON
```
