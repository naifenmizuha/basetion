---
name: query-team-data
description: "查询 Basetion 中球队、球员和比赛事实，或分析比赛表现；适用于只读问题，不用于创建或修改数据。"
---

# 查询球队数据

使用 `team_fetch` 获取服务端已组合的比赛摘要、记录、阵容和表现；只有需要自由组合球队、球员、比赛和原子 Play 时才使用只读的 `team_query` Lua。先通过 `team_describe` 对所需叶子调用精确的 `fetch.*` 或 `query.*` topic，以本次返回的参数和字段为准。

## 选择读取方式

- 需要比分、赛果、阵容、完整记录或球员表现：优先 `team_fetch`。
- 需要把基础事实跨实体关联、过滤或聚合：使用 `team_query`，在一个 `main(data)` 中完成关联与投影。
- Lua 的 `game.plays` 只接收同一次执行中 `game.list` 返回并原样保留的比赛对象；不返回或推测内部 ID。

在 Lua 中新建面向回答的最小 table。不要直接返回原始领域对象；大量明细应先过滤、分组、聚合和排序。

## Few-shot

### 读取球队本月比赛摘要

先 describe `fetch.game.summaries` 后，直接使用服务端摘要：

```json
{"operation":"game.summaries","arguments":{"participant_names":["蜀汉队"],"date_from":"2026-08-01","date_to":"2026-08-31","limit":100}}
```

### 按球队投影当前球员名单

先 describe `query.player.list`，再在 Lua 中仅保留回答需要的业务字段：

```lua
function main(data)
  local rows = data.player.list({team_names = {"蜀汉队"}})
  local result = {}
  for _, player in ipairs(rows) do
    table.insert(result, {team_name = player.team_name, name = player.name, jersey_number = player.jersey_number, positions = player.positions})
  end
  return result
end
```

### 读取已筛选比赛的原子 Play

先 describe `query.game.list` 和 `query.game.plays`。筛选与 Play 读取留在同一个程序中，再投影所需字段：

```lua
function main(data)
  local matches = data.game.list({participant_names = {"曹魏队", "蜀汉队"}, date_from = "2026-09-01", date_to = "2026-09-01", limit = 1})
  local plays = data.game.plays({matches = matches})
  local result = {}
  for _, play in ipairs(plays) do
    table.insert(result, {inning = play.inning, half = play.half, batter = play.batter.name, result = play.result_description})
  end
  return result
end
```
