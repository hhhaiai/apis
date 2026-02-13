# CC Gateway

统一 LLM API 网关，兼容 Anthropic Messages、OpenAI Chat Completions/Responses，并提供 CC 扩展能力（Session/Run/Plan/Todo/Event/MCP）。

## 完整文档

- 统一总文档：`docs/PROJECT_FULL_GUIDE.md`

## 快速启动

```bash
go build ./cmd/cc-gateway
go run ./cmd/cc-gateway
```

默认监听：`http://127.0.0.1:8080`

## 快速验证

```bash
bash scripts/smoke.sh
```

## 常用配置样例

- 运行时设置：`configs/runtime-settings.example.json`
- 本地 GLM 上游：`configs/upstream-glm-local.example.json`
- Probe 模型映射：`configs/probe-models.example.json`
- 工具目录：`configs/tool-catalog.example.json`
