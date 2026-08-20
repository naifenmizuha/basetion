---
name: record-game
description: "录入一场已结束的棒球比赛，适用于用户同时提供双方首发和逐打席或逐球比赛过程；不用于查询比赛或单独维护比赛元数据、阵容。"
---

# 录入完整比赛

使用 `team_modify` 的 `game.create` 一次录入比赛、双方首发和全部 Play。球队与球员通过精确球队名和当前背号解析，不传内部 ID。

## 工作流

1. 首先调用 `team_modify.describe` 并仅请求 `game.create`。字段含义、枚举、`conventions` 与 `invariants` 是当前协议的权威来源。
2. 将用户叙述整理为双方首发和连续的 Play。阵容条目只写背号；每个 Play 只填写事件事实：打者/投手背号、逐球结果、打席结果、跑垒和守备事实。球队、好坏球数、垒包、出局和比分由服务端推导。
3. 向用户复述比赛安排、双方首发、Play 数量、最终比分及将写入的逐球、跑垒和守备范围，并取得完整确认。用户已明确确认“直接执行”时，可将该确认用于本次写入。
4. 只调用一次 `team_modify.execute`：`operation="game.create"`、完整 `arguments`、`confirmed=true`。不要把一场比赛拆成多个修改。
5. 成功后报告比赛和完成数量。返回 `partial` 时，依据 `completed_lineups`、`completed_plays`、`stopped_stage`、`stopped_play_index` 说明已保留内容；不要自动重试，先读取现状并由用户决定后续处理。

## Play 编码 few-shot

以下是字段组合示例，不是可直接执行的完整参数。实际字段、枚举和额外约束以本次 `describe` 为准。

### 打者一垒安打

打者本身也是跑者，首次上垒从 `from_base=0` 开始；不填写球队名或 `scored`：

```json
{
  "batting_result": "single",
  "batter_jersey_number": 1,
  "pitcher_jersey_number": 17,
  "pitches": [{"result": "in_play"}],
  "runner_outcomes": [{"jersey_number": 1, "result": "advance", "from_base": 0, "to_base": 1}]
}
```

### 安打送回垒上跑者

得分要标注自责分与可选打点；责任投手省略时默认为本打席投手：

```json
{
  "batting_result": "double",
  "runner_outcomes": [
    {"jersey_number": 9, "result": "score", "from_base": 2, "earned": true, "rbi_batter_jersey_number": 1},
    {"jersey_number": 1, "result": "advance", "from_base": 0, "to_base": 2}
  ]
}
```

### 双杀

跑者封杀用 runner outcome；打者的滚地出局由 `batting_result` 推导。守备员保留各自助杀、刺杀事实：

```json
{
  "batting_result": "ground_out",
  "runner_outcomes": [
    {"jersey_number": 6, "result": "force_out", "from_base": 1}
  ],
  "fielding_outcomes": [
    {"jersey_number": 6, "position": "shortstop", "result": "assist"},
    {"jersey_number": 4, "position": "second_base", "result": "assist"},
    {"jersey_number": 3, "position": "first_base", "result": "putout"}
  ]
}
```

### 牺牲触击、牺牲飞球和本垒打

牺牲触击、牺牲飞球和阳春本垒打的打者出局或得分由打席结果和 `runner_outcomes` 推导。三种情况的最后一球使用 `in_play`；不要填写 `before`、`after` 或球数。
