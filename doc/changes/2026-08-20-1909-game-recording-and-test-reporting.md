# 完整比赛录入与测试报告升级

## 背景

原有球队管理 Skill 把查询、名单、比赛安排、整场比赛录入和训练维护混在同一份指引中。录入整场比赛时，模型还必须重复填写球队归属、局面、球数、跑垒和比分，参数体积大且容易出现不连续的比赛状态。批量测试 JSONL 已不再保存完整模型请求和输出，评测器仍需适配新的 Trace 格式，并提供便于人工分析的调用报告。

## 变更

- 将 `manage-team` 拆分为 `query-team-data`、`manage-roster`、`manage-game-setup`、`record-game` 和 `manage-training` 五个必需 Skill；`project-knowledge` 按任务把用户导向对应 Skill。Harness 启动时校验新的六个必需 Skill，README 同步列出它们。
- 新增 `team_modify` 的 `game.create`。它用精确球队名、首发背号和紧凑 Play 事件流创建一场 final 比赛，领域层推导球队归属、逐球好坏球数、局面、跑者、出局和比分。一次调用完成批量解析与校验后，按比赛、两套阵容和 Play 写入；若写入阶段失败，返回 `GameCreateProgress` 的 `partial` 进度而不自动回滚。
- 比赛领域新增姓名化与紧凑比赛草稿、名称/背号解析端口及可选直接 Repository 配置。PostgreSQL 新增按球队名、球队/背号的批量查询，Store 提供 `DirectGameRepositories`；迁移 `008_unique_team_names.sql` 为未删除球队名称建立唯一索引，重复名称映射为 `team.ErrNameOccupied`。
- 批量测试结果升级为 schema version 2。入口输出请求级 token usage，评测脚本校验新 schema、生成 `.evaluation.json` 和可读 `.report.md`；Markdown 报告从事件中呈现 Prompt、思考、工具调用、工具结果和回复。
- 移除旧的 `config/test.toml` 和 `manage-team` Skill，新增按职责拆分的测试 TOML 与 Skill 文件，并更新 sqlc 生成绑定、集成测试和工具/领域测试。

## 影响模块

### Harness 与 Skill

- `src/internal/harness/skills.go`、`src/internal/harness/skills_test.go`、`skills/`：必需 Skill 从两个变为六个。模型仍通过同一 `skill` 工具加载说明，但按只读查询、名单、比赛安排、比赛录入和训练维护选择窄范围指引。
- `skills/record-game/SKILL.md`：定义 `game.create` 的单次完整录入与确认流程，要求通过 describe 取得当前字段约束，并在 `partial` 时先读取现状而不是自动重试。

### 工具与领域层

- `src/internal/tools/team_modify.go`、`src/internal/tools/team_modify_game.go`、`src/internal/tools/team_modify_game_test.go`：新增 `game.create` 的 describe、校验和执行适配；成功结果提供完成写入回执，部分写入以原有工具错误语义暴露进度。
- `src/internal/domain/game/compact_record.go`、`named_record.go`、`service.go`：紧凑草稿展开为既有实体可校验的姓名化记录，新增一次性名称/背号解析和顺序写入服务。普通比赛、阵容和 Play 的事务性修改接口保持不变。

### 基础设施与数据库

- `sql/migrations/008_unique_team_names.sql`、`sql/queries/player.sql`、`sql/queries/team.sql`、`src/internal/infra/postgres/repositories.go`、`store.go` 及 `sqlcgen/`：新增未删除球队名唯一索引、两类批量读取及适配层错误映射。软删除后仍可复用球队名，现有球员背号唯一约束不变。
- `src/internal/bootstrap/app.go`：为比赛写服务装配直接 Repository 与 UUID 生成器；配置和数据库 profile 未改变。

### 入口、评测与测试配置

- `src/internal/entry/cli/test_mode.go`、`test_mode_test.go`、`scripts/evaluate_test_results.py`、`scripts/test_evaluate_test_results.py`：测试 JSONL 升级为 schema version 2，评测从请求级 token usage 统计并输出 Markdown 调用轨迹。模型输入、状态栏和模型输出继续不进入 JSONL。
- `config/test/` 替代单个 `config/test.toml`，覆盖拆分后的 Skill 场景；`README.md` 更新运行时必需 Skill 说明。

### 文档

- `doc/overview/harness.md`、`tools.md`、`game-domain.md`、`roster-domain.md`、`infrastructure.md`、`bootstrap-and-config.md`、`entry.md`：同步当前 Skill 划分、完整比赛录入协议、名称唯一性、直接 Repository 装配和批量测试报告边界。

## 验证

- `env GOCACHE=/tmp/basetion-go-cache go test ./...`：通过。
- `python3 -m unittest scripts/test_evaluate_test_results.py`：通过，5 个测试全部成功。
- `git diff --check`：通过。
