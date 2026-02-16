# 后台功能清单（Admin Console）

更新时间：2026-02-16  
后台入口：`/admin/`

## 1. 登录与鉴权

1. 鉴权探测：`GET /admin/auth/status`
2. 管理接口鉴权：支持 `x-admin-token` 或 `Authorization: Bearer <token>`
3. 默认管理员口令：`admin123456`（仅开发便利，生产需替换）
4. 若仍使用默认口令，登录页与顶部会显示安全警告

## 2. 后台模块总览（Vue 控制台）

当前页面模块：

1. Overview
2. Settings
3. Intelligent Dispatch
4. Scheduler
5. Models
6. Tools
7. Plugins
8. MCP
9. Bootstrap
10. Channels
11. Auth（用户与令牌）
12. Teams
13. Subagents
14. Events
15. Todos
16. Plans
17. Skills
18. Rules
19. Cost
20. Eval

## 3. 核心能力映射

1. 路由与模型：`/admin/settings`、`/admin/model-mapping`、`/admin/upstream`
2. 调度与探针：`/admin/intelligent-dispatch`、`/admin/scheduler`、`/admin/probe`
3. 工具治理：`/admin/tools`、`/admin/tools/gaps`、`/admin/capabilities`
4. 插件与市场：`/v1/cc/plugins*`、`/v1/cc/marketplace*`、`/admin/marketplace/cloud/*`
5. MCP 管理：`/v1/cc/mcp/servers*`
6. 组织协作：`/v1/cc/teams*`、`/v1/cc/subagents*`、`/v1/cc/plans*`、`/v1/cc/todos*`
7. 用户与令牌：
   - `GET/POST /admin/auth/users`
   - `GET/PUT/DELETE /admin/auth/users/{user_id}`
   - `GET/POST /admin/auth/users/{user_id}/tokens`
   - `GET/PUT/DELETE /admin/auth/users/{user_id}/tokens/{token_id}`
   - `GET/POST /admin/auth/users/{user_id}/quota`
8. 渠道管理：
   - `GET/POST /admin/channels`
   - `GET/PUT/DELETE /admin/channels/{id}`
   - `PUT /admin/channels/{id}/status`
   - `POST /admin/channels/{id}/test`

## 4. 事件与诊断可视化（新增）

Events 面板支持以下诊断事件：

1. `request.unsupported_fields`
2. `request.decode_failed`

诊断列可直接查看：

1. 不支持字段列表（`unsupported_fields`）
2. 失败原因（`reason`）
3. 请求参数原文（`request_body`）
4. 复现命令（`curl_command`，敏感 token 已脱敏）

这套信息同时来自事件流与 runlog，便于排障闭环。

## 5. 作用域与项目隔离

后台支持 `project/global` 两级作用域：

1. Header：`x-project-id`
2. Query：`scope=project|global` + `project_id=<id>`
3. `plugins / mcp / tools` 默认按项目隔离，`global` 可做全局策略

## 6. UI 运行模式

1. 优先加载 `web/admin/dist`（可由 `ADMIN_UI_DIST_DIR` 覆盖）。
2. 若 dist 缺失，自动回退内置页面 `internal/gateway/static/dashboard.html`。

## 7. 多语言与显示约束

1. 控制台支持中英文切换，语言保存在 `cc_admin_lang`。
2. 协议字段保持英文原名（如 `run_id`、`event_type`），避免审计歧义。
