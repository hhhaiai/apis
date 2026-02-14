# 项目梳理（Project Overview）

更新时间：2026-02-14

## 1. 项目定位

CC Gateway 是统一网关层，目标是：

1. 协议兼容：Claude Code / Anthropic / OpenAI
2. 控制平面：通过后台和 API 统一配置与运维
3. 扩展能力：MCP、Plugins、Skills、Agent Teams、Subagents、Workflow（Plan/Todo/Event）

## 2. 代码结构

1. 启动入口：`cmd/cc-gateway/main.go`
2. 核心网关：`internal/gateway/`
3. 路由与调度：`internal/upstream/`、`internal/scheduler/`、`internal/probe/`
4. 扩展能力：
   - MCP：`internal/mcpregistry/`
   - 插件：`internal/plugin/`
   - 团队/子代理：`internal/agentteam/`、`internal/subagent/`
   - 计划/待办/事件：`internal/plan/`、`internal/todo/`、`internal/ccevent/`
5. 后台前端：`web/admin/`
6. 测试代码：`tests/`（全部测试集中在此）
7. 文档：`docs/`
8. 脚本：`scripts/`

## 3. 运行形态

默认端口：`8080`

关键入口：

1. `GET /`：文档介绍 + 后台入口
2. `GET /admin/`：后台控制台
3. `POST /v1/messages`：Claude Code / Anthropic 风格
4. `POST /v1/chat/completions`：OpenAI Chat
5. `POST /v1/responses`：OpenAI Responses

## 4. 管理与控制能力

后台 + API 支持：

1. 模型调度与路由参数控制
2. 工具目录与运行策略控制
3. MCP 服务编排与工具调用入口
4. 插件安装、启停、删除
5. Team/Subagent 协作编排
6. Plan/Todo/Event 工作流闭环
7. 成本统计与评估入口

## 5. 一键运行脚本

脚本：`scripts/build_run_gateway.sh`

用途：

1. 构建前端后台
2. 构建后端网关
3. 可选运行测试
4. 一键启动服务

使用详情见 `README.md`。

## 6. 测试与质量

1. 测试文件统一在 `tests/`
2. 建议变更后执行：`go test ./...`
3. 前端变更后执行：`npm --prefix web/admin run build`
