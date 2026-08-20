---
name: manage-game-setup
description: "创建或维护比赛安排、比赛状态和阵容；不用于逐球完成比赛录入或比赛数据分析。"
---

# 管理比赛安排与阵容

使用 `team_modify` 的 `match.create`、`match.update`、`match.set_status`、`match.delete`、`lineup.create`、`lineup.replace` 和 `lineup.delete`。完整的已结束比赛、双方首发和 Play 应加载 `record-game`，不在本 Skill 中拆写。

先加载每个所需 operation 的精确 describe。涉及现有比赛、球队或球员的 ID 只能使用用户提供或可信工具结果；确认后再 execute。删除是软删除，比赛删除会同步软删除关联阵容和 Play。

## Few-shot

### 更新比赛状态

```json
{"mode":"execute","operation":"match.set_status","arguments":{"match_id":"<已确认的比赛 ID>","status":"in_progress"},"confirmed":true}
```

### 创建首发阵容

先 describe `lineup.create`；以下展示阵容 key 和条目结构，所有 ID 都须已确定：

```json
{"mode":"execute","operation":"lineup.create","arguments":{"match_id":"<已确认的比赛 ID>","team_id":"<已确认的球队 ID>","kind":"starter","variant_number":0,"variant_name":"首发","entries":[{"player_id":"<已确认的球员 ID>","batting_order":1,"position":"first_base"}]},"confirmed":true}
```

### 替换阵容

`lineup.replace` 使用同一阵容 key 和完整的新 entries；它不是逐条 patch。确认范围应包含被替换的整套阵容。
