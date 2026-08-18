---
name: manage-team
description: "使用 Basetion 的球队查询与修改能力读取结构化事实、执行受限 Lua 计算，或在用户确认后修改球队数据。"
---

# 管理球队数据

`team_query` 是球队结构化事实的唯一权威入口，`team_modify` 是唯一允许的修改入口。不得根据对话、示例或一般棒球知识猜测具体球队数据、实体 ID 或工具参数。

## 查询流程

### 查询能力目录

- `roster.teams`：列出球队并取得球队 ID。
- `roster.players`：按球队读取球员、名单关系和球员 ID。
- `game.matches`：按球队、日期或状态筛选比赛。
- `game.match`：读取单场比赛。
- `game.plays`：一次读取单场比赛的完整过程，包括逐球、跑垒和守备结果。
- `game.score`：读取单场比赛的当前或最终比分。
- `lineup.list`：读取单场比赛的阵容。
- `training.records`：按球员和日期范围读取每日自训记录。

### 编排规则

1. 涉及球队、球员、比赛、阵容、训练或表现分析事实时，先从上述目录选择完成任务所需的全部精确叶子 topic。
2. 一次调用 `team_query` 的 `describe`，在 `topics` 中同时加载全部所需叶子；只有它们均 `found=true` 且 `available=true` 时才能查询。Skill 目录只用于选择 topic，参数、返回字段和示例始终以本次 `describe` 结果为准。
3. 调用 `query` 前规划完整数据依赖，一次启用全部必要模块，并提供定义了 `main(team)` 的受限 Lua 5.1 程序。
4. 只要中间结果不需要模型重新判断，就必须在一次 `query` 的同一个 Lua 程序中完成 ID 查找、关联、过滤、聚合和去重；不得仅为取得下一次查询所需的 ID 而拆分 `query`。只有中间结果确实会改变后续语义决策时才分步查询。
5. 不带 `topics` 的根目录 `describe` 仅用于无法从本 Skill 确定 topic、精确 topic 返回未找到，或用户明确要求探索能力的情况。
6. 使用最窄的数据范围，并只返回回答或修改所需的数据。只有工具成功返回的数据才能作为球队事实；区分查询事实、推导判断与一般建议。

### 查询结果投影

1. 写 Lua 前先确定最终回答需要的字段和明细粒度。领域查询返回的原始对象只用于脚本内部计算，不得直接作为 `main` 的返回值。
2. 必须新建结果 table，构造面向当前任务的最小证据结构，只复制回答或后续修改必需的字段。
3. 除非后续 `team_modify` 确实需要稳定 ID，否则不返回实体 ID、关联 ID、版本、创建/更新时间等内部字段；已经转换为名称或业务标签的 ID 不得同时返回。
4. 对大量明细先在 Lua 中过滤、分组、聚合、排序和压缩重复模式。用户要求摘要时不得返回完整原始记录；用户要求完整明细时仍须删除每项重复且无用的字段。
5. 返回结构应使用清晰的业务名称，让模型能直接依据结果回答，不再承担第二次数据清洗或关联工作。

### 跨模块组合示例

已通过一次 `describe` 加载 `roster.teams`、`roster.players` 和 `training.records` 后，可用一次 `query` 启用 `roster` 与 `training`，在同一程序内完成“球队名 → 球员 ID → 当日训练记录”：

```lua
function main(team)
  local target_team
  for _, value in ipairs(team.roster.teams({active = true})) do
    if value.name == "蜀汉队" then target_team = value; break end
  end
  if not target_team then return {error = "team_not_found"} end

  local target_player
  for _, value in ipairs(team.roster.players({team_id = target_team.id})) do
    if value.name == "刘备" then target_player = value; break end
  end
  if not target_player then return {error = "player_not_found"} end

  local projected_records = {}
  for _, record in ipairs(team.training.records({
    player_id = target_player.player_id,
    from_date = "2026-08-18",
    to_date = "2026-08-18",
  })) do
    projected_records[#projected_records + 1] = {
      date = record.training_date,
      content = record.content,
      reflection = record.reflection,
    }
  end

  return {
    player = {name = target_player.name, team = target_team.name},
    records = projected_records,
  }
end
```

比赛查询同样应投影结果。以下示例只保留首发阵容的棒次、姓名和位置，并把完整打席压缩为回答所需的顺序、局次、打者姓名和结果说明：

```lua
function main(team)
  local match = team.game.match({match_id = "match-1"})
  local player_name = {}
  for _, team_id in ipairs({match.home_team_id, match.away_team_id}) do
    for _, player in ipairs(team.roster.players({team_id = team_id})) do
      player_name[player.player_id] = player.name
    end
  end

  local starters = {}
  for _, lineup in ipairs(team.lineup.list({match_id = match.id})) do
    if lineup.kind == "starter" then
      local entries = {}
      for _, entry in ipairs(lineup.entries) do
        entries[#entries + 1] = {
          order = entry.batting_order,
          name = player_name[entry.player_id],
          position = entry.position,
        }
      end
      starters[#starters + 1] = {entries = entries}
    end
  end

  local events = {}
  for _, play in ipairs(team.game.plays({match_id = match.id})) do
    events[#events + 1] = {
      sequence = play.sequence,
      inning = play.inning,
      half = play.half,
      batter = player_name[play.batter_id],
      result = play.result_description,
    }
  end

  return {starters = starters, events = events}
end
```

## 修改流程

### 修改能力目录

- `team.create`：创建球队。
- `player.create`、`player.update`、`player.set_active`：创建、更新或启停球员。
- `roster.assign`、`roster.change_jersey`、`roster.leave`：入队、改号或离队。
- `match.create`、`match.update`、`match.set_status`、`match.delete`：创建、更新、修改状态或软删除比赛。
- `lineup.create`、`lineup.replace`、`lineup.delete`：创建、替换或软删除阵容。
- `training.create`、`training.update`、`training.delete`：创建、更新或软删除自训记录。

### 编排与确认

1. 先通过 `team_query` 查明当前状态及稳定 ID，不得猜测或根据名称自行构造 ID。
2. 从上述目录选择全部必要 operation，调用一次 `team_modify` 的 `describe`，在 `topics` 中同时加载其精确参数定义；已能确定查询叶子和修改 operation 时，应在同一轮并行执行两者的精确 `describe`。不带 `topics` 的修改根目录 `describe` 仅用于无法确定 operation、精确 operation 未找到或能力探索。
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
