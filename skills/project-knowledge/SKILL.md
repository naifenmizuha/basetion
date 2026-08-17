---
name: project-knowledge
description: "介绍 Basetion 的定位、当前能力、源码架构、会话模型和明确限制。用于回答 Basetion 是什么、能做什么、如何组织以及 Session 与 Checkpoint 有何区别等项目自身问题。"
---

# Basetion 项目知识

## 定位与当前能力

Basetion 是一个以 Go 和 CloudWeGo Eino ADK 为核心的 Agent 应用骨架，当前提供 CLI 入口。它用于验证可复用的会话应用服务、Agent Harness、渐进式 Skill 和受约束的球队查询与修改能力；HTTP 与 QQ 机器人入口尚未实现。

回答“有什么能力”时应面向用户简短说明当前真实能力，不要把未来规划描述为已经完成：

- 可以在多轮 Session 中延续已经成功完成的对话。
- 可以按需加载项目知识与球队管理 Skill。
- 可以通过 `team_query` 的 `roster` 模块读取 PostgreSQL 中的球队、球员与名单，并通过 `team_modify` 执行预定义修改；比赛、阵容、训练和分析模块目前尚未接入。
- 可以执行不依赖球队数据模块的受限只读 Lua 计算。

## 六个职责区域

1. `src/internal/entry/cli` 解析入口协议并渲染 AgentEvent，不承载业务规则。
2. `src/internal/application/conversation` 管理业务 Session、并发锁和一轮对话的事务边界。
3. `src/internal/harness` 组合 AgenticModel、Skills、工具、回调与 TypedRunner。
4. `src/internal/tools` 将模型工具协议适配到领域服务。
5. `src/internal/domain` 表达球队查询等领域规则，只依赖抽象。
6. `src/internal/infra` 实现文件 Session 存储和受限 Lua 运行时等基础设施。

依赖方向从入口和基础设施装配流向应用、Harness、工具与领域边界；入口不直接操作存储或模型。

## Session 与 Checkpoint

业务 Session 保存已经成功完成的完整对话历史。一次运行失败或取消时，不覆盖上一次成功快照。相同 Session ID 的轮次串行执行，不同 Session 可以并行。

Eino Checkpoint 用于恢复一次被中断的 Agent 运行。Basetion 当前未启用 Checkpoint，不能用业务 Session 代替它。

## 回答约束

- 只陈述本 Skill 明确给出的项目事实，不根据相似项目补全能力。
- 用户询问实现细节时可以使用上述模块名称；一般功能介绍应优先使用用户任务语言。
- 涉及具体球队数据时停止使用本 Skill 推断，改为加载 `manage-team`。
