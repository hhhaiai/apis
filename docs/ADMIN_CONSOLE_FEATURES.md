# 后台功能清单（Admin Console）

更新时间：2026-02-14  
后台入口：`/admin/`

## 1. 入口与鉴权

1. 首页入口：`/`（文档介绍 + 后台入口）
2. 后台鉴权状态：`GET /admin/auth/status`
3. 默认后台密码：`admin123456`
4. 若未自定义 `ADMIN_TOKEN`，系统允许登录但持续告警

## 2. 后台模块总览

后台页面采用 Vue + Vite，按模块分为：

1. Overview（系统总览）
2. Settings（运行时设置）
3. Models（模型路由）
4. Tools（工具目录）
5. Plugins（插件中心）
6. MCP（MCP 服务管理）
7. Agent Teams（团队与任务协作）
8. Subagents（子代理管理）
9. Events（事件流与审计）
10. Todos（任务看板）
11. Plans（计划编排）
12. Skills（技能管理）
13. Rules（本地规则草稿）
14. Cost（成本追踪）
15. Eval（智能评估）

## 3. 各模块能力映射

1. Overview
   - 系统健康、调度器状态、关键运行参数
2. Settings
   - 反思轮次、超时、并行候选、重试、工具循环、模式路由
3. Models
   - mode -> model 覆写、mode route chain 展示
4. Tools
   - 工具目录查看（类别、启用状态）
5. Plugins
   - 插件安装、启停、删除、详情查看
6. MCP
   - 服务注册、健康检查、重连、工具列表预览、删除
7. Agent Teams
   - 团队创建、任务创建、任务列表、一键 orchestrate
8. Subagents
   - 查询、终止、删除、时间线查看
9. Events
   - 事件查询 + SSE 流式订阅（支持过滤）
10. Todos
    - 创建、列表、状态更新、详情查看
11. Plans
    - 创建、审批、执行推进、详情及关联 Todo
12. Skills
    - 注册、列表、删除
13. Rules
    - 本地草稿规则维护（后端规则 API 预留）
14. Cost
    - 总成本、预算、模型维度成本统计
15. Eval
    - 评测输入、打分结果、分析文本

## 4. 多语言支持

后台支持中英文双语切换：

1. 登录页与导航支持中英切换
2. 各功能面板标题、按钮、空状态、主要错误提示均支持中英展示
3. 语言状态会持久化到浏览器本地（`cc_admin_lang`）

说明：

1. API 返回字段名保持协议原样（如 `run_id` / `event_type`），不做本地化改写
2. 技术字段与路径保持英文，保证与协议/日志一致

## 5. 日志与记录文本（Record Text）

为支持高并发任务同步与审计，系统在事件数据中保留文本记录：

1. 子代理生命周期事件包含 `record_text`
2. Team 任务生命周期事件包含 `record_text`
3. 事件流可通过 `/v1/cc/events` 与 `/v1/cc/events/stream` 查询/订阅

这部分可以作为多任务同步与追踪的统一文本依据。
