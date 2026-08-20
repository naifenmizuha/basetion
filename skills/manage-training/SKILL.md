---
name: manage-training
description: "创建、更新或软删除球员每日自训记录；不用于球队名单、比赛或比赛过程管理。"
---

# 管理训练记录

使用 `team_modify` 的 `training.create`、`training.update` 和 `training.delete`。先 describe 精确 operation；`player_id`、`training_id` 只能使用用户提供或可信工具结果，不能从球员姓名拼接。

向用户复述训练对象、日期、内容和将发生的变化，获得确认后 execute。删除为软删除；批量失败时依次解释已成功、失败和跳过项。

## Few-shot

### 创建每日训练

```json
{"mode":"execute","operation":"training.create","arguments":{"player_id":"<已确认的球员 ID>","training_date":"2026-09-01","content":"短打练习 40 分钟","reflection":"触击方向稳定"},"confirmed":true}
```

### 更新训练内容

```json
{"mode":"execute","operation":"training.update","arguments":{"training_id":"<已确认的训练 ID>","content":"短打与跑垒练习 60 分钟","reflection":"补充二垒起跑判断"},"confirmed":true}
```

### 删除训练记录

```json
{"mode":"execute","operation":"training.delete","arguments":{"training_id":"<已确认的训练 ID>"},"confirmed":true}
```
