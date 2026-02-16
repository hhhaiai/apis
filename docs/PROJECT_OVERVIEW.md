# 项目梳理（Project Overview）

更新时间：2026-02-16

## 1. 项目定位

CC Gateway 是统一 LLM 网关，当前目标保持三条主线：

1. 协议统一：兼容 Claude Code / Anthropic / OpenAI。
2. 控制平面：后台可实时调整路由、工具、插件、MCP、渠道与鉴权。
3. 可移植落地：在不改客户端协议的前提下，把上游、权限、诊断、观测集中在网关层。

## 2. 代码结构（当前主干）

1. 启动入口：`cmd/cc-gateway/main.go`
2. 协议与路由入口：`internal/gateway/`
3. 上游编排：`internal/upstream/`、`internal/scheduler/`、`internal/probe/`
4. 鉴权与配额：`internal/auth/`、`internal/token/`、`internal/gateway/middleware_auth.go`
5. 渠道分组路由：`internal/channel/`、`internal/gateway/channel_route_policy.go`
6. 扩展能力：
   - MCP：`internal/mcpregistry/`
   - 插件与市场：`internal/plugin/`、`internal/marketplace/`
   - 团队与子代理：`internal/agentteam/`、`internal/subagent/`
   - 工作流：`internal/plan/`、`internal/todo/`、`internal/ccevent/`
7. 运行时状态与日志：`internal/settings/`、`internal/statepersist/`、`internal/runlog/`
8. 上下文记忆：`internal/memory/`
9. 后台前端：`web/admin/`（Vue）+ `internal/gateway/static/dashboard.html`（内置回退）
10. 测试：`tests/`（集中管理）

## 3. 运行形态与默认行为

1. 默认端口 `8080`。
2. 默认启用管理员口令 `ADMIN_TOKEN=admin123456`（未设置环境变量时）。
3. 若未配置上游，自动启用 `mock-primary/mock-fallback`。
4. 默认接入 Session/Run/Plan/Todo/Event/MCP、插件市场、用户/令牌/渠道内存服务。
5. 设置 `STATE_PERSIST_DIR` 后，Run/Plan/Todo 会自动落盘与恢复。

关键入口：

1. `GET /`：首页与文档入口。
2. `GET /admin/`：后台控制台（优先 Vue dist，缺失则回退内置页面）。
3. `POST /v1/messages`：Anthropic / Claude Code 风格。
4. `POST /v1/chat/completions`：OpenAI Chat。
5. `POST /v1/responses`：OpenAI Responses。

## 4. 控制平面能力（已接入）

1. 路由与上游：模型映射、上游配置、能力矩阵、智能调度、探针。
2. 工具与扩展：工具目录、工具缺口统计、MCP、插件管理、市场安装。
3. 组织与流程：Teams/Subagents、Plans/Todos、Events 流式订阅。
4. 治理能力：用户管理、令牌管理、额度管理、渠道管理。
5. 启动与导入：Bootstrap 一键导入配置（tools/plugins/mcp/upstream）。

## 5. 请求诊断链路（新增重点）

针对 JSON 解析失败，网关已形成统一诊断闭环：

1. 解析层：`decodeJSONBodyStrict`/`decodeJSONBodySingle` 保留原始请求体。
2. 诊断上报：`reportRequestDecodeIssue` 识别 `unsupported_fields`、`trailing_json`、`empty_body`、`invalid_json`。
3. 事件可见：写入 `request.unsupported_fields` 或 `request.decode_failed`。
4. 审计可追溯：runlog 记录 `unsupported_fields`、`request_body`、`curl_command`（敏感头已脱敏）。
5. 后台可视：Events 面板展示不支持字段、请求参数和复现 curl。

## 6. 当前可移植状态

1. 协议侧（Anthropic/OpenAI/CC）已可作为统一入口迁移。
2. 控制面（路由、工具、插件、MCP、调度）已具备在线治理能力。
3. 诊断与审计链路已覆盖请求解码失败场景，便于迁移期排错。
4. 仍建议在生产迁移前补齐：
   - Auth/Token/Channel 的持久化后端（当前默认内存实现）。
   - 发布环境密钥、日志留存与备份策略。

## 7. 测试与质量

1. 测试文件总数：71（`tests/`）。
2. 全量测试基线：`go test ./... -count=1`。
3. 前端构建校验：`npm --prefix web/admin run build`。
