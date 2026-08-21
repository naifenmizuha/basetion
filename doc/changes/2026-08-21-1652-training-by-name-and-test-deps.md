# 自训按名录入与读取、测试 run 依赖与 team_fetch 命名空间澄清

## 背景

工具协议缺少面向最终用户的自训录入路径：`team_modify.training.create` 原本要求模型自己先查到 `player_id` 再写入，一旦同名球员存在就没有消歧机制；自训记录也无法通过 `team_fetch` 的服务端组合读取拿到带姓名的视图，只能在 Lua 里手工关联。批量测试 `basetion test` 的并行度之前由 CLI 旗标控制，且不同 `[[test.<run>]]` 之间没有依赖表达能力，"先写入后查询"这类用例只能串行执行才稳定。另外，模型多次把 `team_describe` 返回的 `fetch.training.records` 当作 `team_fetch.operation` 直接传入，触发 `unknown team fetch operation`；tool schema 里没说清 `operation` 应当是模块内叶子名。

## 变更

- `team_modify.training.create` 接受 `player_name`（精确匹配必填）与可选 `team_name`，服务端在领域层按姓名解析球员：唯一命中直接写入；同名多人时返回带球队与背号的错误，由调用方补 `team_name` 重试；查无此人返回 not found。原有按 `player_id` 的写法不再暴露给工具层。
- `team_fetch` 新增 `training.records` operation，支持 `player_name`（精确匹配，同名全返回）、`date_from`、`date_to`、`limit`，返回由领域层拼装的 `{player_name, team_name, training_date, content, reflection}`，不暴露内部 ID。`team_describe` 的 `fetch` 目录新增 `fetch.training.records` topic 供模型按需读取参数说明。
- `team_fetch.operation` 的 JSON Schema 描述改为明确"模块内叶子名，不带任何前缀"，并在描述里直接列出可用 operation；`team_fetch` 遇到未知名称时的错误信息附带全部可用 operation 列表，便于模型自愈。
- 批量测试用例 TOML 新增 `[settings]` 表：`max_concurrency` 为正整数，缺省 4。`basetion test` 的 `--max-concurrency` 旗标同步删除，避免文件与 CLI 双源。
- 批量测试 TOML 的 `[[test.<run>]]` 首个表新增 `depends_on = ["<run 名>"]`，表达 run 级依赖。加载阶段会校验：引用必须存在、不得自依赖、不得重复、不得成环，且非首表不允许携带 `depends_on`。调度按拓扑稳定字典序启动 goroutine，被依赖 run 未完成时依赖方不占用并发信号量；上游任一 turn 失败（含写 JSONL 失败）会把整个下游 run 输出为 `status="skipped"`，并把失败沿依赖链继续向下游传播。
- `config/test/training-record.toml` 使用 `[settings] max_concurrency = 2`，以自训记录分析场景覆盖 `team_fetch.training.records`。

## 影响模块

### 数据库与 SQL

- 路径：`sql/queries/player.sql`、`sql/queries/team.sql`、`sql/queries/training.sql`。新增按姓名查球员（可选球队 ID 过滤）、按球员 ID 集批量查球员、按球队 ID 集批量查球队、按球员 ID 集加日期范围加 limit 列训练记录；`just sqlc generate` 重新生成 `src/internal/infra/postgres/sqlcgen/{player,team,training}.sql.go` 与 `querier.go`。所有读取继续只返回未软删除数据，未引入跨表 JOIN。

### 领域层

- 路径：`src/internal/domain/training/service.go`。`Service.Create` 签名改为按 `playerName` + `teamName` 解析；新增 `ErrPlayerNotFound` 与 `ErrAmbiguousPlayerName`（后者携带候选列表供上层拼装报错文案）；`PlayerRepository` 端口扩展 `ListByNameWithTeam`，返回带 `TeamName` 的 `PlayerWithTeam` 投影。
- 路径：`src/internal/domain/training/query.go`。`Filter` 增加 `PlayerName` 与 `Limit`，保留 `PlayerID` 供 Lua 代理继续按 ID 精确查询；新增 `RecordView`、`PlayerReader`、`TeamReader` 端口以及 `WithPlayerReader`、`WithTeamReader` 选项；`ListViews` 统一"两次查询"：先按名或 ID 解析球员候选，再按球员 ID 集读取记录，最后批量取球员与球队组装视图，跳过软删除或失效引用。

### 基础设施层

- 路径：`src/internal/infra/postgres/repositories.go`。`TeamRepository` 新增 `GetByIDs` 返回 `map[team.ID]string`；`PlayerRepository` 新增 `ListByNameWithTeam`、`ListByName`、`GetByIDs`，全部显式过滤 `deleted_at`。
- 路径：`src/internal/infra/postgres/training_repository.go`。新增 `ListByPlayers(ctx, []player.ID, from, to *training.Date, limit int)` 实现按球员集合 + 日期范围 + 上限的查询。
- 路径：`src/internal/bootstrap/app.go`。`training.NewQueryService` 通过 `WithPlayerReader`、`WithTeamReader` 注入数据库实现，再把 `*training.QueryService` 传给 `NewTeamTools`，供 `team_fetch` 的 `training.records` 使用。

### 工具层

- 路径：`src/internal/tools/team_modify.go`。`TrainingModifier.Create` 接口签名改为按 `player_name` + `team_name`，`trainingCreateArguments` 同步调整；`training.create` 的 describe 文档更新为按名创建及重名处理约定。
- 路径：`src/internal/tools/team_fetch.go`。注册 `training.records` operation；`fetchTrainingFilter` 负责参数校验；`fetchTrainingRecords` 把领域 `RecordView` 投影成不含内部 ID 的对外结构；`teamFetchInput.Operation` 的 JSON Schema 描述改写并列出可用值；未知 operation 的错误信息附带可用列表。
- 路径：`src/internal/tools/team_describe.go`。`NewTeamTools` 接收 `*training.QueryService`；`fetch` 目录新增 `training` 模块与 `training.records` 叶子。

### 入口层

- 路径：`src/internal/entry/cli/test_mode.go`。`testCasesFile` 新增 `Settings testSettings`；`testSettings.MaxConcurrency` 使用 `*int` 以区分未设置与显式 0；`testCase` 新增 `DependsOn` 字段，`testRun` 同步承载；`loadTestCases` 负责 `depends_on` 的存在性、自依赖、重复与首表位置校验；新增 `validateTestDependencies` 与 `sortTestRunsTopologically`，按字典序稳定输出拓扑序；`ExecuteTest` 为每个 run 维护 `done` channel 与失败标志，依赖未就绪时 goroutine 阻塞等待，不占用信号量；上游失败时为下游 run 直接写入 `status="skipped"` 的 `testResult`，并把失败沿依赖链向下游传播。
- 路径：`src/internal/entry/cli/cli.go`。usage 文案同步删除 `--max-concurrency` 旗标。
- 路径：`config/test/training-record.toml`。新增 `[settings] max_concurrency = 2`；`[[test.analyze]]` 查询蜀汉队近三天的自训记录，并要求调用 `team_fetch`。

### 测试

- 路径：`src/internal/domain/training/service_test.go`、`src/internal/domain/training/query_test.go`。新增按名创建的候选解析、重名报错、球队消歧、空名、球队不匹配用例，以及 `ListViews` 在按 ID、按名、日期范围、limit、跳过失效球员等场景下的行为。
- 路径：`src/internal/entry/cli/test_mode_test.go`。覆盖 `--max-concurrency` 旗标已删除（收到该旗标退出码 2）、`[settings]` 默认值与非法值、未知 settings 键、`depends_on` 的解析与全部校验失败路径、拓扑序稳定性、依赖顺序执行、上游失败下游整 run 跳过。`orderedTestConversation` 与 `failBySessionIDConversation` 作为测试桩分别验证执行顺序与失败注入。
- 路径：`src/internal/tools/team_modify_test.go`、`src/internal/tools/team_fetch_test.go`、`src/internal/tools/team_describe_test.go`。更新工具协议测试桩以匹配新签名；新增 `training.records` describe 与 fetch 行为用例。

### 文档

- 路径：`doc/overview/entry.md`、`doc/overview/tools.md`、`doc/overview/training-domain.md`。入口模块说明批量测试的 `[settings]` 与 `depends_on`；tools 模块说明 `training.records`、按名创建以及 fetch operation 命名约定；training 领域说明按名创建、错名/重名错误和 `ListViews` 的两次查询装配。

## 验证

- `go build ./...`：通过。
- `go test ./...`：全部包通过，包括 `src/internal/entry/cli`、`src/internal/tools`、`src/internal/domain/training`。
- `just test config/test/training-record.toml`：通过；`analyze` run 成功调用 `team_fetch` 查询蜀汉队近三天的自训记录，未再出现 `unknown team fetch operation "fetch.training.records"` 报错。
