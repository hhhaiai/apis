# CC Gateway

统一 LLM API 网关，兼容 Claude Code / Anthropic Messages、OpenAI Chat Completions/Responses，并支持运行时路由、模型映射、Script Adapter、MCP/Plugins/Plans/Todos 等能力。


## 文档入口

- 项目完整文档：`docs/PROJECT_FULL_GUIDE.md`
- 项目结构与运行梳理：`docs/PROJECT_OVERVIEW.md`
- 后台功能清单：`docs/ADMIN_CONSOLE_FEATURES.md`
- 转换进度：`docs/CC_CONVERSION_PROGRESS.md`
- 收尾计划：`docs/CC_CONVERSION_COMPLETION_PLAN.md`
- Script Adapter 基座：`docs/SCRIPT_ADAPTER_BASE.md`
- Script Adapter 示例：`docs/SCRIPT_ADAPTER_EXAMPLES.md`
- 插件市场使用指南：`docs/MARKETPLACE_GUIDE.md`
- 插件市场 API 文档：`docs/MARKETPLACE_API.md`
- Claude Code / Codex / Tamagotchi 兼容性验证：`docs/AGENT_CLIENT_COMPATIBILITY.md`

## 一键运行

- 一键编译并前台运行：

```bash
bash scripts/one_click.sh
```

- 一键编译并后台运行：

```bash
bash scripts/build_run_gateway.sh --detached
```

- 停止后台进程：

```bash
bash scripts/build_run_gateway.sh --stop
```

等价完整脚本（可选参数更多）：

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
  -H 'x-api-key: admin123456' \
  -H 'anthropic-version: 2023-06-01' \
  -H 'content-type: application/json' \
  --data-raw '{
    "model":"GLM-5",
    "max_tokens":128,
    "stream":true,
    "messages":[{"role":"user","content":"hi"}]
  }'
```

也可以使用 `Authorization: Bearer <token>`；Claude Code 的 `ANTHROPIC_API_KEY` 默认会走 `x-api-key`，因此网关已兼容该头。

### 3) Claude Code 直连本地网关

```bash
ANTHROPIC_BASE_URL=http://127.0.0.1:18080 \
ANTHROPIC_API_KEY=admin123456 \
claude --bare --model GLM-5 -p "你好，确认你已经通过本地网关响应"
```

说明：
- `ANTHROPIC_BASE_URL` 指向本网关根地址，不需要追加 `/v1`。
- `ANTHROPIC_API_KEY` 会以 `x-api-key` 发送；网关同时兼容 `Authorization: Bearer`。
- 生产环境请把 `ADMIN_TOKEN` 改成自定义值，或使用后台生成的用户 token。

### 4) Codex 直连本地网关

Codex 使用 OpenAI Responses 协议，base URL 需要包含 `/v1`。建议先用临时 `CODEX_HOME` 验证，不污染自己的全局配置：

```bash
mkdir -p /tmp/ccgw-codex-home
cat >/tmp/ccgw-codex-home/config.toml <<'EOF'
model = "GLM-5"
model_provider = "ccgateway"

[model_providers.ccgateway]
name = "CC Gateway"
base_url = "http://127.0.0.1:18080/v1"
wire_api = "responses"
env_key = "CCGATEWAY_API_KEY"
requires_openai_auth = false
EOF

NO_PROXY=127.0.0.1,localhost \
no_proxy=127.0.0.1,localhost \
CODEX_HOME=/tmp/ccgw-codex-home \
CCGATEWAY_API_KEY=admin123456 \
codex exec --skip-git-repo-check "你好，确认你已经通过本地网关响应"
```

说明：
- Codex 会发送 Responses API、namespace tools、web_search service tool 等字段；网关会把可转发的 function/namespace tools 转为上游工具，无法本地执行的 `web_search` service tool 会被安全忽略。
- 如果本机配置了 HTTP 代理，务必设置 `NO_PROXY=127.0.0.1,localhost`，否则 Codex 可能把本地网关请求发到代理。

### 5) Claude Code Tamagotchi 直连本地网关

`/Users/sanbo/code/claude-code-tamagotchi` 的 AI feedback 可以走本网关的 OpenAI-compatible `/v1/chat/completions`：

```bash
export PET_FEEDBACK_ENABLED=true
export PET_GROQ_API_KEY=admin123456
export PET_GROQ_BASE_URL=http://127.0.0.1:18080/v1
export PET_GROQ_MODEL=GLM-5
export PET_GROQ_TIMEOUT=30000
```

说明：
- `PET_GROQ_API_KEY` 对应本网关的 `ADMIN_TOKEN` 或用户 token。
- `PET_GROQ_BASE_URL` 要包含 `/v1`，Tamagotchi 会请求 `${PET_GROQ_BASE_URL}/chat/completions`。
- 本地模型响应较慢时，建议把 `PET_GROQ_TIMEOUT` 调大到 `30000` 或更高。

### 6) 一键验证 Claude Code / Codex / Tamagotchi 兼容性

```bash
bash scripts/smoke_agent_clients.sh
```

默认会临时启动本网关 mock upstream，并验证：
- `/v1/models`：OpenAI-compatible 模型枚举
- `/v1/models/{model}`：OpenAI-compatible 模型详情
- `/v1/messages`：Anthropic / Claude Code 协议
- `/v1/responses`：Codex Responses 协议
- `/openai/v1/models`：Groq SDK 固定路径模型枚举 alias
- `/openai/v1/models/{model}`：Groq SDK 固定路径模型详情 alias
- `/openai/v1/chat/completions`：Groq SDK 固定路径兼容 alias
- 如本机存在 `/Users/sanbo/code/claude-code-source/cli.js`、`codex`、`/Users/sanbo/code/claude-code-tamagotchi` 且依赖已安装，会继续跑真实客户端 smoke；如果要测试源码仓库自己编译的 Codex，可设置 `CODEX_CLI=/Users/sanbo/code/codex/.../codex`。

测试已有运行中的网关：

```bash
CCGW_SMOKE_BASE_URL=http://127.0.0.1:18080 \
CCGW_SMOKE_TOKEN=admin123456 \
CCGW_SMOKE_MODEL=GLM-5 \
bash scripts/smoke_agent_clients.sh
```

如果本机真实国产模型还没启动，也可以先用 deterministic JSON upstream 验证 Tamagotchi 的 AI observation 不只是 fallback：

```bash
CCGW_SMOKE_JSON_UPSTREAM=true bash scripts/smoke_agent_clients.sh
```

这会额外启动 `scripts/json_openai_mock.py`，让 Tamagotchi 收到可解析 JSON，并断言解析出的 summary 为 `local gateway json smoke`。

真实国产模型启动后，跑严格质量门禁：

```bash
bash scripts/smoke_real_models.sh
```

默认先检查 `http://127.0.0.1:5025` 的 `GLM-5`；如果没有显式设置 `CCGW_REAL_SMOKE_UPSTREAM_URL` 且默认端口不通，脚本会自动扫描 localhost 上的 OpenAI-compatible endpoint，找到 ready 服务后重试。也可用 `CCGW_REAL_SMOKE_UPSTREAM_URL` / `CCGW_REAL_SMOKE_MODEL` 固定覆盖；如真实服务使用非默认路径，可设置 `CCGW_REAL_SMOKE_UPSTREAM_ENDPOINT=/openai/v1/chat/completions`；如需禁止自动发现，设置 `CCGW_REAL_SMOKE_AUTO_DISCOVER=false`。如果候选端口返回 `REQUIRES_AUTH`，把本地模型 key 放进 `CCGW_REAL_SMOKE_DISCOVER_KEYS` 或直接设置 `CCGW_REAL_SMOKE_UPSTREAM_API_KEY` 后重跑。

完整验收汇总：

```bash
bash scripts/verify_agent_gateway.sh
```

该脚本会跑 Go 测试、Claude Code/Codex/Groq SDK/Tamagotchi smoke、JSON 契约 smoke、Tamagotchi `tsc/build`，并把真实国产模型门禁显示为 `PASS` 或 `PENDING`。如果要求真实模型必须通过：

```bash
CCGW_VERIFY_REQUIRE_REAL=true bash scripts/verify_agent_gateway.sh
```

如果不确定真实模型在哪个本地端口，先自动发现：

```bash
python3 scripts/discover_openai_upstreams.py --keys free admin123456 <你的本地key>
```

也可以不在命令行暴露 key，改用环境变量：

```bash
CCGW_REAL_SMOKE_DISCOVER_KEYS='<你的本地key>' \
python3 scripts/discover_openai_upstreams.py
```

如果模型名不是默认的 `GLM-5`，可以显式传入候选模型；脚本也会自动读取可用服务的 `/v1/models` 并尝试返回的模型 ID。没有 ready endpoint 但发现疑似服务时，它会列出需要认证的 URL、endpoint，并给出下一条重跑命令：

```bash
python3 scripts/discover_openai_upstreams.py \
  --keys free admin123456 <你的本地key> \
  --models GLM-5 GLM-4.7 qwen-max deepseek-chat
```

### 7) 后台运行时模型映射（无需重启）

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

### 8) 后台运行时上游接入（Script/HTTP）

```bash
curl 'http://127.0.0.1:18080/admin/upstream' \
  -H 'authorization: Bearer secret-admin' \
  -H 'content-type: application/json' \
  -X PUT \
  --data-raw '{
    "adapters":[
      {"name":"glm-5","kind":"openai","base_url":"http://127.0.0.1:5025","api_key":"free","model":"GLM-5","supports_vision":false,"force_stream":true},
      {"name":"glm-47","kind":"openai","base_url":"http://127.0.0.1:5022","api_key":"free","model":"GLM-4.7","supports_vision":true,"force_stream":true}
    ],
    "default_route":["glm-5","glm-47"],
    "model_routes":{"GLM-5":["glm-5"],"GLM-4.7":["glm-47"],"*":["glm-5","glm-47"]}
  }'
```

### 9) 任意 API 接入（Script Adapter）

- `kind=script` 支持 `curl/python/go` 自定义桥接。
- 入口：`docs/SCRIPT_ADAPTER_BASE.md` 与 `docs/SCRIPT_ADAPTER_EXAMPLES.md`。
- 示例脚本：`scripts/script-adapters/`。

### 10) 插件市场（一键安装常用集成）

浏览可用插件：

```bash
curl http://127.0.0.1:8080/v1/cc/marketplace/plugins
```

安装插件（如 GLM Local）：

```bash
curl -X POST http://127.0.0.1:8080/v1/cc/marketplace/plugins/glm-local/install \
  -H 'content-type: application/json' \
  --data-raw '{
    "config": {
      "glm5_url": "http://127.0.0.1:5025",
      "glm47_url": "http://127.0.0.1:5022"
    }
  }'
```

预置插件包：
- `glm-local`：本地 GLM 模型集成
- `openai-proxy`：OpenAI API 代理
- `anthropic-proxy`：Anthropic Claude API 代理
- `search-tools`：网络搜索工具包
- `file-tools`：文件操作工具包

详见：`docs/MARKETPLACE_GUIDE.md`

## 后台接口清单（核心）

- `GET/PUT /admin/settings`
- `GET/PUT /admin/model-mapping`
- `GET/PUT /admin/upstream`
- `GET /admin/capabilities`（模型/渠道能力矩阵与 fallback 诊断）
- `GET/PUT /admin/tools`（支持 `scope=project|global` 与 `project_id`）
- `GET /admin/tools/gaps`（聚合 `tool.gap_detected` 缺口统计）
- `GET/PUT/POST /admin/intelligent-dispatch`
- `GET/PUT /admin/scheduler`
- `GET/PUT /admin/probe`
- `GET /admin/auth/status`
- `GET/POST /admin/auth/users`
- `GET/PUT/DELETE /admin/auth/users/{user_id}`
- `GET/POST /admin/auth/users/{user_id}/tokens`
- `GET/PUT/DELETE /admin/auth/users/{user_id}/tokens/{token_id}`
- `GET/POST /admin/auth/users/{user_id}/quota`
- `GET/POST /admin/channels`
- `GET/PUT/DELETE /admin/channels/{id}`
- `PUT /admin/channels/{id}/status`
- `POST /admin/channels/{id}/test`
- `GET /admin/status`
- `GET/POST /admin/bootstrap/apply`（配置模板/一键导入 tools+plugins+mcp+upstream）
- `POST /admin/marketplace/cloud/list`（按云端 URL 拉取插件清单）
- `POST /admin/marketplace/cloud/install`（云端清单多选安装，支持作用域）

作用域与项目隔离（推荐）：
- Header: `x-project-id: <project>`（默认 `default`）
- Query: `scope=project|global` + `project_id=<project>`
- `plugins / mcp / tools` 默认按项目隔离；`scope=global` 可用于全局配置

## 插件市场接口

- `GET /v1/cc/marketplace/plugins` - 列出所有可用插件
- `GET /v1/cc/marketplace/plugins/{name}` - 获取插件详情
- `POST /v1/cc/marketplace/plugins/{name}/install` - 安装插件（支持 `scope=project|global` + `project_id`）
- `GET /v1/cc/marketplace/search` - 搜索插件
- `GET /v1/cc/marketplace/updates` - 检查更新
- `GET /v1/cc/marketplace/recommendations` - 获取推荐插件

## 鉴权与配额要点

- `/v1/messages`、`/v1/chat/completions`、`/v1/responses` 与全部 `/v1/cc/*` 默认都需要鉴权。
- 管理员可使用 `ADMIN_TOKEN`；业务调用建议使用用户 token（支持配额、模型/IP 限制）。
- 请求会基于实际 usage 进行额度结算，管理员 token 不走用户配额扣减。

## 不支持字段与解码失败诊断

- 严格解码接口（主要是后台与 CC 管理接口）遇到未知字段会记录诊断事件。
- 事件类型：`request.unsupported_fields`、`request.decode_failed`。
- 后台 Events 可直接查看：
  - 不支持字段列表
  - 请求参数（原始 body）
  - 脱敏后的复现 curl 命令

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

### Tool 仿真配置（上游不支持原生 tool call 时）

`runtime settings` 中可设置：

```json
{
  "vision_support_hints": {
    "gpt-3.5-*": false,
    "gpt-4o": true
  },
  "tool_loop": {
    "mode": "server_loop",
    "max_steps": 4,
    "emulation_mode": "hybrid",
    "planner_model": "planner-model"
  }
}
```

- `mode`: `client_loop | server_loop | native | react | json | hybrid`
- `emulation_mode`: `native | react | json | hybrid`
- `planner_model`: 可选，指定工具规划模型；工具执行后会回到原请求模型生成最终答复
- `vision_support_hints`: 按模型名/通配符声明图形能力（`true/false`），用于“图片识别后文本透传”自动降级

请求级覆盖（`metadata`）：
- `upstream_supports_vision: true|false`
- `vision_fallback_mode: off|auto|force`

上游渠道级能力声明（`UPSTREAM_ADAPTERS_JSON` / `/admin/upstream`）：
- `supports_vision: true|false`
