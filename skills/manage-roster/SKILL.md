---
name: manage-roster
description: "创建或维护球队与球员资料、启用状态和背号；不用于比赛查询、比赛安排或整场比赛录入。"
---

# 管理球队与球员

使用 `team_describe` 读取 `modify.team.create`、`modify.player.create`、`modify.player.update`、`modify.player.set_active` 和 `modify.player.change_jersey` 的说明，再使用 `team_modify` 执行。describe 载荷为 `{"topics":["modify.<operation>"]}`。ID 型参数只能使用用户提供或可信工具结果中的 ID，不能从名称推测。

修改前向用户复述 operation、目标和改变后的值，并获得完整确认。所有写入（包括单项）都通过 `team_modify` 的 `operations` 数组提交；批量按顺序执行且不回滚。

## Few-shot

### 创建球队

先通过 `team_describe` 读取 `modify.team.create`，在用户确认后执行：

```json
{"confirmed":true,"operations":[{"key":"create-team","operation":"team.create","arguments":{"name":"江东队"}}]}
```

### 修改已识别球员的背号

先通过 `team_describe` 读取 `modify.player.change_jersey`；`player_id` 必须来自用户或可信读取结果：

```json
{"confirmed":true,"operations":[{"key":"change-jersey","operation":"player.change_jersey","arguments":{"player_id":"<已确认的球员 ID>","jersey_number":10}}]}
```

### 合并两项独立变更

```json
{"confirmed":true,"operations":[{"key":"activate-player","operation":"player.set_active","arguments":{"player_id":"<已确认的球员 ID>","active":true}},{"key":"change-jersey","operation":"player.change_jersey","arguments":{"player_id":"<已确认的球员 ID>","jersey_number":10}}]}
```

若返回 `stopped`，按 `stopped_at` 和逐项 `results` 报告已经生效、失败和跳过的项目；不自动重试整批。
