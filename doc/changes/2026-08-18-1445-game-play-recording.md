# 比赛过程记录改造：Plate 替换为 Play

## 背景

旧的比赛记录以 `Plate`（打席）为最小单元，只保存 B/S/F 投球序列字符串、打席类型和打席结束后的累计比分快照，无法表达逐球明细、跑者进垒与得分归属和守备贡献，支撑不了按球员汇总进攻、投球、守备数据的分析类查询。本次按 `doc/designs/game-recording.md` 的设计，用 `Play` 聚合替换 `Plate`，一次 Play 记录一次已结束打席的前后局面、逐球过程、打击结果以及跑垒、守备子结果。

## 变更

- 领域层删除 `Plate` 聚合，新增 `Play` 聚合及 `Situation`、`Pitch`、`RunnerOutcome`、`FieldingOutcome` 子结构；新增 `GameRecord` 作为整场比赛的读写边界，写服务支持一次校验并写入整场记录，单条 Play 的创建、替换、删除会校验比赛内顺序唯一和相邻局面衔接。
- 数据库删除 `plates` 表，新增 `plays`、`pitches`、`play_runner_results`、`play_fielding_results` 四张表，均带乐观版本和 `deleted_at` 软删除。
- 查询契约 `game.plates` 改名 `game.plays`，返回带 `before`、`after` 局面和逐球、跑垒、守备子表的完整比赛过程；当前比分改取 sequence 最大有效 Play 的结束局面。
- `team_modify` 移除 `plate.create`、`plate.update`、`plate.delete` 三个操作，工具层不再暴露逐条改写比赛过程的入口；比赛过程写入暂时只能走领域服务的 `CreateGameRecord` 等内部路径。
- 开发 fixture 的演示比赛从 65 条打席缩减为一条九局下半刘备再见一垒安打的完整 Play（含逐球和跑垒明细），最终比分蜀汉 5:4 曹魏不变。
- `just test complex` 新增一个跨球员攻防数据分析的冒烟提示词。

## 影响模块

### 领域层

- `src/internal/domain/game/plate.go` 删除，新增 `src/internal/domain/game/play.go`：`Play` 持有比赛内顺序、局次、上下半局、棒次、打者、开局投手、`Before`/`After` 两个 `Situation` 值对象（出局数、主客队比分、一、二、三垒上的跑者）、`BattingResult` 以及按各自 sequence 排序的 `Pitch`、`RunnerOutcome`、`FieldingOutcome` 子结果；`NewPlay`/`RestorePlay` 校验局面出局数范围、子结果顺序和最后一球能否结束打击结果。原 `Plate` 的可空比分快照、`pitch_sequence` 字符串和 `PlateType` 随之移除。
- `src/internal/domain/game/service.go`：`PlateRepository` 端口改名 `PlayRepository`，`Update` 改为 `Replace`（按整条草案替换并校验版本）；`CreatePlate`/`UpdatePlate`/`DeletePlate` 改为 `CreatePlay`/`ReplacePlay`/`DeletePlay`，替换和删除要求调用方传入期望版本；新增 `validatePlayNeighbors`，在事务内加载同场 Play 列表检查顺序唯一与相邻局面连续；新增 `CreateGameRecord` 与 `validateGameRecord`，一次校验并写入 Match、双方阵容和全部 Play。`DeleteMatch` 仍在同一事务内显式软删除关联阵容与比赛过程（含子结果），软删除编排方式不变。
- `src/internal/domain/game/query.go`：`QueryRepository.ListPlates` 改名 `ListPlays`；新增 `GetGameRecord` 返回完整 `GameRecord`，`GetDetail` 改为复用它组装比赛、阵容和过程；`CurrentScore` 不再读 Plate 比分快照，改取 sequence 最大有效 Play 的 `After` 局面比分，`Final` 仍只由 Match 状态决定。
- `src/internal/domain/teamquery/service.go`：能力目录中的 `game.plates` topic 改名 `game.plays`，字段说明从打席快照改为前后局面、打击结果和子结果列表，供 `team_query` 的 `describe` 输出给模型。

### 基础设施层

- `src/internal/infra/postgres/migrations/002_game_soft_delete.sql`：移除 `plates` 建表语句和它的两个索引；`matches`、`lineups` 表结构不变。
- `src/internal/infra/postgres/migrations/003_game_status_score.sql`：移除给 `plates` 加 `home_score`、`away_score` 的语句，只保留 `matches.status`。已有数据库的迁移账本不会重放这两条迁移，改动只影响全新建库；旧库中现存的 `plates` 数据由 005 的 `DROP TABLE IF EXISTS plates` 清理，不做数据迁移。
- `src/internal/infra/postgres/migrations/005_game_plays.sql` 新增：删除 `plates`，新建 `plays`（前后局面内联为列）、`pitches`、`play_runner_results`、`play_fielding_results` 四张表，均有 `version`、`created_at`、`updated_at`、`deleted_at`；`plays` 和 `pitches` 建部分唯一索引保证有效记录内比赛顺序和逐球顺序唯一。无外键和级联删除，符合软删除约定。
- `src/internal/infra/postgres/game_repositories.go`：删除 Plate 存储实现；新增 `src/internal/infra/postgres/play_repository.go`，在一个事务内写入 Play 主行和三组子结果，读取时按 sequence 还原完整聚合；`Replace`、`SoftDelete` 校验乐观版本，`SoftDeleteByMatch` 级联软删除四张表中的有效行，供领域服务的事务编排调用。
- `src/internal/infra/postgres/store.go`：`GameDetail` 返回结构中的 `Plates` 字段改名 `Plays`。
- `src/internal/infra/postgres/fixtures/002_game_demo.sql`：演示比赛从 65 条打席改为一条完整 Play（九局下半、两出局、二垒有人，刘备再见一垒安打，4:4 到 5:4），附带一条逐球记录和一条二垒跑者得分且记打者打点的跑垒结果。
- `src/internal/infra/teamquery/lua_executor.go`：Lua 侧 `team.game.plates` 改名 `team.game.plays`，`playToLua` 输出 `before`、`after` 局面子表和 `pitches`、`runner_results`、`fielding_results` 三个显式数组；`plateTypeName` 换成覆盖更多枚举的 `battingResultName`。只读转换职责不变。

### 工具层

- `src/internal/tools/team_modify_game.go`：删除 `plate.create`、`plate.update`、`plate.delete` 的分派和参数定义；`team_modify` 的 `describe` 目录随之不再有 `plate` 组。比赛过程的写入口收敛到领域层，等后续按 `GameRecord` 设计专门的工具协议。
- `src/internal/tools/team_modify.go`：操作顺序表和 `describeGroup` 的组列表移除 `plate`；`match.delete` 的摘要改为“软删除比赛及关联阵容和比赛过程”。批次执行语义、50 项上限和逐项状态报告不变。

### Harness/Skill

- `skills/manage-team/SKILL.md`：查询清单和 Lua 示例从 `game.plates` 改为 `game.plays`，操作清单删除 `plate.*` 三条；投影与最小证据结构的规范不变。
- `src/internal/harness/skills_test.go`：Skill 必备语料检查同步把 `game.plates` 换成 `game.plays`，去掉 `plate.create/update/delete`，并把“不得直接返回原始对象”的反例从 `return team.game.plates` 换成 `return team.game.plays`。

### 测试

- `src/internal/domain/game/game_test.go`：Plate 构造与校验用例改写为 Play/Situation/Pitch 的新建与恢复校验。
- `src/internal/infra/postgres/store_integration_test.go`：fixture 断言从 65 条打席改为 1 条 Play 加 4 条阵容；CRUD 用例改用 `CreatePlay` 并断言 `plays`、`pitches` 表的软删除行数。
- `src/internal/infra/teamquery/game_test.go`、`src/internal/tools/team_modify_test.go`：跟随契约改名和 `plate.*` 操作移除调整断言。

### 工具与任务入口

- `just/test.just`：新增 `just test complex`，用 dev profile 让 Agent 汇总全部比赛数据并按进攻、投球、守备三类输出球员数据排序，作为新数据结构的端到端冒烟提示词。文件末尾补了换行。

### 文档

- `doc/designs/game-recording.md` 新增：比赛记录改造设计草案，定义 `GameRecord`/`Play`/`Situation`/`Pitch` 等结构和校验规则，明确分析指标、工具协议和历史数据迁移不在本次范围。
- `doc/overview/game-domain.md`：聚合、服务和 fixture 段落改写为 Play 模型。
- `doc/overview/infrastructure.md`：Repository 清单和比赛过程表结构说明改为 `plays` 及三张子结果表。
- `doc/overview/teamquery-domain.md`、`doc/overview/tools.md`：`game.plates` 与 `plate.*` 操作的相关描述同步更新。
- `doc/overview/README.md`：模块索引和当前边界中残留的“打席”表述改为比赛过程。

# 验证

- `GOTOOLCHAIN=auto go build ./...`：通过。
- `GOTOOLCHAIN=auto go test ./src/internal/domain/... ./src/internal/tools/... ./src/internal/harness/... ./src/internal/infra/teamquery/...`：全部通过。
- `GOTOOLCHAIN=auto go test ./...`：沙箱内运行时，除 `src/internal/infra/postgres` 外的包全部通过；该包的集成测试需要连接本地 PostgreSQL（localhost:5432），沙箱禁止网络连接导致失败，申请在沙箱外运行被审批流程拒绝，故本次未执行依赖数据库的完整测试，迁移与 fixture 的实际加载未在本机验证。
