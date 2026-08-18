# Lua 内部引用与比赛查询上下文设计

## 背景与目标

`team_query` 当前以 JSON 可序列化的 Lua table 暴露领域对象。对象的关联完全依赖字符串 UUID：比赛返回 `id`、`home_team_id`、`away_team_id`，名单返回 `team_id`、`player_id`，打席和阵容继续以 `match_id`、`player_id`、`fielder_id` 等字段关联。

这使模型面对复杂但低频的自由分析时，需要在 Lua 程序中反复执行“查询比赛列表 → 找到目标比赛 UUID → 查询比赛过程 → 以球队 UUID 查询两队名单 → 构建 UUID 到姓名的映射”。如果中间结果被 `main` 返回，UUID、完整阵容和完整名单会进入下一轮模型上下文；如果模型不能从契约推导枚举含义，还会再次读取同一比赛的原始打席样例。

`log/test.log` 中“复盘曹魏队最近一场比赛”共有四次成功或尝试的 `query`：重复读取同一场 54 个打席，三次重新定位比赛，至少两次重新读取双方名单，且向模型返回 40 人 UUID 映射、四套阵容及其条目 UUID。最终回答只使用少量姓名、比分、打席聚合和关键得分事件。第一次结构探测还因返回稀疏 Lua array 失败，导致额外一次完整重试。

本设计的目标不是用固定领域报表取代 Lua。高频、口径稳定的问题仍可单独提供领域聚合；低频和探索性分析继续由 Lua 自由组合。此次改造为自由 Lua 提供内部关联能力和更小的结果边界，使稳定标识只在 Lua VM 内部流转，尽量不进入模型上下文。

本次范围：

- 引入不可序列化的 Lua 内部实体引用，用于比赛、球队和球员之间的跨 topic 关联。
- 为比赛原始查询提供按名称定位单场的选择器，并提供按需读取的单场比赛工作上下文。
- 使原始比赛事件中的投球、跑垒和守备枚举以语义化名称输出；同时公开必要的数据归因限制。
- 收紧 Lua 结果转换的内部引用与 UUID 泄漏处理，并改进稀疏 table 报错定位。
- 更新 Team Query topic、Skill 与测试，确保 Agent 优先一次 Lua 调用完成可确定的组合查询。

不在本次范围：

- 删除既有 `*_id` 输入参数或立刻破坏已有 Lua 脚本。
- 在此改造中实现完整的 `game.team_performance` 等高频领域报表；此类聚合可在内部引用协议稳定后单独设计。
- 将所有数据库实体都改为可由名称唯一定位；名称重名仍需由 Lua 投影可区分业务信息，并由模型请求用户确认。
- 对底层 UUID 存储、写入操作、事务、乐观锁和软删除规则作出修改。

## 设计原则

1. 稳定标识是基础设施和 Lua 内部关联机制，不是模型推理所需的业务事实。
2. Lua 保留自由组合、循环、聚合和跨模块分析能力；领域层仅固化稳定的筛选、导航、上下文装配和业务枚举解释。
3. 可下推的名称、日期、状态、位置、排序和单条选择必须在 topic 参数或领域 Filter 中完成，不应由 Lua 先拉宽范围再处理。
4. `main` 的返回值只能包含业务字段、显式投影的统计和数据限制说明；内部引用不能序列化。
5. 兼容既有 UUID 参数，但新示例和 Skill 必须优先使用内部引用或名称选择器。

## 目标行为

### 内部引用

Lua 读取对象时，公开业务字段和一个只供程序使用的引用字段：

```lua
local matches = team.game.matches({
  team_name = "曹魏队",
  status = "final",
})

local latest = matches[#matches]
-- latest.id 不再是默认公开字段。
-- latest.ref 是不可序列化的 MatchRef。
local plays = team.game.plays({match = latest.ref})
```

引用类型包括：

- `MatchRef`：封装 `game.MatchID`。
- `TeamRef`：封装 `team.ID`。
- `PlayerRef`：封装 `player.ID`。

内部引用是 GopherLua userdata，具有以下行为：

- 只能由 Team Query Lua 适配层创建，不能从字符串或 Lua table 构造。
- 只能作为同类型或明确兼容类型的 topic 参数传入。
- 不提供 `__tostring`、可写字段、可枚举内部值或可取回 UUID 的元方法。
- 可使用 identity 比较来判断“是否为同一引用”，但不得与字符串比较。
- `main` 返回值中出现任一内部引用时，结果转换器返回带字段路径的错误，例如 `lua result contains internal reference at $.match.ref`。
- 内部引用不属于模型可见的 `describe` 返回字段；topic 文档只说明某个参数可接受“由上游查询返回的内部引用”。

对象公开形态如下：

```lua
-- game.matches 的单项
{
  ref = <MatchRef>,
  home = {name = "蜀汉队", ref = <TeamRef>},
  away = {name = "曹魏队", ref = <TeamRef>},
  scheduled_at = "2026-08-16T19:00:00+08:00",
  location = "成都棒球场",
  status = "final",
}

-- roster.players 的单项
{
  ref = <PlayerRef>,
  name = "曹操",
  team = {name = "曹魏队", ref = <TeamRef>},
  jersey_number = 1,
  positions = {"first_base"},
  active = true,
}
```

名单关系自身的 `membership_id` 不进入默认 Lua 对象。若未来必须支持按某段名单关系修改，另行引入 `MembershipRef` 并只接受该引用的写操作；不能回退为默认输出 UUID。

### 兼容与参数优先级

既有字符串参数保留，但所有相关 topic 增加引用参数：

| Topic | 新参数 | 兼容参数 | 校验规则 |
| --- | --- | --- | --- |
| `game.match` | `match`、`team_name + selection` | `match_id` | 三种定位方式恰好提供一种。 |
| `game.plays` | `match` | `match_id` | 恰好提供一种。 |
| `game.score` | `match` | `match_id` | 恰好提供一种。 |
| `lineup.list` | `match`、`team` | `match_id`、`team_id` | `match`/`match_id` 恰好一项；`team`/`team_id` 至多一项。 |
| `roster.players` | `team` | `team_id`、`team_name`、`player_name` | `team` 与 `team_id`、`team_name` 互斥；未使用引用时保持现有至少一个定位条件的规则。 |
| `training.records` | `player` | `player_id` | 恰好提供一种。 |

Lua 内部引用优先于字符串 ID；一旦传入引用，适配层不再接受同时提供的对应 UUID 或名称，以避免同一调用表达两个冲突目标。

### 单场比赛选择器

为避免每段 Lua 重复“查列表、按时间排序、取最新完赛”的样板逻辑，扩展 `game.match`：

```lua
local match = team.game.match({
  team_name = "曹魏队",
  selection = "latest_final",
})
```

`selection` 初始可选值：

- `latest`：按 `scheduled_at DESC, id DESC` 的第一场未删除比赛。
- `latest_final`：按相同顺序的最新 `final` 比赛。
- `latest_in_progress`：按相同顺序的最新 `in_progress` 比赛。
- `earliest`：按 `scheduled_at ASC, id ASC` 的第一场未删除比赛。

`game.match` 名称选择器命中零场时返回领域的 `ErrMatchNotFound`；不返回空 table。名称不唯一时不以球队名称本身消歧：所有同名球队参与的比赛都在排序集合中，结果按统一顺序选择；若该语义不能满足调用方，调用方应使用 `game.matches` 返回业务摘要，由用户确认后继续。

领域层在 `MatchFilter` 上增加排序和限制表达，或新增专用 `FindMatch` Filter；Repository 必须以 SQL `ORDER BY ... LIMIT 1` 完成选择，不能先读取比赛列表再在 Lua 排序。

### 比赛工作上下文

新增 `team.game.context`，用于围绕一场比赛进行任意自由分析：

```lua
function main(team)
  local game = team.game.context({
    team_name = "曹魏队",
    selection = "latest_final",
  })

  local plays = game.plays()
  local caowei_players = game.team_players()
  local opponent_players = game.opponent_players()

  -- 自由进行统计、关联和投影。
  return {
    match = {
      home = game.match.home.name,
      away = game.match.away.name,
      score = game.score,
    },
  }
end
```

`context` 的创建只读取：比赛摘要、双方 `TeamRef` 和比分。它不默认读取完整打席、名单或阵容。上下文对象是不可序列化 userdata，其公开方法按需读取并在本次 Lua 执行内缓存：

- `context.plays()`：首次调用以 `MatchRef` 读取完整比赛过程，后续调用返回同一只读数组。
- `context.lineups([teamRef])`：首次以 `MatchRef`，可选 `TeamRef` 读取阵容并缓存；不调用时不读取。
- `context.team_players()`：以目标球队 `TeamRef` 读取名单并缓存。
- `context.opponent_players()`：以对手球队 `TeamRef` 读取名单并缓存。
- `context.players()`：按确定的主客两队读取并合并名单；仅在确实需要跨队姓名映射时调用。
- `context.match`：只读业务摘要，含主客队名称和内部引用。
- `context.score`：以目标球队视角提供 `{team, opponent, final, known}`，避免调用方自行判断主客队。
- `context.team`、`context.opponent`：只读 `{name, ref}`。

上下文没有跨 `team_query` 调用的持久化能力，不改变 Session、数据库连接或缓存生命周期；每次 `query` 都建立新的 Lua VM 和新的上下文缓存。

`game.context` 是查询导航能力，不是表现分析报表。投打守聚合、过滤维度、排序、阈值和最终结构依然由 Lua 编写。高频需求例如 `game.team_performance` 可在后续以相同 `MatchRef`/`TeamRef` 契约提供领域聚合。

### 语义化原始比赛事件

当前 `playToLua` 对 `PitchResult`、`RunnerResult`、`FieldingResult` 输出整数，迫使模型读取样例并猜测 `1`、`2`、`3`、`6` 的含义。改为输出稳定字符串：

```lua
{
  batting_result = "home_run",
  pitches = {{result = "in_play", pitcher = <PlayerRef>, batter = <PlayerRef>}},
  runner_results = {{runner = <PlayerRef>, result = "scored", scored = true}},
  fielding_results = {{fielder = <PlayerRef>, result = "putout"}},
}
```

字符串值由 `domain/game` 导出的枚举名称函数定义，不能由 Lua 适配层单独维护映射。领域层至少增加：

- `PitchResult.Name() string`
- `RunnerResult.Name() string`
- `FieldingResult.Name() string`
- `Half.Name() string`
- 已有 `BattingResult` 同样改为导出 `Name() string`

各 `Name` 在枚举无效时返回空字符串；Lua 适配层遇到空名称视为损坏领域数据并报错，而不是继续输出数字。

原始 `Play` 输出默认去除 `id`、`match_id`、`batter_id`、`starting_pitcher_id`、`fielder_id`、`runner_id` 等 UUID 字符串，以 `ref` 替代需要继续关联的字段。业务字段仍保留：局次、上下半、棒次、打击结果、文字描述、前后比分、出局数和语义化子结果。

为防止模型误把不完整守备归因当作完整个人防守统计，`game.context` 以及 `game.plays` 的调用说明新增 `data_limits`：

```lua
{
  fielding_attribution = "recorded_outcome_only",
  fielding_individual_evaluation_supported = false,
}
```

这只陈述当前记录模型：守备子结果只表示记录到的野手结果，并不保证覆盖该打席所有参与野手，不能直接推导完整个人守备机会或防守范围。

### 结果投影与错误语义

`converter` 仍只转换 JSON 兼容的 nil、boolean、number、string、array 和 object。变更如下：

1. 转换函数增加 JSONPath 风格路径参数。遇到内部引用、函数、线程、未知 userdata、循环、过深层级或非法 key 时，错误附带路径。
2. `nullValue` 是唯一允许从 userdata 转为 JSON null 的类型；`MatchRef`、`TeamRef`、`PlayerRef` 和 `GameContext` 都报“internal reference/context cannot be returned”。
3. 结果对象不会按字段名猜测 UUID 并隐式删除。这样不会篡改用户明确要求的业务文本，也不依赖脆弱的命名规则。UUID 不泄漏由默认对象模型和不可序列化引用保证。
4. 稀疏整数 key 不再只报笼统的 `lua result contains a sparse array`；错误应指出 table 路径、元素数量、最大索引和缺失索引范围，例如 `lua result at $.play_summary is sparse: 12 elements, maximum index 13, missing index 8`。
5. `team.array()` 仍是返回空数组的唯一显式标记。map 必须使用字符串 key；统计聚合结果应采用名称或语义字符串作为 key，不能使用 UUID key。

### 面向模型的最小投影示例

使用新能力后的自由复盘只需一次查询：

```lua
function main(team)
  local game = team.game.context({
    team_name = "曹魏队",
    selection = "latest_final",
  })
  local plays = game.plays()
  local players = {}
  for _, value in ipairs(game.players()) do
    players[value.ref] = value.name
  end

  local batting = {}
  local pitching = {}
  for _, play in ipairs(plays) do
    if play.half == "top" then
      local value = batting[play.batter.ref] or {name = players[play.batter.ref], pa = 0, home_runs = 0}
      value.pa = value.pa + 1
      if play.batting_result == "home_run" then value.home_runs = value.home_runs + 1 end
      batting[play.batter.ref] = value
    else
      local value = pitching[play.starting_pitcher.ref] or {name = players[play.starting_pitcher.ref], faced = 0, home_runs_allowed = 0}
      value.faced = value.faced + 1
      if play.batting_result == "home_run" then value.home_runs_allowed = value.home_runs_allowed + 1 end
      pitching[play.starting_pitcher.ref] = value
    end
  end

  return {
    match = {
      team = game.team.name,
      opponent = game.opponent.name,
      score = game.score,
    },
    batting = batting,
    pitching = pitching,
    data_limits = game.data_limits,
  }
end
```

示例中 `PlayerRef` 作为 Lua map 的内部 key 是允许的，但 `main` 直接返回 `batting`/`pitching` map 会包含 userdata key，结果转换器应拒绝。因此实际 Skill 示例必须在返回前将统计整理为数组，只投影姓名和数值：

```lua
local batting_rows = team.array()
for _, value in pairs(batting) do batting_rows[#batting_rows + 1] = value end
return {batting = batting_rows}
```

这条限制应在 runtime topic 和 Skill 中明确写出，避免引用作为结果 key 造成新的序列化失败。

## 分层设计

### 领域层

`src/internal/domain/game` 保持实体用 UUID 值对象建立关联；不引用 Lua、userdata 或 JSON。新增内容：

- 对 `MatchID`、`team.ID`、`player.ID` 的内部引用不进入领域模型；它们是 Lua 基础设施对领域 ID 的封装。
- `QueryService` 增加按 `team_name + selection` 获取单场比赛的查询方法，例如 `FindMatchSummary(ctx, MatchSelection)`。选择器、日期排序、完赛状态和无结果错误属于比赛查询语义。
- 新增 `GameContextSnapshot` 或等价的只读查询值对象，包含 `MatchSummary`、目标球队与对手的业务名称/ID、比分及数据限制。它不包含完整打席、名单或阵容。
- `QueryRepository` 新增一次性读取上下文所需的端口。PostgreSQL 用一次带双方球队 JOIN 的比赛查询和必要比分来源完成；不得在 Lua 基础设施中组合多个 Repository 查询来伪造领域上下文。
- `play.go` 的枚举类型增加导出 `Name()` 方法。其测试覆盖每个有效枚举、无效值和 Lua 输出字符串。

`game.team_performance` 不在本次实现，但后续若实现，应接收同样的 `MatchSelection` 或 `MatchID`、`team.ID` 输入，返回姓名化和带数据限制的统计值对象，不暴露 Lua 内部引用。

### 基础设施层

`src/internal/infra/postgres/game_repositories.go`：

- 实现单场选择 SQL，使用 `MatchFilter` 的名称过滤、`status`、`ORDER BY scheduled_at,id` 和 `LIMIT 1`。
- 实现上下文快照 Repository 查询，确保主客队名称、目标队身份和比分在数据库/领域查询层确定。
- 保持所有查询排除 `matches.deleted_at` 与关联球队的 `deleted_at`；不引入物理删除。

`src/internal/infra/teamquery/lua_executor.go`：

- 新增私有 `matchRef`、`teamRef`、`playerRef`、`gameContext` userdata payload 类型。
- 在 `matchSummaryToLua`、`rosterPlayerToLua`、`playToLua`、`lineupToLua` 中将 ID 字符串替换为对应引用，公开业务名称和必要业务字段。
- 新增统一的 `parseMatchTarget`、`parseTeamTarget`、`parsePlayerTarget` 辅助函数，集中处理引用与兼容 UUID 参数的互斥、类型校验和错误信息。
- 新增 `game.context` proxy；其闭包持有本次执行 `context.Context`、只读领域服务、上下文快照和按需缓存。缓存仅存在于当前 Lua VM，不能跨执行复用。
- 更新 `game.match`、`game.plays`、`game.score`、`lineup.list`、`roster.players`、`training.records` 以接受引用参数。
- 更新 converter，递归携带结果路径并拒绝内部引用、上下文和 userdata key；改进稀疏数组错误。

`src/internal/domain/teamquery/service.go`：

- 在 `game` 子 topic 中增加 `game.context`。
- 更新相关 topic 的参数说明，标明可接受“上游 topic 返回的内部引用”，以及引用不可返回。
- 更新 returns，仅列模型可投影的业务字段；引用不是 returns 字段。
- 增加原始 `game.plays` 的语义化枚举值列表与 `data_limits` 描述。
- runtime `ExecutionTip` 说明：同一 Lua 内优先将上游对象的内部引用传给后续 topic；只将名称和聚合结果放入 `main` 返回值。

### Skill

`skills/manage-team/SKILL.md` 更新为：

1. 对“最近一场”“指定球队单场”的分析，优先使用 `game.context` 或 `game.match` 的名称选择器，不得先查询列表、Lua 排序并手工传递 UUID。
2. 上游 topic 返回的引用只可在同一 Lua 执行内传给下游 topic；不得放入 `main` 返回值、字符串化或作为返回对象 key。
3. `game.plays` 只在需要逐打席事实的自由分析中调用，并在同一 Lua 内完成聚合；不得先返回 sample play、完整 UUID 映射或完整原始阵容以探索枚举含义。
4. 守备、跑垒、投球枚举使用 topic 声明的语义字符串；分析必须保留 `data_limits` 中声明的数据归因限制。
5. 若一次 Lua 的中间结论不需要模型语义判断，必须一次调用完成。仅当用户的后续选择、同名实体消歧或工具返回的真实歧义会改变查询目标时，才允许第二次 `team_query`。

## 文件级修改清单

| 路径 | 具体落点 | 修改内容、原因与依赖关系 |
| --- | --- | --- |
| `src/internal/domain/game/play.go` | `PitchResult`、`RunnerResult`、`FieldingResult`、`BattingResult`、`Half` | 新增 `Name()` 方法并测试完整映射。领域层拥有枚举语义，Lua 不再自行维护数字映射。 |
| `src/internal/domain/game/query.go` | 单场选择类型、上下文快照值类型、`QueryRepository`、`QueryService` | 定义 `latest_final` 等选择规则和比赛上下文只读查询契约。只依赖领域值对象与端口。 |
| `src/internal/domain/game/query_test.go` | 选择器与上下文服务测试 | 验证排序、状态过滤、无匹配错误、目标球队/对手判定和错误传播。 |
| `src/internal/infra/postgres/game_repositories.go` | 比赛选择和上下文快照 SQL | 通过 SQL 过滤、排序和 `LIMIT 1` 取得单场；一次 JOIN 恢复双方业务名称，继续排除软删除。 |
| `src/internal/infra/postgres/store_integration_test.go` | 比赛选择、上下文、软删除集成测试 | 验证最新完赛、目标队为主/客的比分视角、名称重名排序语义和已软删除数据排除。 |
| `src/internal/infra/teamquery/lua_executor.go` | 内部 ref userdata、目标解析、各 proxy、`game.context`、Lua table 转换、converter | 实现内部引用流转、懒加载上下文、兼容 UUID 输入、无 UUID 默认输出、语义化原始事件和可定位的序列化错误。 |
| `src/internal/infra/teamquery/game_test.go` | 引用组合、选择器、上下文缓存与枚举序列化测试 | 验证一次 Lua 用 `MatchRef` 和 `TeamRef` 串联比赛/名单/打席；验证重复 `context.plays()` 不重复调用服务；验证引用返回被拒绝。 |
| `src/internal/infra/teamquery/roster_test.go` | `TeamRef` 筛选和 `PlayerRef` 输出测试 | 验证引用类型、与 UUID/名称参数的互斥以及名单业务投影。 |
| `src/internal/infra/teamquery/lua_executor_test.go` | converter 路径与稀疏数组测试 | 新增 userdata 返回、userdata key、深层错误路径、稀疏数组缺失索引和 `team.array()` 空数组测试。 |
| `src/internal/domain/teamquery/service.go` | `game.context` topic、相关 topic 参数与返回字段、runtime 提示 | 对外发布选择器、内部引用可作参数但不可返回、语义化枚举和数据限制契约。 |
| `src/internal/domain/teamquery/service_test.go` | topic 描述断言 | 覆盖新增 topic、children 数量、参数、返回字段、示例和可用性。 |
| `skills/manage-team/SKILL.md` | 查询编排、投影约束、比赛示例 | 引导 Agent 使用 context/ref，禁止将 UUID、内部引用和原始样例作为中间结果返回。 |
| `skills/project-knowledge/SKILL.md` | 当前能力说明 | 简要说明球队查询支持同次 Lua 执行内的内部关联和单场上下文；不复制参数细节。 |
| `src/internal/harness/skills_test.go` | Skill 关键约束断言 | 确保 Skill 不退化回 UUID 中转或多轮原始读取。 |
| `doc/overview/game-domain.md`、`doc/overview/teamquery-domain.md`、`doc/overview/harness.md` | 提交时的现状文档 | 仅在用户要求创建提交时，更新领域查询、Lua 内部引用、上下文缓存与结果边界的长期事实。 |

## 错误、兼容性与安全

- 引用参数类型错误时，Lua 报错必须包含 topic、参数和期望类型，例如 `game.plays match must be a MatchRef`。
- 同时提供引用与对应 UUID/名称时返回参数冲突错误；不允许静默优先任意字段。
- 旧 Lua 继续可使用 `match_id`、`team_id`、`player_id`；其参数兼容性不变。默认返回对象移除 UUID 是一个可见协议变化，因此先通过本次所有查询 topic 的 `describe` 与 Skill 同步发布，且不提供“默认泄漏 ID”的兼容开关。
- 对确实需要后续 `team_modify` 的查询，后续设计应让 `team_modify` 也接受对应内部引用，或在同一次受控流程中生成操作目标；不得为迁就写入重启默认 UUID 输出。
- 内部引用和上下文不得跨 `query` 调用、跨 Session、跨进程或存入任何持久化存储。
- Lua userdata 的元表设置 `__metatable=false`，禁止脚本读取或替换内部实现；Lua 运行时仍保持只读数据、无文件系统、无网络和无 shell 能力。
- 领域数据的默认读取继续排除软删除；上下文缓存只是本次读操作结果，不改变软删除、事务或并发语义。

## 验证方案

实现完成后执行：

1. 领域层测试：覆盖所有比赛枚举名称、单场选择器排序/状态、上下文快照的目标队与比分视角。
2. PostgreSQL 集成测试：覆盖 SQL `LIMIT 1` 选择、两队名称、软删除排除和上下文快照。
3. Lua 适配测试：一段 Lua 使用 `game.context`、`MatchRef`、`TeamRef`、`PlayerRef` 完成比赛—名单—打席组合，最终仅返回名称和统计；断言服务调用次数不会因同一 context 重复方法调用而增加。
4. 结果边界测试：返回 ref、context、userdata key 和稀疏 array 时均得到包含路径的错误；返回业务投影和 `team.array()` 空数组仍可成功编码。
5. Team Query `describe` 测试：确认 `game.context`、选择器、引用参数说明、语义化枚举和数据限制与真实 Lua 协议一致。
6. 运行 `go test ./...`、`go vet ./...`、架构测试，以及一次实际“复盘曹魏队最近一场比赛”的端到端 CLI 场景。验收标准是只需一次 `describe` 加一次 `query`，查询结果不含 UUID、原始阵容或全量名单映射，且模型无需额外读取样例解释枚举。
7. 若用户后续要求提交，在提交前更新受影响 overview，并按仓库规范新建且仅新建一份 `doc/changes/` 记录；本设计文档不替代变更记录。
