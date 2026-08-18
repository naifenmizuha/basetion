# 比赛记录改造设计

## 状态与范围

本文是比赛记录改造的设计草案，讨论一场棒球比赛如何写入数据库，以及 `game` 领域提供哪些增删改查能力。分析领域、统计指标、模型工具协议和历史数据迁移方案不在本次范围内。

`GameRecord` 是整场比赛的输入和读取边界，包含 Match、双方阵容及全部 Play。`Play` 是可单独修正的最小记录单元，一条 Play 表示一次已经结束的打席，包含打席开始和结束时的场上局面、逐球过程、打击结果、跑者变化及守备贡献。首版不保存进行到一半的打席。

## 领域结构

```text
Match
  ├── Lineup[]
  └── Play[]
       ├── BeforeSituation
       ├── Pitch[]
       ├── BattingResult
       ├── PitchingResult
       ├── RunnerResult[]
       ├── FieldingResult[]
       └── AfterSituation
```

`GameRecord` 按这张结构一次接收或返回整场比赛。领域服务可以在内存中校验全部 Play 的顺序和局面连续性，再通过 UnitOfWork 写入。单条修正仍以 Play 为单位，不要求每次修改都加载整场比赛。

Match 的版本同时充当整场记录版本。阵容或 Play 发生变化时，Match 版本一并增加；调用方据此判断自己持有的整场快照是否过期。Lineup 和 Play 保留各自版本，用于局部修正时报告精确冲突。

### Match

`Match` 沿用现有职责，保存主客队、计划比赛时间、地点和比赛状态。比赛记录只能挂在有效 Match 下。`final` 表示记录工作已经完成，仍允许通过显式修正流程更正阵容或 Play；修正会同时增加子实体版本和 Match 的整场记录版本。

### Play

`Play` 是比赛过程的基本记录单元。ID、比赛内顺序、局数、上下半局、棒次、打者和开局投手构成它的基本身份。Play 自己持有一次打席的完整记录，并负责以下局部规则：

- sequence 为正数，在同一比赛内唯一。
- inning 为正数，half 只能是 top 或 bottom。
- 棒次在 1 至 9 之间。
- 至少有一条 Pitch，最后一球必须能够结束 BattingResult。
- BeforeSituation 的出局数为 0 至 2，AfterSituation 的出局数为 0 至 3。
- 打席产生的出局、得分和垒包变化必须能与前后 Situation 对上。
- RunnerResult 与 FieldingResult 中引用的球员不得为空，且不能出现重复结果 ID。

### Situation

Situation 是值对象，描述打席边界上的场上局面：

```text
outs
home_score
away_score
runner_on_first_id
runner_on_second_id
runner_on_third_id
```

BeforeSituation 保存打席开始前的事实，AfterSituation 保存这次打席所有动作完成后的事实。结束半局的 Play 可以保留 `outs=3`；下一条 Play 进入新的半局，并以 `outs=0` 和空垒开始。领域服务据此检查相邻 Play 是否衔接。

### Pitch

Pitch 保存 Play 内的投球顺序、投手、打者、投球结果及球数前后状态。首版支持以下结果：

```text
ball
called_strike
swinging_strike
foul
foul_tip
in_play
hit_by_pitch
intentional_ball
pitchout
other
```

球种、球速和进垒区域使用可空字段，缺少这些数据不影响比赛记录成立。投手和打者通常与 Play 一致；保留在 Pitch 上可以表达打席中途换投，也便于逐球记录独立校验。

### BattingResult

BattingResult 是 Play 上的值对象，只保存一次打席的最终结果，不保存 AVG、OPS 等统计值。结果类型包括安打、保送、三振、各类出局、野手选择、失误上垒、牺牲打、妨碍和 other。

文字说明用于展示和人工复核，不能覆盖结构化结果。两者发生冲突时，记录无效，不能提交。

### PitchingResult

PitchingResult 是从 Play 事实形成的领域视图，不单独持久化。面对打者、被安打、保送、三振和投球数都能由 Pitch 与 BattingResult 得到；失分责任由 RunnerResult 记录。避免单独存一份投球结果，可以防止打击结果和投球结果互相矛盾。

打席中途换投时，一个 Play 可以形成多个 PitchingResult。每名投手的投球数来自其 Pitch，打席结果归属及出局责任按领域规则分配。

### RunnerResult

RunnerResult 记录一名跑者在本 Play 中的一次结果：起始垒、结束垒、动作类型、是否出局、是否得分、责任投手和是否为自责分。一次 Play 可以有多条 RunnerResult。

首版动作类型包括推进、得分、封杀、触杀、盗垒失败、牵制出局和因失误推进。打点归属放在得分 RunnerResult 上，通过可空的 `rbi_batter_id` 表达；没有打点时为空。

### FieldingResult

FieldingResult 记录守备员、实际守位和一次守备动作。动作类型包括刺杀、助杀、失误、参与双杀或三杀、捕逸、捕手妨碍和 other。一记 6-4-3 双杀会产生多条 FieldingResult，各守备员的贡献分别保存。

FieldingResult 记录的是比赛事实，不保存守备率等汇总值。

## 数据库表示

一场比赛由 `matches` 中的一行、该比赛的 `lineups` 行，以及按 sequence 排列的 `plays` 和子记录组成。

```text
matches
  ├── lineups
  └── plays
       ├── pitches
       ├── play_runner_results
       └── play_fielding_results
```

PitchingResult 不设表。BattingResult 和前后 Situation 都是一条 Play 固有且固定数量的数据，直接存入 `plays`，避免为一对一值对象增加表和联接。

所有持久化领域数据都带 `version`、`created_at`、`updated_at` 和可空的 `deleted_at`。Repository 默认排除 `deleted_at IS NOT NULL` 的记录。表之间不使用 `ON DELETE CASCADE`；删除 Play 或 Match 时，由领域服务通过 UnitOfWork 显式编排软删除。

### plays

```sql
id UUID PK
match_id UUID
sequence INTEGER
inning SMALLINT
half SMALLINT
batting_order SMALLINT
batter_id UUID
starting_pitcher_id UUID

batting_result SMALLINT
result_description TEXT

before_outs SMALLINT
before_home_score SMALLINT
before_away_score SMALLINT
before_runner_on_first_id UUID NULL
before_runner_on_second_id UUID NULL
before_runner_on_third_id UUID NULL

after_outs SMALLINT
after_home_score SMALLINT
after_away_score SMALLINT
after_runner_on_first_id UUID NULL
after_runner_on_second_id UUID NULL
after_runner_on_third_id UUID NULL

version BIGINT
created_at TIMESTAMPTZ
updated_at TIMESTAMPTZ
deleted_at TIMESTAMPTZ NULL
```

活动记录需要 `(match_id, sequence)` 部分唯一索引。按比赛读取时使用相同字段排序。

### pitches

```text
id UUID PK
play_id UUID
sequence SMALLINT
pitcher_id UUID
batter_id UUID
result SMALLINT
balls_before SMALLINT
strikes_before SMALLINT
balls_after SMALLINT
strikes_after SMALLINT
pitch_type SMALLINT NULL
velocity NUMERIC NULL
zone SMALLINT NULL
description TEXT
version BIGINT
created_at TIMESTAMPTZ
updated_at TIMESTAMPTZ
deleted_at TIMESTAMPTZ NULL
```

活动记录需要 `(play_id, sequence)` 部分唯一索引。

### play_runner_results

```text
id UUID PK
play_id UUID
sequence SMALLINT
runner_id UUID
result SMALLINT
from_base SMALLINT
to_base SMALLINT NULL
out_recorded BOOLEAN
scored BOOLEAN
charged_pitcher_id UUID NULL
earned BOOLEAN NULL
rbi_batter_id UUID NULL
description TEXT
version BIGINT
created_at TIMESTAMPTZ
updated_at TIMESTAMPTZ
deleted_at TIMESTAMPTZ NULL
```

`charged_pitcher_id` 和 `earned` 只在得分需要归属投手时填写。活动记录需要 `(play_id, sequence)` 部分唯一索引。

### play_fielding_results

```text
id UUID PK
play_id UUID
sequence SMALLINT
fielder_id UUID
position SMALLINT
result SMALLINT
description TEXT
version BIGINT
created_at TIMESTAMPTZ
updated_at TIMESTAMPTZ
deleted_at TIMESTAMPTZ NULL
```

活动记录需要 `(play_id, sequence)` 部分唯一索引。

## 一场比赛的落库过程

调用方可以提交一个完整 GameRecord，其中包含 Match、双方阵容和按比赛顺序排列的 Play。每条 Play 包含：

1. BeforeSituation 取自当时的出局数、比分和垒上跑者。
2. Pitch 按实际顺序列出本次打席的每一球。
3. BattingResult 保存打席最终结果。
4. RunnerResult 逐项说明跑者推进、出局和得分。
5. FieldingResult 保存参与这次结果的守备动作。
6. AfterSituation 保存所有动作完成后的局面。

领域服务先校验两队身份、阵容、球员归属、Play 顺序和相邻局面，再在一个事务中写入整场记录。跨半局时按三出局、清空垒包和比分延续规则转换。任意记录失败都会使事务回滚，不留下半场比赛或缺少子记录的 Play。

逐次录入也使用同一套 Play 规则。创建 Match 和阵容后，调用方可以按 sequence 追加完整 Play；新 Play 的 BeforeSituation 必须与上一条有效 Play 的 AfterSituation 衔接。整场导入和逐条录入最终形成相同的数据库结构。

读取整场比赛时，Repository 返回 Match、双方阵容和按 sequence 排列的 Play。每条 Play 内的 Pitch、RunnerResult 与 FieldingResult 也按各自 sequence 排列。调用方不需要拼接无序事件。

## 领域层能力

### 整场比赛写能力

领域服务提供整场入口，工具只需一次调用就能创建或替换完整比赛记录：

```go
CreateGameRecord(ctx, GameRecordDraft) (GameRecord, error)
ReplaceGameRecord(ctx, MatchID, expectedVersion, GameRecordDraft) (GameRecord, error)
DeleteGameRecord(ctx, MatchID, expectedVersion) error
```

`GameRecordDraft` 包含 MatchDraft、LineupDraft 列表和按 sequence 排列的 PlayDraft 列表。CreateGameRecord 在一次 UnitOfWork 中创建全部数据。ReplaceGameRecord 用于整场重导入或大范围修正，按稳定 ID 对账现有记录：仍存在的实体更新，新增实体创建，草稿中缺失的实体软删除。Match ID 和参赛双方不能在 ReplaceGameRecord 中改变；需要替换比赛身份时应删除原记录并创建新记录。

整场写入执行完整校验，包括：

- 两支球队不同，阵容与 Play 中的球员属于参赛球队。
- Match 内的 Play sequence 连续且不重复。
- 每条 Play 的内部结果闭合。
- 相邻 Situation 连续，半局转换合法。
- 最后一条 Play 的比分与比赛状态一致。
- GameRecordDraft 标记为 final 时，不允许没有 Play 的空比赛。

整场入口不会逐项返回成功状态。它具备事务原子性，任何校验或持久化错误都会撤销本次写入。

### Match 写能力

沿用现有 Match 能力：

```go
CreateMatch(...)
UpdateMatch(...)
SetMatchStatus(...)
DeleteMatch(...)
```

这些接口用于比赛元数据和状态的小范围修改。删除 Match 时，UnitOfWork 显式软删除所属 Lineup、Play、Pitch、RunnerResult 和 FieldingResult。状态改为 final 时，领域服务检查比赛至少有一条有效 Play，并确认最后局面和比分合法。任何写操作都会增加 Match 的整场记录版本。

### Lineup 写能力

阵容能力维持现状：

```go
CreateLineup(...)
ReplaceLineup(...)
DeleteLineup(...)
```

Lineup 表示比赛登记阵容。Play 中出现的打者、投手、跑者和守备员必须属于本场一方球队；是否强制出现在 starter 或 backup 阵容中，由领域服务统一校验。创建、替换或删除阵容后，Match 的整场记录版本随事务增加。

### Play 写能力

比赛记录提供三个写入口：

```go
CreatePlay(ctx, PlayDraft) (Play, error)
ReplacePlay(ctx, PlayID, expectedVersion, PlayDraft) (Play, error)
DeletePlay(ctx, PlayID, expectedVersion) error
```

`PlayDraft` 包含完整的前后 Situation、Pitch、BattingResult、RunnerResult 和 FieldingResult。CreatePlay 校验比赛引用、球员归属、内部一致性及前一条 Play 的局面。ReplacePlay 用于修正已经录入的打席；它在同一事务中替换 Play 的值对象和子集合，并重新校验前后相邻 Play。子记录按 ID 对账：仍存在的记录更新版本，草稿中新出现的记录创建，草稿中缺失的原记录显式软删除。DeletePlay 软删除 Play 及其子记录，也要检查删除后剩余 sequence 和局面是否仍连续。三种操作都会增加 Match 的整场记录版本。

首版不开放独立的 `CreatePitch`、`UpdateRunnerResult` 等细粒度服务。子记录离开 Play 后没有完整业务语义，单独修改容易留下无法闭合的局面。调用方需要修正某一球或守备动作时，读取 Play、修改草稿，再通过 ReplacePlay 提交整个聚合。

### 查询能力

查询接口按返回数据量和调用目的划分，避免所有调用都加载逐球记录。

比赛目录只返回 Match 元数据和最终比分摘要，适合按球队、时间范围和状态查找比赛：

```go
ListMatches(ctx, MatchFilter) ([]MatchSummary, error)
GetMatch(ctx, MatchID) (Match, error)
```

单场概览返回 Match、双方阵容、Play 数量、局数范围和比分变化，不展开逐球、跑者及守备明细。它适合比赛浏览和确认记录完整度：

```go
GetGameOverview(ctx, MatchID) (GameOverview, error)
```

完整读取一次返回一场比赛的全部结构化记录，对应整场写入的 GameRecord：

```go
GetGameRecord(ctx, MatchID) (GameRecord, error)
```

局部查询用于修正某个打席，或者按局数和 sequence 查看一段比赛。`PlayFilter` 支持 inning、half、sequence 起止值，并可限定是否加载 Pitch、RunnerResult 和 FieldingResult：

```go
ListPlays(ctx, MatchID, PlayFilter) ([]Play, error)
GetPlay(ctx, PlayID) (Play, error)
```

接口划分依据当前已有的四类使用场景：比赛检索、单场浏览、整场导入导出、局部修正。分析领域以后可以直接读取 GameRecord，也可以增加面向批量计算的 Repository 投影；这不影响本设计中的公开查询语义。

Repository 还需要面向写服务的内部查询：按 ID 加锁读取 Play、取得前后相邻 Play，以及检查比赛内 sequence 是否占用。这些端口不作为公开查询用例暴露。

## 事务、并发与错误

整场写入及 Play 的创建、替换和删除都在 UnitOfWork 中完成。Match 版本保护整场快照，Play 版本保护局部记录；任一预期版本不匹配都返回明确的版本冲突错误。相邻局面校验所依赖的 Play 要在事务中加锁，防止两个调用同时向同一比赛位置写入。

领域错误至少区分以下情况：

```text
match not found
game record version conflict
play not found
play version conflict
play sequence conflict
invalid play situation
discontinuous play situation
invalid pitch sequence
inconsistent batting result
invalid runner result
invalid fielding result
player does not belong to match team
```

数据库恢复出的非法记录映射为 corrupted Play 错误，不能把部分数据当作有效比赛返回。

## 暂不处理的内容

- 按比赛或时间范围计算打击、投球和守备指标。
- 分析结果缓存和统计快照。
- 未完成打席的实时保存与恢复。
- 自动判定自责分。
- 完整跑垒规则、改判和申诉流程。
- 已有比赛记录的数据迁移策略。
- Agent Skill、`team_query` 与 `team_modify` 协议调整。

这些需求会读取本设计提供的结构化比赛事实，但不改变 Play 作为完整打席记录的边界。
