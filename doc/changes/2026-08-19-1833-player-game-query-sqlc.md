# 背景

球队数据原先将球员与球队的关系拆分为 `domain/roster.Membership`，比赛读取返回包含 UUID 的聚合对象，Lua 只能通过旧的 `roster`、`lineup` 等模块查询。PostgreSQL 的持久化 SQL 与 Go 适配代码分散维护，逐球表仍名为 `pitches`。这使按名称查询球员、组合比赛事实、让 Agent 读取逐场表现都需要跨层拼接或重复规则。

# 变更

## 领域层

- `src/internal/domain/player/{player.go,service.go,query.go}` 将 `team_id`、`jersey_number` 收归 `Player`，新增 `PlayerFilter`、`PlayerView` 和 `QueryService`。创建球员在 Player UnitOfWork 中校验启用球队与同队背号占用；改号复用该事务检查。删除球队时显式软删除该队球员，删除球员时显式软删除训练记录。`src/internal/domain/roster/` 及 Membership、效力日期、名单查询/写服务已删除，球员没有转队入口。
- `src/internal/domain/game/query.go` 将比赛只读端口改为 `ListMatches`、`SummarizeMatches`、`GetMatchRecords`、`ListMatchLineups`、`AnalyzeMatchPlayers` 五个批量投影。它们共享 `MatchFilter`：最多两个参与球队名称、闭区间赛程和非负 `Limit`。比赛记录、阵容和表现使用姓名、球队名、背号识别球员；统计按每场输出，并通过 `PerformanceLimitsView` 标注不能从现有事实推断的项目。
- `src/internal/domain/{team,game,training}` 删除实体的 `version` 字段和乐观并发检查，保留创建、更新和 `deleted_at` 的时间合法性校验。比赛写服务仍在 Game UnitOfWork 中显式软删除关联阵容、Play 及其子结果。
- `src/internal/domain/teamquery/service.go` 将模型目录收敛为 `team`、`player`、`game`。叶子 topic 的可用性以 Executor 的具体 `AvailableTopics` 为准，不因模块被注入就把未注册查询标记可用。

## 基础设施层与数据库

- `sql/migrations/001_team_roster.sql` 直接创建带 `team_id`、`jersey_number` 的 `players`，不再创建 `memberships`；`005_game_plays.sql` 将逐球明细表改为 `play_pitching_results`。`007_remove_entity_versions.sql` 删除既有库的版本列。development fixture 和 `sql/test/reset.sql` 同步移除旧名单表与旧表名。
- `sql/queries/`、`sqlc.yaml`、`src/internal/infra/postgres/sqlcgen/` 新增 sqlc 管理的 pgx/v5 绑定，覆盖球队、球员、训练、比赛写入、比赛读取和表现聚合。`Store`、Repository 和事务适配器改为调用生成 `Queries`；`sqlvalues.go` 集中处理 UUID、时间戳和日期转换。球员唯一约束冲突转换为 `player.ErrJerseyOccupied`，默认读取持续排除软删除行。
- `src/internal/infra/postgres/game_repositories.go` 将比赛目录筛选、最终比分、完整记录、阵容和表现计算下推 SQL，按筛选后的比赛批量读取，避免逐比赛 N+1 查询。投球归属取最后一颗有效球的投手，缺少逐球记录时使用开局投手；守备统计只累计已保存的守备结果。
- `src/internal/infra/teamquery/{lua_executor.go,query_modules.go}` 发布 `team.list`、`player.list` 和五个 `game.*` 函数。Lua 只转换领域投影和参数；训练服务不再发布为模型可见模块。

## 工具、Harness 与仓库工具

- `src/internal/tools/team_modify.go` 删除名单入队、离队和改号关联操作，改为 `player.create` 时指定球队与背号，并提供 `player.change_jersey`。`team_query` 的协议与 `skills/manage-team/SKILL.md`、`skills/project-knowledge/SKILL.md` 同步为姓名化 Player/Game topic；复杂比赛分析使用 `game.performances`，不从记录或阵容重算统计。
- `src/internal/bootstrap/app.go` 装配 Player 的姓名化 Reader 和 Game QueryService，供 Lua Executor 与工具使用；不新增 application 子包。
- `scripts/manage_dev_db.py` 简化为 `reset|check`：reset 重建 TOML 配置的固定开发库、执行迁移和 fixture，再检查最小比赛数据；check 只验证现有 fixture。`just/db.just`、`just/test.just`、`justfile` 同步该命令边界，`complex` 不再隐式重建开发库。
- 新增 `just/sqlc.just`：`just sqlc generate` 以固定 `sqlc v1.30.0` 生成绑定，`just sqlc check` 生成后检查已提交文件是否漂移。应用和测试配方不隐式生成代码。

## 测试与文档

- 删除围绕旧 Membership、旧查询对象和手写 PostgreSQL store 的测试，新增 `src/internal/infra/postgres/sqlc_integration_test.go`，其余领域、Lua、工具和 Harness 测试同步新协议。
- `doc/overview/` 更新球队/球员、比赛、Team Query、PostgreSQL、工具、组合根、入口和训练模块的现状；`doc/designs/` 中已被新实现取代的四份旧设计稿随代码一并删除。本记录已按 `remove-ai-flavor` Skill 审校，未删减路径、协议或验证事实。

# 影响模块

## 领域层

- 路径：`src/internal/domain/player`、`team`、`game`、`training`、`teamquery`，并删除 `src/internal/domain/roster`。球员归属与背号从 Membership 移到 Player；比赛读取从 ID 聚合改为姓名化批量 View；Team Query 由旧的 roster/lineup/training 目录改为 team/player/game。基础设施实现新的 Repository 端口，工具只能通过这些服务读写。

## 基础设施层和数据库

- 路径：`src/internal/infra/postgres`、`src/internal/infra/teamquery`、`sql/migrations/`、`sql/queries/`、`sqlc.yaml`、`src/internal/infra/postgres/sqlcgen/`。数据库模式移除 memberships 与版本列，逐球表改名；sqlc 生成 SQL 绑定，Repository 继续负责软删除和领域错误映射；Lua 依赖 Player/Game 查询服务而不包含统计规则。

## 工具层、Harness 与 Skill

- 路径：`src/internal/tools/`、`src/internal/bootstrap/app.go`、`skills/manage-team/SKILL.md`、`skills/project-knowledge/SKILL.md`。工具协议不再发布名单变更或训练查询模块，新增按球队创建球员和改号；Bootstrap 将新读写服务注入工具；Skill 指导模型按 `describe` 选择新叶子并使用逐场表现投影。

## 工具链、fixture 与测试

- 路径：`scripts/manage_dev_db.py`、`just/{db,test,sqlc}.just`、`justfile`、`sql/development/`、`sql/test/reset.sql`、`src/internal/**/*_test.go`。开发库的重建被限定为显式 `db reset`，fixture 与新表结构匹配；sqlc 生成具备单独检查入口；测试覆盖迁移后的领域、工具和 Lua 契约。

## 文档

- 路径：`doc/overview/*.md`、`doc/designs/{game-recording,lua-internal-references-and-game-context,name-based-roster-game-queries,test-database-provisioning}.md` 与本文件。概览改为描述球员所有权、五类比赛投影、sqlc 工作流和新的工具边界；四份已失效的设计稿不再与现状文档并存；本文件记录本提交前后的外部行为、依赖方向和验证情况。

# 验证

- `GOTOOLCHAIN=auto go test ./...`：通过。
- `git diff --check`：通过。
- `just sqlc check`：未完成。受限环境无法解析 `proxy.golang.org` 以下载固定的 `github.com/sqlc-dev/sqlc/cmd@v1.30.0`；申请受控网络执行也被自动审批策略拒绝，未尝试绕过该限制。
