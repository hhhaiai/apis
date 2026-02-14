# Script Adapter Base

`kind=script` 适配器用于接入任意 API（本地服务、第三方服务、自定义脚本），并统一转为网关标准响应。

## 1. 配置字段

在 `UPSTREAM_ADAPTERS_JSON` 中增加：

```json
{
  "name": "local-bridge",
  "kind": "script",
  "command": "python3",
  "args": ["scripts/script-adapters/python_openai_bridge.py"],
  "env": {
    "OPENAI_BASE_URL": "http://127.0.0.1:8000",
    "OPENAI_API_KEY": "sk-local",
    "OPENAI_MODEL": "qwen-max"
  },
  "model": "qwen-max",
  "timeout_ms": 120000,
  "max_output_bytes": 4194304
}
```

字段说明：

- `command`: 可执行命令（如 `python3`、`go`、`bash`）。
- `args`: 参数数组。
- `env`: 仅对该脚本生效的环境变量。
- `work_dir`: 脚本工作目录。
- `timeout_ms`: 单次调用超时。
- `max_output_bytes`: 输出上限，防止脚本异常刷屏。

## 2. 输入协议

网关通过标准输入传入：

```json
{
  "version": "ccgateway.script_adapter.v1",
  "mode": "complete|stream",
  "request": {
    "run_id": "xxx",
    "model": "xxx",
    "max_tokens": 1024,
    "system": "...",
    "messages": [],
    "tools": [],
    "metadata": {},
    "headers": {}
  }
}
```

## 3. 输出协议

`complete` 输出一个完整对象：

```json
{
  "model": "xxx",
  "blocks": [{"type":"text","text":"hello"}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 10, "output_tokens": 20}
}
```

`stream` 支持两种：

1. 输出 NDJSON 事件（`message_start` / `content_block_delta` / `message_stop` 等）。
2. 输出 `{"response": ...}` 最终响应；网关会自动合成流式事件。

## 4. 示例脚本

参考：

- `docs/SCRIPT_ADAPTER_EXAMPLES.md`
- `scripts/script-adapters/curl_passthrough_adapter.sh`
- `scripts/script-adapters/python_openai_bridge.py`
- `scripts/script-adapters/go_openai_bridge.go`
- `scripts/use-script-adapter-env.sh`
