# 统一球队工具发现协议与批量评测产物

## 背景

球队读取、Lua 查询和预定义修改原先各自携带 describe 分支，模型在调用前需要选择 mode；修改工具已经被拆成两个名称，工具清单变成五项。批量测试结果则按 UUID 命名 JSONL，评测 JSON 与 Markdown 的位置需要由调用者推断，缓存命中 token 也没有进入评测数据。OpenAI Responses 的请求头和响应存储选项无法从配置表达。

## 变更

- 模型可见球队协议收敛为 `team_describe`、`team_fetch`、`team_query`、`team_modify` 四项。`team_describe` 接受 `fetch.*`、`query.*`、`modify.*` topic，可一次读取不同目录的说明；其余三项只执行读取、Lua 或确认后的批量修改。
- 批量测试的默认产物改为 `.basetion/test-results/<用例名-时间戳>/log.jsonl`。评测脚本遇到 `log.jsonl` 时在同目录生成固定名称的 `evaluation.json` 与 `report.md`。
- Trace 和评测报告新增 `cached_tokens`。它是 prompt token 的缓存命中细分，JSON、Markdown 和终端摘要都会展示，不重复计入 total token。
- `openai` 配置新增 `custom_headers` 和 `disable_response_storage`，前者传给 Responses 请求，后者显式关闭服务端响应存储；诊断数据只记录请求头数量，不记录值。
- 仓库协作规范调整为：未限定范围的提交包含工作区全部活跃文件，并在提交前确认完整范围。

## 影响模块

### 工具层

- 路径：`src/internal/tools/{team_describe.go,team_fetch.go,team_query.go,team_modify.go}`。`NewTeamTools` 以共享的修改 operation 目录装配四个工具；`team_describe` 为 fetch、query、modify 的同名 topic 加来源前缀。原 `mode`、`topics` describe 分支和 `team_modify_describe`、`team_modify_execute` 工具名不再暴露。`team_modify` 仍要求 `confirmed=true`、非空 operations、最多 50 项、顺序执行和失败后不回滚。
- 路径：`src/internal/bootstrap/app.go`。组合根将四项工具注册到 Harness，保证模型只看到统一后的协议。

### Harness、配置与 Skill

- 路径：`src/internal/harness/{model.go,trace.go}`、`src/internal/config/config.go`、`config/config.example.toml`。Responses 配置接收自定义请求头和关闭响应存储选项；`TurnTrace` 写入每次请求的 `cached_tokens`，以供入口 JSONL 和离线评测使用。现有 API Key 环境变量约定、状态栏和 Telemetry 汇总保持不变。
- 路径：`skills/{query-team-data,manage-roster,manage-game-setup,record-game,manage-training,project-knowledge}/SKILL.md`。各 Skill 改为先调用 `team_describe` 的前缀 topic，再调用对应执行工具；读取和写入的领域能力、确认要求及软删除语义不变。

### 入口、评测与测试

- 路径：`src/internal/entry/cli/test_mode.go`、`justfile`、`scripts/evaluate_test_results.py`。未指定输出路径的批量测试按用例名和启动时间建立目录并写入 `log.jsonl`；评测脚本据此使用固定产物名，并汇总 cached、prompt、completion、total token。评测仍不连接数据库或调用模型，违规与格式错误退出码保持不变。
- 路径：`src/internal/tools/*_test.go`、`src/internal/harness/*_test.go`、`src/internal/entry/cli/test_mode_test.go`、`scripts/test_evaluate_test_results.py`、`config/test/game-input.toml`。测试覆盖四项工具注册、跨目录 describe、执行工具 Schema、批量修改语义、缓存 token 捕获与汇总、固定评测产物路径，并把完整比赛用例的预期工具更新为 `team_describe` 与 `team_modify`。

### 文档与协作规范

- 路径：`AGENTS.md`、`doc/overview/{README.md,tools.md,harness.md,bootstrap-and-config.md,entry.md,infrastructure.md}`。协作规范定义未限定提交的工作区范围；overview 记录四工具协议、配置边界、缓存 token Trace 和新的批量产物布局。

## 验证

- `env GOCACHE=/tmp/basetion-go-cache go test ./...`：通过。
- `env PYTHONPYCACHEPREFIX=/tmp/basetion-python-cache python3 scripts/test_evaluate_test_results.py`：通过，6 个测试全部通过。
- `git diff --check`：通过。
