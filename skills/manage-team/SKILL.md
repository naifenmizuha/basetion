---
name: manage-team
description: "使用 Basetion 的球队查询与修改能力读取结构化事实、执行受限 Lua 计算，或在用户确认后修改球队数据。"
---

# 管理球队数据

`team_query` 是球队结构化事实的唯一权威入口，`team_modify` 是唯一允许的修改入口。不得根据对话、示例或一般棒球知识猜测具体球队数据、实体 ID 或工具参数。

## 查询流程

1. 涉及球队、球员、比赛、阵容、训练或表现分析事实时，先调用 `team_query` 的 `describe`。
2. 先不传 `topics` 获取顶层目录，再按任务加载精确叶子 topic。
3. 只有所需 topic 均 `found=true` 且 `available=true` 时才能查询。
4. 调用 `query` 时使用最少模块和最窄数据范围，并提供定义了 `main(team)` 的受限 Lua 5.1 程序。
5. 只有工具成功返回的数据才能作为球队事实；区分查询事实、推导判断与一般建议。

## 修改流程

1. 先通过 `team_query` 查明当前状态及稳定 ID，不得猜测或根据名称自行构造 ID。
2. 调用 `team_modify` 的 `describe` 加载所需精确 operation 及参数定义。
3. 向用户复述 operation、目标实体和所有会改变的值，并获得明确确认。
4. 仅在确认后调用 `execute`，传入精确的 `operation`、`arguments` 和 `confirmed=true`。
5. 修改成功后，必要时通过 `team_query` 复查最终状态；工具失败不代表状态已修改。

## 安全与失败处理

- `team_query` 只读；不得尝试通过 Lua 修改数据。
- `team_modify` 只执行已声明的 operation；不得提交 Lua、SQL 或任意写入脚本。
- 不得调用 shell、数据库或文件系统绕过这两个工具。
- topic 不存在、能力不可用、参数错误、执行失败、超时或取消都不是成功结果；说明原因，不得补造结果。
- 不重复提交已经失败或结果未知的修改；第一版不提供持久化幂等保护。
