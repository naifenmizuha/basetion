# 背景

原有 `team_query` 同时承担简单读取、比赛摘要、完整记录、阵容、球员表现和 Lua 组合。模型需要在工具调用前选择模块，并在复杂分析中预判后续查询条件；Lua 循环也可能把逐条读取放大为多次数据库访问。实验分支需要把常用的完整比赛读取拆出来，同时让 Lua 能根据真实的中间结果继续读取 Play，而不把内部比赛 ID 放进模型上下文。

# 变更

## 工具层

- 新增 `src/internal/tools/team_fetch.go` 的 `team_fetch` 工具。它提供 `describe` 和 `fetch`，首批 operation 为 `game.summaries`、`game.records`、`game.lineups`、`game.performances`。每个 operation 复用 `game.QueryService` 的既有读模型和统计口径，接受参与球队、日期范围和 1 到 500 的数量限制；工具层不执行 Lua、不拼接 SQL，也不重算球员表现。
- 修改 `src/internal/tools/team_query.go`。`query` 仅接受定义 `main(data)` 的程序，移除原来的 `modules` 参数；工具说明明确将完整比赛投影导向 `team_fetch`。
- `src/internal/bootstrap/app.go` 现在同时注册 `team_fetch`、`team_query` 和 `team_modify`。这改变了模型可见工具清单，但没有修改写入工具的确认、批处理或错误语义。

## 领域层与基础设施层

- `src/internal/domain/teamquery/service.go` 将 Lua 目录收窄为 `team.list`、`player.list`、`game.list` 和 `game.plays`，不再发布摘要、完整记录、阵容和表现 topic；查询服务不再校验模块选择。运行时入口改为 `main(data)`。
- `src/internal/domain/game/query.go` 与 `src/internal/infra/postgres/game_repositories.go` 增加按一组内部 `MatchID` 读取 `PlayEventView` 的端口和实现。`MatchView` 在领域读模型中携带内部 ID，模型工具的投影转换不会序列化该字段；没有数据库迁移、表结构变化或删除语义变化。
- `src/internal/infra/teamquery/lua_executor.go` 和 `query_modules.go` 在每次 State 执行中维护 Lua table 到内部比赛引用的映射。`data.game.plays` 只接受同一次 `data.game.list` 返回并原样保留的 table，复制、伪造或跨执行引用会失败。执行期缓存复用相同的基础读取；读取预算限制为最多 8 次调用、单次 1,000 项、总计 5,000 项，调用次数在访问服务前保留。原有 2 秒超时、源码和 JSON 结果限制保持不变。

## Harness 与 Skill

- `skills/manage-team/SKILL.md` 改为判断任务应使用 `team_fetch` 的完整投影还是 `team_query` 的基础事实组合；Skill 说明 Lua 可基于筛选出的比赛用原始对象批量取 Play，且不应尝试获取内部 ID。
- `src/internal/harness/runtime_test.go` 与 `skills_test.go` 随工具注册和 Skill 约束更新，覆盖新的 `main(data)` 指令和 `team_fetch` 描述。

# 影响模块

## 模型工具与组合根

- `src/internal/tools/team_fetch.go`、`src/internal/tools/team_query.go`、`src/internal/bootstrap/app.go`：读取能力从单一 `team_query` 拆为完整投影读取和 Lua 基础读取两条协议。`team_fetch` 依赖 Game 领域查询服务，`team_query` 依赖 Team Query 领域服务；Bootstrap 把二者与既有修改工具一同交给 Harness。`team_modify` 的接口、确认要求和批量中止行为保持不变。

## 领域查询与 PostgreSQL

- `src/internal/domain/game/query.go`、`src/internal/domain/teamquery/service.go`、`src/internal/infra/postgres/game_repositories.go`：Game 查询端口新增批量 Play 投影，Team Query 协议删除组合比赛 topic 并删除模块选择。PostgreSQL 仍通过现有 `plays`、球员和球队读取 SQL 过滤软删除记录；没有新增 SQL 迁移、没有物理删除，也没有公开数据库 ID。

## Lua 运行时

- `src/internal/infra/teamquery/lua_executor.go`、`src/internal/infra/teamquery/query_modules.go`：从按调用方选择模块的 `team` 代理改为固定基础数据面的 `data` 代理。比赛 table 的身份成为只在当前 State 有效的不透明句柄，供 `game.plays` 精确批量读取；最终 JSON 转换不包含句柄。缓存和预算减少相同读取与循环读取的成本，同时保留原有沙箱、超时和结果转换约束。

## Harness/Skill、测试与文档

- `skills/manage-team/SKILL.md`、`src/internal/harness/*.go`：模型指令从“启用全部模块并调用组合 topic”改为按任务选择 Fetch 或 Query，调用关系与注册工具保持一致。
- `src/internal/tools/*_test.go`、`src/internal/domain/teamquery/service_test.go`、`src/internal/infra/teamquery/lua_executor_test.go`：新增 Fetch 四个 operation、动态 Play 读取、伪造引用拒绝、读取预算和同次执行缓存的测试；既有工具、Skill 和运行时测试改为新协议。
- `doc/overview/`：更新工具、Team Query、基础设施、Harness、组合根和总览的当前行为说明。本次 changes 文件记录该提交的协议拆分与验证结果。

# 验证

- `go test ./...`：通过。
- `git diff --check`：通过，无空白错误。
- 未运行人工 Agent 对比：该提交提供两种工具协议和单元/运行时覆盖，实际模型调用轮数、证据质量和成本仍需用同一批真实提问在基线分支与本分支上人工比较。
