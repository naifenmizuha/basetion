# 背景

CLI 原先只能手工执行单轮或手工续接 Session，缺少可复现的批量会话运行、模型轨迹留档和离线评测入口。开发测试命令还散落在 `just/test.just`，无法把一次命名测试的执行和结果检查串成固定流程。

# 变更

## 入口层

- `src/internal/entry/cli/test_mode.go` 新增 `basetion test` 批量协议。它读取 `[[test.<run-name>]]` TOML 数组表；同名表按顺序复用一个业务 Session，不同 run 在 `--max-concurrency` 限制内并行。每张表的 `prompt` 必填，`expect_tools`、`expect_skills`、`forbid_tools` 可选；空名称、重复名称和同时要求又禁用的工具会在执行前被拒绝。
- 每轮结果写为私有权限 JSONL，保存 run、Session、轮次、状态、Agent 事件、模型请求 Trace 和可选路径预期。未给 `--output` 时文件落在 `.basetion/test-results/`，文件名带 run ID，避免覆盖既有结果。
- `src/internal/entry/cli/cli.go` 保留普通单提示词协议，并将 `test` 作为独立子命令分派。`config/test.toml` 提供项目知识和球队名单的示例 run，名单用例要求 `manage-team`、`team_query` 并禁用 `team_modify`。

## Harness、基础设施与组合根

- `src/internal/harness/trace.go` 增加按 Context 启用的 `TurnTrace`；`status_model.go` 在每次模型调用处采集实际请求、临时状态栏、流式响应和错误。Trace 只供批量结果写入使用，普通对话不会因此保留额外数据。
- `src/internal/infra/session/memory_store.go` 实现进程内 SessionStore，供批量 run 保存多轮历史。`src/internal/bootstrap/app.go` 对 `test` 子命令选择该 Store，普通会话仍使用文件 Store；数据库 profile、领域工具和写入语义没有改动。

## 工具层与配置

- 删除 `just/test.just` 的旧冒烟和 Go 测试配方。根 `justfile` 保留 `just test <cases.toml>`，由 `test-run` 执行 `just db check` 与 Go 批量测试，再由 `test-evaluate` 调用离线评测；前一步失败时不会伪造或继续评测缺失的 JSONL。
- `scripts/evaluate_test_results.py` 只使用 Python 标准库读取 JSONL，输出同名 `.evaluation.json` 和终端汇总。它按每个模型请求最大 token usage 去重，统计轮次、run 与并发墙钟耗时，解析工具和 Skill 路径，并把失败轮、缺失预期和禁用工具调用作为违规。报告不复制提示词、模型输入、工具结果或 reasoning。`scripts/test_evaluate_test_results.py` 覆盖流式 token 去重、路径违规和损坏 JSONL。

## 测试与文档

- `src/internal/entry/cli/test_mode_test.go` 覆盖命名 run 排序、多轮上下文、预期字段写入与非法预期拒绝；`src/internal/harness/status_model_test.go` 覆盖可选 Trace 的状态栏和输出采集；`src/internal/infra/session/memory_store_test.go` 覆盖内存 SessionStore。
- 更新 `doc/overview/entry.md`、`harness.md`、`infrastructure.md` 与 `bootstrap-and-config.md`，说明批量协议、Trace、内存会话、评测脚本和 Just 命令边界。

# 影响模块

## 入口层

路径：`src/internal/entry/cli`、`src/cmd/basetion`。CLI 新增批量测试外部协议和 JSONL 结果契约；正常单轮命令、渲染和退出码语义保持不变。入口通过既有 Conversation 接口调用应用层，不把模型或领域规则下沉到 CLI。

## Harness

路径：`src/internal/harness`。`TurnTrace` 由 Context 传递，状态栏模型在委托 AgenticModel 前后采集调试数据；它与 `TurnTelemetry` 并存，不改变 token 汇总日志、Skill Middleware 或工具注册。

## 基础设施层

路径：`src/internal/infra/session`。新增 `MemoryStore` 只实现应用层 `SessionStore` 端口，测试 run 的历史不会写入 FileStore 的原子 JSON 快照；PostgreSQL、迁移和软删除行为无变化。

## 工具与脚本

路径：`justfile`、`scripts/evaluate_test_results.py`、`scripts/test_evaluate_test_results.py`。`just test` 需要已准备的开发数据库，运行成功后把 JSONL 交给不访问数据库或模型的评测脚本。评测退出码可用于自动化门禁：输入格式错误为 2，确定性违规为 1，无违规为 0。

## 配置与测试数据

路径：`config/test.toml`。新增非密钥的批量测试示例；它不改变 `config/config.toml` 的运行配置，也不包含 API Key 或数据库 URL。

## 测试与文档

路径：`src/internal/entry/cli/test_mode_test.go`、`src/internal/harness/status_model_test.go`、`src/internal/infra/session/memory_store_test.go`、`doc/overview/`。测试覆盖新协议、Trace 和内存存储；overview 描述的是提交后模块行为，changes 记录本次外部协议、分层和验证范围。

# 验证

- `just sqlc generate`：通过，生成绑定无错误。
- `just --fmt --check`、`just --show test`：通过，确认组合命令按 `test-run` 后 `test-evaluate` 的顺序执行。
- `env PYTHONPYCACHEPREFIX=/tmp/basetion-python-cache python3 -m unittest scripts/test_evaluate_test_results.py`：通过，3 项评测脚本测试成功。
- `env PYTHONPYCACHEPREFIX=/tmp/basetion-python-cache python3 scripts/evaluate_test_results.py .basetion/test-results/test-d25b3e3e4555105484daf6213f82007a.jsonl --output /tmp/basetion-evaluation.json`：通过，历史样本 3 轮完成且无违规，生成评测报告。
- `env GOCACHE=/tmp/basetion-go-cache go test -race ./src/internal/entry/cli ./src/internal/harness ./src/internal/infra/session`：通过。
- `env GOCACHE=/tmp/basetion-go-cache go test ./...`：通过。
- `env GOCACHE=/tmp/basetion-go-cache go vet ./...`：通过。
