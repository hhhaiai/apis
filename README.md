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
      {"name":"glm-5","kind":"openai","base_url":"http://127.0.0.1:5025","api_key":"free","model":"GLM-5","supports_vision":false,"force_stream":true},
      {"name":"glm-47","kind":"openai","base_url":"http://127.0.0.1:5022","api_key":"free","model":"GLM-4.7","supports_vision":true,"force_stream":true}
    ],
    "default_route":["glm-5","glm-47"],
    "model_routes":{"GLM-5":["glm-5"],"GLM-4.7":["glm-47"],"*":["glm-5","glm-47"]}
  }'
```

### 5) 任意 API 接入（Script Adapter）

- `kind=script` 支持 `curl/python/go` 自定义桥接。
- 入口：`docs/SCRIPT_ADAPTER_BASE.md` 与 `docs/SCRIPT_ADAPTER_EXAMPLES.md`。
- 示例脚本：`scripts/script-adapters/`。

### 6) 插件市场（一键安装常用集成）

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
