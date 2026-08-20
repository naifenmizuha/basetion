---
name: manage-roster
description: "创建或维护球队与球员资料、启用状态和背号；不用于比赛查询、比赛安排或整场比赛录入。"
---

# 管理球队与球员

使用 `team_modify` 的 `team.create`、`player.create`、`player.update`、`player.set_active` 和 `player.change_jersey`。先精确 describe 所需 operation；ID 型参数只能使用用户提供或可信工具结果中的 ID，不能从名称推测。

修改前向用户复述 operation、目标和改变后的值，并获得完整确认。可独立确定的多项修改合并进一次 `operations` 调用；批量按顺序执行且不回滚。

## Few-shot

### 创建球队

先 describe `team.create`，在用户确认后执行：

```json
{"mode":"execute","operation":"team.create","arguments":{"name":"江东队"},"confirmed":true}
```

### 修改已识别球员的背号

先 describe `player.change_jersey`；`player_id` 必须来自用户或可信读取结果：

```json
{"mode":"execute","operation":"player.change_jersey","arguments":{"player_id":"<已确认的球员 ID>","jersey_number":10},"confirmed":true}
```

### 合并两项独立变更

```json
{"mode":"execute","confirmed":true,"operations":[{"key":"activate-player","operation":"player.set_active","arguments":{"player_id":"<已确认的球员 ID>","active":true}},{"key":"change-jersey","operation":"player.change_jersey","arguments":{"player_id":"<已确认的球员 ID>","jersey_number":10}}]}
```

若返回 `stopped`，按 `stopped_at` 和逐项 `results` 报告已经生效、失败和跳过的项目；不自动重试整批。
