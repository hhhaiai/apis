# CC Gateway

统一 LLM API 网关，兼容 Claude Code / Anthropic Messages、OpenAI Chat Completions/Responses，并提供自有扩展能力（MCP / Plugins / Agent Teams / Subagents / Plans / Todos / Skills 等）。

## 文档入口

- 项目完整文档：`docs/PROJECT_FULL_GUIDE.md`
- 项目结构与运行梳理：`docs/PROJECT_OVERVIEW.md`
- 后台功能清单与多语言说明：`docs/ADMIN_CONSOLE_FEATURES.md`

## 一键运行（推荐）

```bash
bash scripts/build_run_gateway.sh
```

默认行为：

1. 构建后台前端（Vue + Vite）
2. 构建网关二进制（`bin/cc-gateway`）
3. 启动服务（默认端口 `8080`）

启动后访问：

- 入口页（文档介绍 + 后台入口）：`http://127.0.0.1:8080/`
- 后台控制台：`http://127.0.0.1:8080/admin/`
- 健康检查：`http://127.0.0.1:8080/healthz`

默认后台密码：

- 默认 `ADMIN_TOKEN=admin123456`
- 未修改时可登录，但后台会持续显示安全告警
- 生产环境务必设置自定义 `ADMIN_TOKEN`

## 一键脚本参数

```bash
bash scripts/build_run_gateway.sh --help
```

常用示例：

```bash
# 仅构建，不启动
bash scripts/build_run_gateway.sh --build-only

# 启动前先跑后端测试
bash scripts/build_run_gateway.sh --test

# 使用自定义端口
bash scripts/build_run_gateway.sh --port 18080

# 跳过前端构建（仅后端）
bash scripts/build_run_gateway.sh --no-ui

# 跳过 npm install（依赖已安装时）
bash scripts/build_run_gateway.sh --skip-npm-install
```

## 测试代码位置

所有测试代码统一位于 `tests/` 目录（按模块分子目录），例如：

- `tests/gateway/`
- `tests/upstream/`
- `tests/agentteam/`
- `tests/subagent/`

执行：

```bash
go test ./...
```

## 常用配置样例

- 运行时设置：`configs/runtime-settings.example.json`
- 本地 GLM 上游：`configs/upstream-glm-local.example.json`
- Probe 模型映射：`configs/probe-models.example.json`
- 工具目录：`configs/tool-catalog.example.json`
