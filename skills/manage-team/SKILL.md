---
name: manage-team
description: "使用 Basetion 的球队查询与修改能力读取结构化事实、执行受限 Lua 计算，或在用户确认后修改球队数据。"
---

# 管理球队数据

`team_fetch` 提供常用的完整比赛读取，`team_query` 提供球队、球员、比赛和原子 Play 的自由组合，`team_modify` 是唯一允许的修改入口。不得根据对话、示例或一般棒球知识猜测具体球队数据、实体 ID 或工具参数。

## 查询流程

### 查询能力目录

- `team.list`：列出球队，可选只返回启用球队。
- `player.list`：按球队名称、背号和守备位置查询当前球员。
- `team_query`：`team.list`、`player.list`、`game.list` 和 `game.plays`。`game.plays` 只接受同一次 Lua 执行中由 `game.list` 返回并原样保留的比赛对象；不提供内部 ID。
- `team_fetch`：`game.summaries`、`game.records`、`game.lineups` 和 `game.performances`。这些结果由服务端按领域规则生成，不能用 Lua 重算或替代。

训练查询当前未适配到新的姓名化 Lua 协议。不得通过猜测或拼接 Player ID 查询训练记录。

### 编排规则

1. 已有完整读取可直接回答时，优先 `team_fetch`；不要为了取得比分、阵容或球员表现生成 Lua。
2. 调用相应工具的 `describe`，在 `topics` 中同时加载全部所需叶子；只有它们均 `found=true` 且可用时才能执行。Skill 目录只用于选择 topic，参数、返回字段和示例始终以本次 `describe` 结果为准。
3. 需要自由组合基础事实时，调用 `team_query` 的 `query`，提供定义了 `main(data)` 的受限 Lua 5.1 程序；不需要也不能声明模块。
4. Lua 可以先读取比赛、依据实际返回结果筛选，再将原始比赛对象批量传给 `data.game.plays`。只要中间结果不需要模型重新判断，必须在同一个 Lua 程序中完成；不得拆分工具调用来猜测或传递内部 ID。
5. 不带 `topics` 的根目录 `describe` 仅用于无法从本 Skill 确定 topic、精确 topic 返回未找到，或用户明确要求探索能力的情况。
6. 使用最窄的数据范围，并只返回回答或修改所需的数据。只有工具成功返回的数据才能作为球队事实；区分查询事实、推导判断与一般建议。

### 查询结果投影

1. 写 Lua 前先确定最终回答需要的字段和明细粒度。领域查询投影只用于脚本内部计算，不得直接作为 `main` 的返回值。
2. 必须新建结果 table，构造面向当前任务的最小证据结构，只复制回答所需的字段。
3. 不返回实体 ID、关联 ID、版本、创建/更新时间等内部字段；不得尝试从名称推测 ID。
4. 对大量明细先在 Lua 中过滤、分组、聚合、排序和压缩重复模式。用户要求摘要时不得返回完整原始记录。
5. 返回结构应使用清晰的业务名称，让模型能直接依据结果回答，不再承担第二次数据清洗或关联工作。

### 查询示例

已通过一次 `team_fetch` 的 `describe` 加载 `game.summaries` 后，直接取得指定球队本月的比赛摘要：

```json
{"mode":"fetch","operation":"game.summaries","arguments":{"participant_names":["蜀汉队"],"date_from":"2026-08-01","date_to":"2026-08-31","limit":100}}
```

### 复杂比赛分析

需要比较所有比赛的进攻、投球和守备表现时，先在一次 `team_fetch` 的 `describe` 中加载 `game.performances`；如需核对比分或交代比赛背景，同时加载 `game.summaries`。确认这些 topic 均可用后，用 `team_fetch` 读取相应结果并按当前回答需要总结。

`game.performances` 是逐场统计的来源，不能用 `game.records` 或 `game.lineups` 重算或替代。回答时要保留 `limits` 中与结论相关的限制：守备结果为空不代表没有守备机会；自责分只统计明确标记的记录；没有被记录的逐球事实不会进入统计。

## 修改流程

### 修改能力目录

- `team.create`：创建球队。
- `player.create`、`player.update`、`player.set_active`、`player.change_jersey`：创建、更新、启停或修改球员当前背号。球员创建时必须指定所属球队；不支持入队、离队或转队。
- `match.create`、`match.update`、`match.set_status`、`match.delete`：创建、更新、修改状态或软删除比赛。
- `lineup.create`、`lineup.replace`、`lineup.delete`：创建、替换或软删除阵容。
- `training.create`、`training.update`、`training.delete`：创建、更新或软删除自训记录；本轮没有新的姓名化目标解析能力。

### 编排与确认

1. 先通过可用的 `team_query` 基础对象或 `team_fetch` 组合读取查明当前状态；不得猜测或根据名称自行构造 ID。
2. 从上述目录选择全部必要 operation，调用一次 `team_modify` 的 `describe`，在 `topics` 中同时加载其精确参数定义；已能确定查询叶子和修改 operation 时，应在同一轮并行执行两者的精确 `describe`。
3. 向用户按执行顺序复述所有 operation、目标实体和会改变的值，并获得对完整批次的明确确认。
4. 仅在确认后调用 `execute`。单项修改传入精确的 `operation`、`arguments` 和 `confirmed=true`；多个可独立确定参数的修改应合并到一个 `operations` 数组，每项提供唯一非空 `key`，整批只传一次 `confirmed=true`。
5. 批量修改严格按数组顺序执行且不回滚；返回 `status=stopped` 时，依据 `stopped_at` 和逐项结果说明此前哪些成功、哪项因何失败、哪些 `skipped` 未执行，不得自动重试整个批次。
6. 修改成功后，必要时通过 `team_query` 复查最终状态；工具失败不代表状态已修改。

## 安全与失败处理

- `team_query` 只读；不得尝试通过 Lua 修改数据。
- `team_modify` 只执行已声明的 operation；不得提交 Lua、SQL 或任意写入脚本。
- 不得调用 shell、数据库或文件系统绕过这两个工具。
- topic 不存在、能力不可用、参数错误、执行失败、超时或取消都不是成功结果；说明原因，不得补造结果。
- 不重复提交已经失败或结果未知的修改；第一版不提供持久化幂等保护。
- 批量修改最多包含 50 项；批次不具备原子性，失败前的 `succeeded` 项已经生效，失败后的 `skipped` 项没有执行。
