# 按业务名称筛选球员与比赛设计

## 背景与目标

当前球队查询工具需要先读取球队或名单的完整结果，再从结果中取得 UUID 作为下一次查询参数：

- `roster.players` 强制要求 `team_id`。已知“蜀汉队的刘备”时，调用方仍要先读取 `roster.teams` 取得球队 UUID，再读取全队名单，并在 Lua 中按球员姓名筛选。
- `game.matches` 的领域查询端口不接收过滤条件。`src/internal/domain/game/query.go` 的 `QueryService.ListMatches` 先通过 `QueryRepository.ListMatches` 读取所有未删除比赛，随后才在内存按球队 UUID、日期和状态过滤。
- 比赛 Lua 返回值只有主客队 UUID。即使已筛选出目标比赛，模型在面向用户说明“谁对谁”时也可能再读取球队列表完成 UUID 到名称的转换。

这会产生与用户问题无关的数据读取、无意义 UUID 暴露和额外的工具调用。本次改造在既有领域只读查询边界中加入球队名称和球员姓名的精确匹配条件，并将全部可下推的筛选交给 PostgreSQL 执行；同时让 Skill 明确优先使用业务名称、限制 UUID 的读取与返回。

本次范围：

- 为名单球员查询增加 `team_name`、`player_name` 条件。
- 为比赛列表查询增加 `team_name` 条件，并将球队、日期、状态筛选下推到 Repository。
- 在 Lua 查询协议、渐进式 topic 描述和 Skill 中公开名称参数及 UUID 使用限制。
- 在名单和比赛 Lua 返回结果中提供必要的球队业务名称，减少为展示名称而追加的 UUID 关联查询。
- 添加数据库索引和自动化测试，覆盖名称筛选、软删除排除、参数冲突和旧 UUID 参数兼容性。

不在本次范围：

- 引入模糊搜索、前缀搜索、拼音搜索或大小写无关搜索。
- 将球队名或球员名设为唯一约束；同名实体必须保留为多个查询结果。
- 修改 `game.match`、`game.plays`、`game.score`、`lineup.list`、`training.records` 等仍以稳定 ID 定位单个实体或关联数据的协议。
- 为名称查询新增 application service、application 子包或平行的查询工具。
- 修改领域写入、事务、乐观锁或软删除规则。

## 目标行为

### 名称匹配语义

`team_name` 与 `player_name` 均采用精确、区分大小写的 PostgreSQL 文本相等匹配。调用方应传递业务系统中保存的完整名称；不会对输入做 `%` 拼接、`ILIKE`、模糊匹配或隐式标准化。

精确匹配的理由是查询边界可预测、便于使用 B-tree 索引，且不会因短名称读取大批无关领域数据。若未来确实需要模糊搜索，应新增含义明确的独立参数，例如 `team_name_contains` 或 `player_name_contains`，不能改变本次 `*_name` 的等值匹配语义。

名称不是稳定唯一标识：

- 同名球队会命中其各自的名单和比赛。
- 同名球员会命中其全部符合日期、位置和球队条件的名单关系。
- 当多个结果仍无法满足单实体读取、修改或跨模块关联需要时，调用方只能从本次已缩小范围的结果中使用对应稳定 ID；不得猜测哪个同名结果正确。

### 名单球员查询

`team.roster.players` 支持下列参数：

```lua
team.roster.players({
  team_id = "...",        -- 可选，与 team_name 互斥
  team_name = "蜀汉队",    -- 可选，与 team_id 互斥
  player_name = "刘备",    -- 可选
  on_date = "2026-08-18", -- 可选
  position_any = {"pitcher"}, -- 可选
})
```

领域过滤器要求 `team_id`、`team_name`、`player_name` 至少有一项非空；`team_id` 和 `team_name` 同时提供时返回参数错误。这样既支持“查询蜀汉队名单”，也支持“查找叫刘备的球员在哪些球队效力过”，同时不允许无范围读取全部名单。

日期和守备位置规则保持不变：`on_date` 缺省时使用当前日期判断效力期；`position_any` 命中球员任一守备位置。`memberships`、`teams`、`players` 三类记录均继续排除 `deleted_at IS NOT NULL` 的软删除数据。

Lua 返回的名单项在已有 `membership_id`、`team_id`、`player_id`、`name` 等字段基础上增加 `team_name`。稳定 ID 继续供下游需要时使用，但普通回答必须只投影姓名、球队名、球衣号、位置和所需业务事实。

### 比赛列表查询

`team.game.matches` 支持下列参数：

```lua
team.game.matches({
  team_id = "...",          -- 可选，与 team_name 互斥
  team_name = "蜀汉队",      -- 可选，与 team_id 互斥
  date_from = "2026-08-01", -- 可选，YYYY-MM-DD 或 RFC3339
  date_to = "2026-08-31",   -- 可选，YYYY-MM-DD 或 RFC3339
  status = "final",         -- 可选
})
```

`team_name` 匹配主队名称或客队名称；可与日期区间、状态组合。`team_id` 与 `team_name` 同时提供时返回参数错误。两者均未提供时仍允许按日期或状态检索比赛，保留当前比赛时间和状态筛选能力；完全空过滤器仍表示读取全部未软删除比赛，与已有协议兼容。

比赛 Lua 返回值在现有 `id`、`home_team_id`、`away_team_id`、`scheduled_at`、`location`、`status` 基础上增加：

- `home_team_name`
- `away_team_name`

这两个字段只用于业务展示和 Lua 投影，不改变 `game.Match` 聚合中通过 `HomeTeamID`、`AwayTeamID` 保存关联的既有模型。软删除的比赛、主队和客队均不能出现在结果中。

## 分层设计

### 领域层

`src/internal/domain/roster/query.go` 的 `PlayerFilter` 增加：

```go
type PlayerFilter struct {
    TeamID      team.ID
    TeamName    string
    PlayerName  string
    OnDate      *Date
    PositionAny player.PositionFlags
}
```

`QueryService.ListPlayers` 负责规范化和校验名称查询边界：空白名称按未提供处理；`TeamID`、`TeamName` 同时非空时报错；三项定位条件均为空时报错。Repository 只接收已经通过领域规则校验的 Filter，不承担协议层的参数解释。

`src/internal/domain/game/query.go` 的 `MatchFilter` 增加 `TeamName string`。`QueryRepository.ListMatches` 改为接受 `MatchFilter`：

```go
type QueryRepository interface {
    ListMatches(context.Context, MatchFilter) ([]Match, error)
    GetMatch(context.Context, MatchID) (Match, error)
    ListLineups(context.Context, MatchID) ([]Lineup, error)
    ListPlays(context.Context, MatchID) ([]Play, error)
}
```

`QueryService.ListMatches` 校验状态、日期上下界以及 `TeamID`/`TeamName` 互斥后，直接将 Filter 传给 Repository；删除当前先读取全部 Match 再逐项过滤的循环。`Match` 仍是纯领域聚合，不添加球队名称字段。

比赛列表需要名称输出时，基础设施不会把 PostgreSQL 行结构泄露给 `game.Match`。为保持现有 `QueryRepository` 契约简单，Lua 适配层在获得每个 `Match` 后使用已注入的球队只读服务按 ID 获取名称；但这会为 N 场比赛增加查询。因此本次实际采用领域查询投影：在 `src/internal/domain/game/query.go` 新增只读 `MatchSummary` 值类型和 `ListMatchSummaries` 查询端口，由它承载 `Match` 与 `HomeTeamName`、`AwayTeamName`。`ListMatches` 保留为对需要聚合的领域调用方返回 `[]Match` 的兼容入口，`ListMatchSummaries` 供 team query 使用。

新增端口形态如下：

```go
type MatchSummary struct {
    Match        Match
    HomeTeamName string
    AwayTeamName string
}

type QueryRepository interface {
    ListMatches(context.Context, MatchFilter) ([]Match, error)
    ListMatchSummaries(context.Context, MatchFilter) ([]MatchSummary, error)
    // 其余既有方法不变
}
```

`QueryService.ListMatchSummaries` 与 `ListMatches` 共用相同的 Filter 校验。这样名称投影属于比赛读取领域的明确契约，基础设施可以使用一次 JOIN 查询，Lua 层只做对象到 table 的转换。

### 基础设施层

`src/internal/infra/postgres/roster_reader.go` 的 `RosterReader.ListPlayers` 继续从 `memberships` 联结 `teams`、`players` 并恢复 `roster.Member`，但将固定的 `m.team_id=$1` 改为可选筛选：

- `TeamID` 非空时以 `m.team_id` 精确过滤。
- `TeamName` 非空时以 `t.name` 精确过滤。
- `PlayerName` 非空时以 `p.name` 精确过滤。
- 保留日期、位置和三个 `deleted_at IS NULL` 条件。
- 保持稳定排序 `m.jersey_number,p.name,p.id`。

`src/internal/infra/postgres/game_repositories.go` 的 `MatchRepository.List` 改为接收 `game.MatchFilter` 并在 SQL 中完成 `TeamID`、日期、状态条件。新增 `MatchRepository.ListSummaries`：

- 联结 `teams home_team` 和 `teams away_team`；
- 在 `home_team.name` 或 `away_team.name` 上实现 `TeamName` 条件；
- 在 `matches`、两个球队表上均排除软删除记录；
- 一次扫描恢复 `game.Match` 并构造 `game.MatchSummary`；
- 按现有 `scheduled_at,id` 规则稳定排序。

`Store` 对 `game.QueryRepository` 的转发方法同步接收过滤器并新增 `ListMatchSummaries`，使 bootstrap 注入方式维持不变。

新增一份顺序靠后的 SQL 迁移，在未软删除记录上创建名称等值筛选索引：

```sql
CREATE INDEX idx_teams_active_name ON teams(name) WHERE deleted_at IS NULL;
CREATE INDEX idx_players_active_name ON players(name) WHERE deleted_at IS NULL;
```

不添加唯一约束；不为这次名称查询物理删除任何数据。比赛名称筛选通过已有主客队 UUID 关联和球队名称索引执行；如集成测试中的 `EXPLAIN` 或实际数据量表明关联成为瓶颈，再另行设计针对 `matches` 的组合索引，而不是在本次预先加入未经验证的索引。

### Team Query 与 Lua 基础设施

`src/internal/infra/teamquery/lua_executor.go` 的 `newRosterProxy`：

- 解析 `team_id`、`team_name`、`player_name` 到扩展后的 `roster.PlayerFilter`；
- 不再在 Lua 层要求 `team_id`；
- 参数缺失和 ID/名称互斥交由领域服务校验后，以现有 Lua 错误机制向调用方报告；
- `rosterPlayerToLua` 写入 `team_name`。

`newGameProxy` 的 `matches`：

- 解析 `team_name` 到 `game.MatchFilter`；
- 改为调用 `QueryService.ListMatchSummaries`；
- `matchToLua` 改为接收 `MatchSummary`，输出主客队名称和既有比赛字段；
- `game.match` 仍返回单个 `Match` 的既有 UUID 字段，不隐式扩大读取范围。

`src/internal/domain/teamquery/service.go` 的渐进式描述同步修改：

- `roster.players` 说明可按球队 ID、球队名称和球员姓名精确筛选，三个定位参数至少提供一个；列出 `team_id` 与 `team_name` 互斥规则，并在返回字段中增加 `team_name`。
- `game.matches` 说明可按球队 ID 或球队名称、时间和状态筛选；列出 ID/名称互斥规则，并在返回字段中增加 `home_team_name`、`away_team_name`。
- `defaultRuntimeDescription` 的提示不再称 `roster.players` 必须指定 `team_id`，改为提醒优先提供名称筛选，并遵循各 topic 的最小范围要求。

### Skill 约束

`skills/manage-team/SKILL.md` 在“查询能力目录”“编排规则”“查询结果投影”和跨模块示例处修改为：

1. 已知球队名称时，优先传递 `team_name` 给 `roster.players` 或 `game.matches`；已知球员名称时，传递 `player_name`。不得先调用 `roster.teams` 读取所有球队，仅为取得 UUID 再发起名称已知的查询。
2. 日期、状态、守备位置等可下推条件必须与名称条件一并传给 topic；不得先读取宽范围结果再在 Lua 内筛选。
3. UUID 只在下游 topic 或 `team_modify` 硬性要求稳定标识时使用：例如以 `match_id` 读取单场过程、以 `player_id` 读取训练、以实体 ID 修改已有数据。普通业务回答不返回实体 ID、关联 ID、版本及时间戳，除非用户明确要求。
4. 名称查询可能命中多个实体。后续必须精确定位单一实体时，Lua 应投影可区分的业务字段和必要稳定 ID；模型不得自行挑选同名结果，必须请求用户确认。
5. 重写“球队名 → UUID → 名单”的示例，改为单次名称参数查询；比赛示例直接使用 `home_team_name`、`away_team_name`，不再为展示对阵信息预读球队表。

`skills/project-knowledge/SKILL.md` 仅在“当前能力”提及查询方式的句子中补充“球员和比赛列表可按业务名称精确筛选”，不展开工具参数和编排规则，避免与 `manage-team` 重复维护同一事实。

## 文件级修改清单

| 路径 | 具体落点 | 修改内容、原因与依赖关系 |
| --- | --- | --- |
| `src/internal/domain/roster/query.go` | `PlayerFilter`、`QueryService.ListPlayers` | 增加 `TeamName`、`PlayerName`，实现定位条件必填、ID/名称互斥和空白值规则。领域层定义名单查询业务语义，依赖 `team`、`player` 值类型，不依赖 PostgreSQL 或 Lua。 |
| `src/internal/domain/roster/query_test.go` | 新增的 Filter 校验用例 | 覆盖仅名称、仅 ID、仅球员名称、ID/名称冲突和无定位条件错误，防止查询边界退化为全量名单读取。 |
| `src/internal/domain/game/query.go` | `MatchFilter`、`QueryRepository`、`QueryService.ListMatches`、新增 `MatchSummary` 与 `ListMatchSummaries` | 添加 `TeamName`；将 Filter 下推 Repository；删除内存过滤；定义含双方球队名称的只读比赛投影。领域层仍不依赖具体数据库。 |
| `src/internal/domain/game/query_test.go` | 新增的 Query Service 测试 | 覆盖 Filter 校验、Repository 收到完整 Filter、`ListMatches` 不再先取全量、摘要查询委托与错误传播。 |
| `src/internal/infra/postgres/roster_reader.go` | `RosterReader.ListPlayers` SQL 和参数 | 按球队 ID/名称、球员名称、日期、位置在 SQL 中筛选，继续排除软删除记录并恢复 `roster.Member`。 |
| `src/internal/infra/postgres/game_repositories.go` | `MatchRepository.List`、新增 `ListSummaries`、`Store` 转发方法 | 将比赛 Filter 下推 SQL；使用双方球队 JOIN 一次读取 `MatchSummary`，避免 Lua 对每场比赛额外读取球队。 |
| `src/internal/infra/postgres/store_integration_test.go` | 名单与比赛 Repository 集成测试 | 插入同名/不同名、软删除和不同状态/时间的测试数据，验证 SQL 筛选、名称投影、组合条件及旧 UUID 条件。固定测试库仍按现有机制重置。 |
| `sql/migrations/006_name_query_indexes.sql` | 新增局部索引 | 为未软删除球队名、球员名的等值查询建立 B-tree 索引；不改变表结构、约束或软删除策略。 |
| `src/internal/infra/teamquery/lua_executor.go` | `newRosterProxy`、`rosterPlayerToLua`、`newGameProxy`、比赛 table 转换函数 | 解析新增 Lua 参数，调用扩展领域服务，输出 `team_name`、`home_team_name`、`away_team_name`。Lua 层不实现业务筛选。 |
| `src/internal/infra/teamquery/roster_test.go` | Lua 名单查询测试 | 验证名称参数能构造 Filter、无 ID 的名称查询可执行、冲突参数报错和 `team_name` 序列化。 |
| `src/internal/infra/teamquery/game_test.go` | Lua 比赛查询测试与 stub | 更新 Repository stub 的端口，实现摘要返回；验证 `team_name` 传递和双方球队名称输出。 |
| `src/internal/domain/teamquery/service.go` | `defaultRuntimeDescription`、`playerFields`、`matchFields`、`roster.players`、`game.matches` topic | 对外描述新增参数、互斥与最小范围规则，公开名称字段，修正旧的 `team_id` 强制提示。 |
| `src/internal/domain/teamquery/service_test.go` | `Describe` topic 断言 | 更新参数数量、必填属性、返回字段和示例断言，确保渐进式工具契约准确。 |
| `skills/manage-team/SKILL.md` | 查询目录、编排规则、投影规则、跨模块示例 | 明确优先名称筛选、UUID 仅用于下游稳定定位、禁止为取 UUID 的宽范围读取、同名结果的确认策略。 |
| `skills/project-knowledge/SKILL.md` | “定位与当前能力”查询能力说明 | 简要同步已支持的按名称精确筛选能力，避免项目介绍与实际能力不一致。 |
| `src/internal/harness/skills_test.go` | Skill 关键短语断言 | 调整断言，确保加载的 Skill 保留名称筛选及 UUID 限制的关键约束。 |
| `doc/overview/roster-domain.md` | “领域服务与端口”段落 | 仅在用户后续要求创建提交时，更新为名称和 ID 均可约束的名单查询现状。 |
| `doc/overview/game-domain.md` | “服务与端口”段落 | 仅在用户后续要求创建提交时，更新为 SQL 下推的比赛筛选与比赛摘要名称投影现状。 |
| `doc/overview/teamquery-domain.md`、`doc/overview/tools.md` | 查询契约与工具说明 | 仅在用户后续要求创建提交时，更新 topic 参数与名称返回字段的长期现状说明。 |

## 错误、兼容性与数据边界

- `team_id`、`team_name` 同时传入时，`roster.players` 与 `game.matches` 都返回明确参数错误；不会静默选择任意一个条件。
- 名称过滤无匹配时返回空数组，不返回“未找到”领域错误；这与现有列表型查询一致。
- `roster.players` 没有 `team_id`、`team_name`、`player_name` 时返回错误，避免恢复全量成员读取。
- `game.matches` 的空 Filter 继续兼容现有“全部未删除比赛”的语义；对已有调用方不构成破坏性变更。
- `game.QueryRepository` 方法签名变化会影响 Repository stub 和实现；所有编译错误必须在同一次改造中修复。`ListMatchSummaries` 是新增能力，不替换 `GetMatch`、`ListLineups`、`ListPlays`。
- 不改变 UUID 的生成、数据库列类型或任何公开写入操作参数。名称只用于读取筛选，不能替代修改协议中的稳定实体 ID。
- 所有业务 Repository 默认读取仍排除 `deleted_at` 非空的数据；新增名称索引同样使用 `WHERE deleted_at IS NULL`，避免已删除数据占用查询索引空间或被意外命中。
- 名称字段新增到 Lua table 和 topic 返回描述，属于向后兼容的响应扩展；已有 Lua 程序不引用它们时行为不变。

## 验证方案

实现完成后执行：

1. 运行名单和比赛领域单元测试，验证 Filter 校验、名称精确匹配语义、ID/名称互斥与 Repository 委托。
2. 执行 PostgreSQL 集成测试，验证球队名、球员名、比赛日期/状态组合在 SQL 层完成筛选，且软删除球队、球员、名单和比赛均不返回。
3. 执行 Lua 适配测试，验证名称参数解析、错误消息、`team_name`、`home_team_name`、`away_team_name` 输出，以及既有 UUID 参数仍可用。
4. 执行 `go test ./...`、`go vet ./...` 和架构测试，确认端口变更未破坏分层依赖。
5. 使用 `team_query describe` 验证 `roster.players`、`game.matches` 的参数、返回字段与示例；加载 `manage-team` Skill，确认其中明确规定已知名称时不应先读取 UUID。
6. 若用户后续要求提交，在创建提交前同步更新本设计列出的 overview 文档，并新建且仅新建一份 `doc/changes/` 变更记录；本设计文档不替代变更记录。
