# CC Gateway

统一 LLM API 网关，兼容 Claude Code / Anthropic Messages、OpenAI Chat Completions/Responses，并支持运行时路由、模型映射、Script Adapter、MCP/Plugins/Plans/Todos 等能力。

## 文档入口

- 项目完整文档：`docs/PROJECT_FULL_GUIDE.md`
- 项目结构与运行梳理：`docs/PROJECT_OVERVIEW.md`
- 后台功能清单：`docs/ADMIN_CONSOLE_FEATURES.md`
- Script Adapter 基座：`docs/SCRIPT_ADAPTER_BASE.md`
- Script Adapter 示例：`docs/SCRIPT_ADAPTER_EXAMPLES.md`

## 一键运行

```bash
bash scripts/build_run_gateway.sh
```

默认端口 `8080`，启动后访问：

- `http://127.0.0.1:8080/`
- `http://127.0.0.1:8080/admin/`
- `http://127.0.0.1:8080/healthz`

默认后台口令为 `ADMIN_TOKEN=admin123456`，生产环境请务必修改。

## 最新完整用法

### 1) 本地 GLM 双路由（GLM-5 + GLM-4.7）

```bash
source <(bash scripts/use-glm-local-env.sh)
bash scripts/build_run_gateway.sh --no-ui --port 18080
```

### 2) Anthropic 协议调用（CC 直连网关）

```bash
curl -N 'http://127.0.0.1:18080/v1/messages' \
  -H 'anthropic-version: 2023-06-01' \
  -H 'content-type: application/json' \
  --data-raw '{
    "model":"GLM-5",
    "max_tokens":128,
    "stream":true,
    "messages":[{"role":"user","content":"hi"}]
  }'
```

### 3) 后台运行时模型映射（无需重启）

```bash
curl 'http://127.0.0.1:18080/admin/model-mapping' \
  -H 'authorization: Bearer secret-admin' \
  -H 'content-type: application/json' \
  -X PUT \
  --data-raw '{
    "model_mappings":{"claude-3-7-sonnet":"GLM-5","gpt-4o":"GLM-4.7"},
    "model_map_strict":true,
    "model_map_fallback":"GLM-5"
  }'
```

### 4) 后台运行时上游接入（Script/HTTP）

```bash
curl 'http://127.0.0.1:18080/admin/upstream' \
  -H 'authorization: Bearer secret-admin' \
  -H 'content-type: application/json' \
  -X PUT \
  --data-raw '{
    "adapters":[
      {"name":"glm-5","kind":"openai","base_url":"http://127.0.0.1:5025","api_key":"free","model":"GLM-5","force_stream":true},
      {"name":"glm-47","kind":"openai","base_url":"http://127.0.0.1:5022","api_key":"free","model":"GLM-4.7","force_stream":true}
    ],
    "default_route":["glm-5","glm-47"],
    "model_routes":{"GLM-5":["glm-5"],"GLM-4.7":["glm-47"],"*":["glm-5","glm-47"]}
  }'
```

### 5) 任意 API 接入（Script Adapter）

- `kind=script` 支持 `curl/python/go` 自定义桥接。
- 入口：`docs/SCRIPT_ADAPTER_BASE.md` 与 `docs/SCRIPT_ADAPTER_EXAMPLES.md`。
- 示例脚本：`scripts/script-adapters/`。

## 后台接口清单（核心）

- `GET/PUT /admin/settings`
- `GET/PUT /admin/model-mapping`
- `GET/PUT /admin/upstream`
- `GET/PUT /admin/tools`
- `GET/PUT /admin/scheduler`
- `GET/PUT /admin/probe`
- `GET /admin/status`

## 测试规范

- 所有测试文件统一在 `tests/` 目录。
- 运行：

```bash
go test ./...
```

## 常用配置样例

- `configs/runtime-settings.example.json`
- `configs/upstream-glm-local.example.json`
- `configs/probe-models.example.json`
- `configs/tool-catalog.example.json`
