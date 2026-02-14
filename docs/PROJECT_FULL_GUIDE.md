# CC Gateway 项目完整文档（当前实现）

> 更新时间：2026-02-14  
> 代码基线：`/Users/sanbo/Desktop/api`

## 文档说明

当前仓库中没有历史 `docs/*.md` 文件，只有 `README.md`。  
本文件已将现有说明与代码实现合并为一份统一文档，内容以当前代码行为为准。

## 1. 项目定位

CC Gateway 是一个统一 LLM 网关，目标是同时兼容：

- Anthropic Messages API（Claude Code 协议风格）
- OpenAI Chat Completions / Responses API
- CC 扩展生命周期接口（Session / Run / Plan / Todo / Event / MCP）

核心能力是把多后端模型（OpenAI / Anthropic / Gemini / Canonical / Mock）统一成一套入口，并在网关侧提供路由、裁判、探针、反思和工具循环。

## 2. 当前默认启动行为（`cmd/cc-gateway/main.go`）

- 默认端口：`8080`（环境变量 `PORT` 可覆盖）
- 默认后台密码：`admin123456`（未设置 `ADMIN_TOKEN` 时启用，并在日志/后台告警）
- 若未配置 `UPSTREAM_ADAPTERS_JSON`：
  - 自动启用两个 mock 适配器：`mock-primary` / `mock-fallback`
- 自动初始化并接入：
  - 上游路由服务（重试、超时、反思、并行候选、裁判）
  - 调度器健康评分引擎
  - 探针 Runner（按间隔探测）
  - 运行时设置中心
  - 工具目录与动态策略
  - Session / Run / Plan / Todo / Event 内存存储
  - MCP Registry
  - Run 日志（默认 `logs/run-events.log`）
- 可选状态持久化：
  - 设置 `STATE_PERSIST_DIR` 后，Run/Plan/Todo 自动加载 + 自动保存

## 3. 整体架构

请求主链路：

1. `gateway` 层接收协议请求（Anthropic/OpenAI/CC）
2. 解析 `mode`、`session`、模型映射、运行时路由策略
3. `policy` 校验工具权限
4. 转为 `orchestrator.Request` 进入 `upstream.RouterService`
5. `scheduler.Engine` 对候选排序（健康、冷却、探针结果）
6. 可选并行候选与 `judge` 选优（启发式或 LLM 裁判）
7. 可选反思循环（critique/fix）
8. 返回同步响应或 SSE 流
9. 写入 Run/Event/Log（如果对应 store/logger 已配置）

## 4. API 总览

### 4.1 健康检查

- `GET /healthz`

### 4.2 Anthropic 兼容

- `POST /v1/messages`
- `POST /v1/messages/count_tokens`

说明：

- 这两个接口都要求 `anthropic-version` 请求头。
- `stream=true` 时，`/v1/messages` 走 SSE。

### 4.3 OpenAI 兼容

- `POST /v1/chat/completions`
- `POST /v1/responses`

说明：

- 两个接口都支持 `stream=true`（SSE）。
- 请求会先转换到内部 canonical 格式，再走统一路由。

### 4.4 CC 扩展接口

- `GET/POST /v1/cc/sessions`
- `GET /v1/cc/sessions/{id}`
- `POST /v1/cc/sessions/{id}/fork`
- `GET /v1/cc/runs`
- `GET /v1/cc/runs/{id}`
- `GET/POST /v1/cc/todos`
- `GET/PUT /v1/cc/todos/{id}`
- `GET/POST /v1/cc/plans`
- `GET /v1/cc/plans/{id}`
- `POST /v1/cc/plans/{id}/approve`
- `POST /v1/cc/plans/{id}/execute`
- `GET /v1/cc/events`
- `GET /v1/cc/events/stream`（SSE）
- `GET/POST /v1/cc/teams`
- `GET /v1/cc/teams/{id}`
- `GET/POST /v1/cc/teams/{id}/agents`
- `GET/POST /v1/cc/teams/{id}/tasks`
- `POST /v1/cc/teams/{id}/orchestrate`
- `GET/POST /v1/cc/teams/{id}/messages`
- `GET /v1/cc/subagents`
- `GET /v1/cc/subagents/{id}`
- `POST /v1/cc/subagents/{id}/terminate`
- `DELETE /v1/cc/subagents/{id}`
- `GET /v1/cc/subagents/{id}/timeline`
- `GET /v1/cc/subagents/{id}/events`
- `GET /v1/cc/subagents/{id}/stream`（SSE）
- `GET/POST /v1/cc/mcp/servers`
- `GET/PUT/DELETE /v1/cc/mcp/servers/{id}`
- `POST /v1/cc/mcp/servers/{id}/health`
- `POST /v1/cc/mcp/servers/{id}/reconnect`
- `POST /v1/cc/mcp/servers/{id}/tools/list`
- `POST /v1/cc/mcp/servers/{id}/tools/call`
- `GET/POST /v1/cc/plugins`
- `GET/DELETE /v1/cc/plugins/{name}`
- `POST /v1/cc/plugins/{name}/enable`
- `POST /v1/cc/plugins/{name}/disable`

### 4.5 管理接口

- `GET /`（入口文档 + 后台入口）
- `GET/PUT /admin/settings`
- `GET/PUT /admin/tools`
- `GET/PUT /admin/scheduler`
- `GET/PUT /admin/probe`
- `GET /admin/auth/status`
- `GET /admin/status`
- `GET /admin/`（内置 Dashboard）

可选接口（默认主程序未接入依赖，返回 `501`）：

- `GET /admin/cost`
- `GET/POST /v1/cc/skills`
- `GET/DELETE /v1/cc/skills/{name}`
- `POST /v1/cc/skills/{name}/execute`
- `POST /v1/cc/eval`

## 5. 协议与行为细节

- `GET /v1/cc/events` 与 `GET /v1/cc/events/stream` 支持 `team_id`、`subagent_id` 过滤。
- `GET /v1/cc/subagents/{id}/timeline` 与 `GET /v1/cc/subagents/{id}/events` 支持 `limit`、`event_type` 查询参数。
- `GET /v1/cc/subagents/{id}/stream` 支持 `event_type` 查询参数。
- 事件 `data` 会自动补充 `record_text`（文本记录），用于日志审计与多任务同步。
- 子代理生命周期会追加事件：`subagent.created`、`subagent.running`、`subagent.completed`、`subagent.failed`。
- Team 任务执行会追加事件：`team.task.running`、`team.task.completed`、`team.task.failed`。
- 插件管理会追加事件：`plugin.installed`、`plugin.enabled`、`plugin.disabled`、`plugin.uninstalled`。

### 5.1 模式（mode）

来源优先级：

1. 请求头 `x-cc-mode`
2. `metadata.cc_mode`
3. 默认 `chat`

mode 影响：

- `settings.ResolveModel` 的模型覆写
- system prompt 前缀注入
- 路由参数注入（重试、超时、并行、裁判、tool loop、mode route）

### 5.2 会话关联

会话 ID 来源优先级：

1. 请求头 `x-cc-session-id`
2. `metadata.session_id`
3. `metadata.cc_session_id`
4. `metadata.sessionId`

### 5.3 运行追踪头

`/v1/messages`、`/v1/chat/completions`、`/v1/responses` 会返回：

- `request-id`
- `x-cc-run-id`
- `x-cc-mode`
- `x-cc-client-model`
- `x-cc-requested-model`
- `x-cc-upstream-model`

### 5.4 工具循环（Tool Loop）

- 默认模式：`client_loop`
- 服务端循环开启条件：`metadata.tool_loop_mode` 为 `server` 或 `server_loop`
- 最大步数：`metadata.tool_loop_max_steps`（默认 4）

服务端循环逻辑：

1. 模型返回 `tool_use`
2. 网关本地工具执行器调用工具
3. 结果包装为 `tool_result` 继续喂回模型
4. 到达 `max_steps` 后停止，`stop_reason` 置为 `max_turns`

### 5.5 流式行为

- Anthropic 流式：优先透传上游原始 SSE（含 strict passthrough 语义）
- OpenAI 流式：将 canonical stream 事件映射为 OpenAI chunk / Responses stream 事件

## 6. 路由、调度、裁判、反思

### 6.1 路由优先级

`RouterService.routeForRequest` 的优先级：

1. `metadata.routing_adapter_route`
2. Dispatcher（若启用任务分发且竞选结果可用）
3. `UPSTREAM_MODEL_ROUTES_JSON` 精确匹配
4. `UPSTREAM_MODEL_ROUTES_JSON` 通配匹配
5. `*` 路由
6. `UPSTREAM_DEFAULT_ROUTE`
7. 适配器注册顺序

### 6.2 调度器健康排序（`scheduler.Engine`）

- 失败阈值 + 冷却窗口
- 成功率、延迟、连续失败综合评分
- 可按探针结果限制：
  - 流式能力
  - 工具能力
  - 模型存在性

### 6.3 并行候选 + 裁判

- `parallel_candidates > 1` 时并行请求多个候选
- `enable_response_judge=true` 时启用裁判
- 裁判模式：
  - `heuristic`：启发式打分（文本质量、stop_reason、工具一致性、延迟）
  - `llm`：用指定 judge 模型返回最优候选索引

### 6.4 反思循环（Reflection）

每轮：

1. critique：让模型审查当前答案
2. fix：按 critique 修复答案

总轮数由 `reflection_passes` 控制，累计 token usage。

### 6.5 智力评估 + 竞选 + 分发

开启条件：

- `ENABLE_TASK_DISPATCH=true`
- 适配器数量 > 1

行为：

1. 启动后约 5 秒执行智力评测（5 类题目，总分 100）
2. 竞选出 scheduler adapter（最高分）
3. 分发器按复杂度分流：
  - 复杂请求：优先 scheduler
  - 简单请求：worker 轮询，scheduler 兜底

## 7. MCP 能力

支持两类 MCP 传输：

- `http`
- `stdio`（JSON-RPC framing + initialize/ping + reconnect）

能力：

- 服务注册、更新、删除、健康检查、重连
- 工具列表缓存（默认 TTL 15s）
- 指定 server 调用工具或按名称全局匹配调用

相关环境变量：

- `MCP_SERVERS_JSON`
- `MCP_TOOLS_CACHE_TTL_MS`

## 8. 数据状态与持久化

默认存储：

- Session/Run/Plan/Todo/Event/MCP 全部为内存实现

可持久化（当前主程序已接入）：

- Run / Plan / Todo
- 通过 `STATE_PERSIST_DIR` 启用文件持久化

当前未接入主程序但已实现的存储抽象：

- `internal/storage`：memory / file / postgres(stub) / redis(stub)

## 9. 工具系统

默认本地工具执行器已注册：

- `get_weather`
- `web_search`
- `image_recognition`
- `image_analyze`
- `image_search`
- `file_read`
- `file_write`
- `file_list`

执行策略：

1. 先尝试本地工具
2. 若本地返回 `ErrToolNotImplemented`，回退 MCP `CallToolAny`

权限策略：

- 通过 `toolcatalog` + `settings` 控制 experimental/unknown 工具是否允许

## 10. 运行时配置与环境变量

### 10.1 核心

- `PORT`（默认 `8080`）
- `ADMIN_TOKEN`（未设置时默认启用 `admin123456`，可登录但会告警）
- `RUN_LOG_PATH`（默认 `logs/run-events.log`）
- `STATE_PERSIST_DIR`（为空表示不启用持久化）
- `MOCK_PRIMARY_FAIL`（仅 mock 模式下生效）

### 10.2 上游与路由

- `UPSTREAM_ADAPTERS_JSON`
- `UPSTREAM_MODEL_ROUTES_JSON`
- `UPSTREAM_DEFAULT_ROUTE`
- `UPSTREAM_TIMEOUT`（默认 `30s`）
- `UPSTREAM_RETRIES`（默认 `1`）
- `REFLECTION_PASSES`（默认 `1`）
- `PARALLEL_CANDIDATES`（默认 `1`）
- `ENABLE_RESPONSE_JUDGE`（默认 `false`）
- `ENABLE_TASK_DISPATCH`（默认 `false`）
- `INTEL_PROBE_TIMEOUT`（默认 `15s`）

### 10.3 裁判

- `JUDGE_MODE`（`heuristic` 或 `llm`，默认 `heuristic`）
- `JUDGE_ROUTE`（默认 `UPSTREAM_DEFAULT_ROUTE`）
- `JUDGE_MODEL`（`llm` 模式必填）
- `JUDGE_TIMEOUT`
- `JUDGE_RETRIES`
- `JUDGE_MAX_TOKENS`
- `JUDGE_SYSTEM_PROMPT`

### 10.4 调度器与探针

- `SCHEDULER_FAILURE_THRESHOLD`（默认 `3`）
- `SCHEDULER_COOLDOWN`（默认 `30s`）
- `SCHEDULER_STRICT_PROBE_GATE`（默认 `false`）
- `SCHEDULER_REQUIRE_STREAM_PROBE`（默认 `false`）
- `SCHEDULER_REQUIRE_TOOL_PROBE`（默认 `false`）
- `PROBE_ENABLED`（默认 `true`）
- `PROBE_INTERVAL`（默认 `45s`）
- `PROBE_TIMEOUT`（默认 `8s`）
- `PROBE_STREAM_SMOKE`（默认 `true`）
- `PROBE_TOOL_SMOKE`（默认 `true`）
- `PROBE_MODELS`
- `PROBE_MODELS_JSON`

### 10.5 模型映射 / 运行时策略 / 工具目录

- `MODEL_MAP_JSON`
- `MODEL_MAP_STRICT`
- `MODEL_MAP_FALLBACK`
- `RUNTIME_SETTINGS_JSON`
- `TOOL_CATALOG_JSON`
- `SEARCH_API_URL`（`web_search` 工具）

### 10.6 MCP

- `MCP_SERVERS_JSON`
- `MCP_TOOLS_CACHE_TTL_MS`

### 10.7 可选模块（代码已实现，默认 main 未接入）

- 限流：`RATE_LIMIT_RPS`、`RATE_LIMIT_BURST`
- 成本：`MODEL_PRICING_JSON`、`BUDGET_LIMIT_USD`

## 11. 功能实现状态（按默认主程序）

| 能力 | 代码实现 | 默认 main 接入 | API 可用 |
|---|---|---|---|
| Anthropic Messages | 是 | 是 | 是 |
| OpenAI Chat/Responses | 是 | 是 | 是 |
| SSE 流式 | 是 | 是 | 是 |
| 多后端路由/重试/超时 | 是 | 是 | 是 |
| 健康评分调度 | 是 | 是 | 是 |
| 探针（模型/流式/工具） | 是 | 是 | 是 |
| 并行候选 + 裁判 | 是 | 是 | 是 |
| 反思循环 | 是 | 是 | 是 |
| 智力评估 + 竞选 + 分发 | 是 | 是（按开关） | 是 |
| Session/Run/Plan/Todo/Event/MCP | 是 | 是 | 是 |
| Run 日志 | 是 | 是 | 是 |
| 状态持久化（Run/Plan/Todo） | 是 | 是（按目录开关） | 是 |
| Skills | 是 | 否 | 默认 501 |
| Eval | 是 | 否 | 默认 501 |
| Cost Tracking | 是 | 否 | 默认 501 |
| Plugin/Subagent/AgentTeam | 是 | 是 | 是 |
| Rules/Hooks/Tenant/Sandbox | 是 | 否 | 无默认入口 |

## 12. 快速启动

推荐直接使用一键脚本：

```bash
bash scripts/build_run_gateway.sh
```

启动后访问：

- `http://127.0.0.1:8080/`（文档入口 + 后台入口）
- `http://127.0.0.1:8080/admin/`（后台控制台）

后台支持中英文切换（登录页、导航、各面板主要文案/提示）。

默认后台密码：

- `admin123456`（仅用于开发验证）
- 生产环境请设置 `ADMIN_TOKEN` 覆盖默认值

```bash
go build ./cmd/cc-gateway
go run ./cmd/cc-gateway
```

默认监听 `http://127.0.0.1:8080`。

管理后台（Vue 版）：

```bash
cd web/admin
npm install
npm run build
cd ../..
go run ./cmd/cc-gateway
```

访问 `http://127.0.0.1:8080/admin/`。  
默认读取 `web/admin/dist`；可通过 `ADMIN_UI_DIST_DIR` 指向其它构建目录。若目录不存在，会自动回退到内置 `internal/gateway/static/dashboard.html`。

常用参数：

- `--no-ui`：跳过 Vue 后台构建（继续使用已有 dist 或旧版内置后台）
- `--skip-npm-install`：跳过 npm install
- `--test`：编译前执行 `go test ./...`
- `--build-only`：只编译不启动

基础冒烟：

```bash
bash scripts/smoke.sh
```

本地 GLM 环境变量模板：

```bash
bash scripts/use-glm-local-env.sh
```

## 13. 测试现状

- 测试目录：`tests/`
- 测试文件：52 个
- 覆盖包：29 个子目录

本地执行：

```bash
go test ./tests/... -count=1
```

说明：在受限沙箱环境下，部分依赖 `httptest` 监听端口的测试会因 `bind` 权限失败；在普通本地环境执行即可。

## 14. 目录功能索引

- `cmd/cc-gateway`：程序入口
- `internal/gateway`：HTTP handler、协议转换、SSE、admin dashboard
- `internal/upstream`：多后端适配器、路由、裁判、分发、反思
- `internal/scheduler`：健康评分、冷却、配置热更新
- `internal/probe`：健康探针与智力评测
- `internal/modelmap`：模型名映射（含通配规则）
- `internal/settings`：运行时设置中心
- `internal/policy`：动态策略引擎（结合工具目录）
- `internal/toolcatalog`：工具登记和状态控制
- `internal/toolruntime`：本地工具执行 + MCP 回退
- `internal/mcpregistry`：MCP server registry（HTTP/STDIO）
- `internal/session`：会话管理（支持 fork 与消息历史）
- `internal/ccrun`：Run 生命周期
- `internal/plan`：Plan FSM + checkpoint
- `internal/todo`：Todo 管理
- `internal/ccevent`：事件存储 + SSE 订阅广播
- `internal/statepersist`：Run/Plan/Todo 状态持久化
- `internal/runlog`：行级 JSON 运行日志
- `internal/storage`：统一存储接口及多后端实现（含 stub）
- `internal/skill`：技能引擎与 SKILL.md 解析
- `internal/eval`：模型评测、基准与回归
- `internal/ratelimit`：令牌桶限流器
- `internal/costtrack`：成本追踪器
- `internal/rules`：规则引擎（glob + 优先级）
- `internal/hooks`：生命周期钩子注册与执行
- `internal/plugin`：插件安装/启停
- `internal/tenant`：多租户与配额管理
- `internal/sandbox`：受限脚本执行器
- `internal/subagent`：子代理生命周期
- `internal/agentteam`：多代理团队编排

## 15. 变更建议

为避免文档继续分叉，建议后续只维护本文档，并让 `README.md` 作为入口导航。
