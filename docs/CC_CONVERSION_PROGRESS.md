# Claude Code 转换系统完善进度

> 第二部分实施进度跟踪（按当前代码）  
> 更新时间：2026-02-16

## 1. 当前结论

项目已从“协议兼容原型”进入“可迁移上线准备”阶段：

1. 协议转换链路可用：Anthropic/OpenAI/CC 三条主入口已统一路由。
2. 治理面已可用：后台可管理上游、工具、MCP、插件、计划、团队、鉴权与渠道。
3. 诊断链路已补齐：请求解码失败可定位“哪个字段不支持、原始参数是什么、如何复现”。

## 2. 完成度矩阵（按目标域）

| 目标域 | 当前状态 | 说明 |
|---|---|---|
| 协议转换（Claude/OpenAI） | 已完成 | `messages/chat/responses` 全链路可用 |
| 模型映射与路由 | 已完成 | 支持运行时更新与多路由策略 |
| Tool Loop 与 MCP 回退 | 已完成 | native/react/json/hybrid + MCP fallback |
| Session/Run/Plan/Todo/Event | 已完成 | 管理接口与事件流可用 |
| 插件市场与云端清单安装 | 已完成 | 本地清单 + 云端拉取/安装 |
| 用户/令牌/配额 | 已完成（内存版） | 已接入网关鉴权与额度结算 |
| 渠道路由（group+model） | 已完成（内存版） | 按用户组与模型选择渠道 |
| 上下文记忆与总结 | 部分完成 | 工作记忆与 recent summary 已接入，长期记忆未产品化 |
| 生产持久化（Auth/Token/Channel） | 待完成 | 目前默认内存，不满足生产恢复要求 |

## 3. 本轮关键落地（新增重点）

### 3.1 不支持字段与解码失败诊断闭环

已落地模块：

1. `internal/gateway/json_body.go`
   - 严格/兼容双解码策略统一。
   - 出错时保留原始 body，便于排障。
2. `internal/gateway/request_decode_diagnostics.go`
   - 自动识别 `unsupported_fields/trailing_json/empty_body/invalid_json`。
   - 生成脱敏复现 curl（`Authorization/x-admin-token/x-api-key` 自动隐藏）。
3. `internal/runlog/logger.go`
   - runlog 新增 `reason/unsupported_fields/request_body/curl_command` 字段。
4. 后台展示
   - Vue：`web/admin/src/components/panels/EventsPanel.vue`
   - 内置回退页：`internal/gateway/static/dashboard.html`
   - 可直接查看不支持字段、请求参数与复现命令。

### 3.2 事件类型

新增可观测事件：

1. `request.unsupported_fields`
2. `request.decode_failed`

## 4. 测试与验证（本次基线）

1. 全量后端测试通过：`go test ./... -count=1`
2. 当前测试规模：
   - 测试文件：71
   - 测试包：34

## 5. 仍需推进（P0）

1. Auth/Token/Channel 持久化存储接入（避免重启丢失）。
2. 生产安全收敛：
   - 移除默认口令依赖
   - 明确最小权限 token 策略
   - 日志留存与脱敏审计策略
3. 迁移演练：
   - 灰度流量下字段兼容性回归
   - 失败诊断事件告警与工单闭环

## 6. 可移植目标（当前判定）

当前代码已具备“可迁移”骨架，但要达到“可生产托管”还需满足：

1. 关键状态持久化完成（Auth/Token/Channel）。
2. 生产安全配置模板完成（密钥、鉴权、日志、备份）。
3. 灰度与回滚预案跑通并留档。
