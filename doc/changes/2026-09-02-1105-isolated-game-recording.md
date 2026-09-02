# 内部比赛录入会话与可观测性

## 背景

完整比赛的阵容与逐打席参数过长，直接暴露给外层 Agent 会挤占其他意图的上下文；此前 `team_modify` 的 `game.create` 路径也让模型在一次调用中容易自造字段。测试日志缺少模型回答、思考及工具载荷预览，难以定位录入失败。开发数据库脚本同时依赖人工安装的 Python 包，批量评测不能解析 Go 写出的纳秒 RFC3339 时间。

## 变更

- Harness/Skill：新增 `src/internal/harness/game_recording_runner.go`、`game_recording_visibility.go`、`game_create_visibility.go` 与 `game_create_context.go`。外层仅在成功加载 `record-game` Skill 后的一次模型请求中暴露小参数 `team_game_create({confirmed})`；调用后同步启动内部 `game_recording_session`。内部会话只继承该轮所有用户文本，不继承助手消息、工具结果或 Schema，并按 `begin`、`append/finalize` 三阶段渐进暴露工具。临时 intake 使用随机 `adjective-animal` ID，只存在嵌套运行内；未 finalize、取消或出错时直接清除，finalize 才调用既有比赛写服务一次。Skill 改为外层确认和委派入口，完整逐打席映射改至内部系统提示，包含双杀、牺牲触击、牺牲飞球和本垒打 few-shot。
- 工具层：新增 `src/internal/tools/team_game_create.go` 和 `game_recording_tools.go`。`team_modify` 及 describe 目录删除 `game.create`。内部 begin 接收元数据和首发，append 接收连续 Play，finalize 合并后调用 `CreateCompactGameRecord`；append 以严格 JSON 解码拒绝未知字段，校验协议层必填项与枚举。紧凑 record 类型补充 JSON Schema 说明，明确可选字段应省略而非填空字符串、0 或 false。业务领域校验、partial 和失败语义仍由原有写服务决定，不引入 linter 或自动重试。
- 应用与组合根：`src/internal/application/conversation/service.go` 新增短生命周期 TurnScope，并在持久化 Session 前移除外层 `team_game_create` 调用/结果；内部 Play 和 intake 不会进入 Session。`src/internal/bootstrap/app.go` 先构建模型和内部 Runner 后注入工具，并把日志同时写入文件与终端 stderr。
- 日志与评测：`src/internal/harness/callbacks.go` 记录截断至 1600 rune 的模型 reasoning/回答及工具参数/结果预览，完整调试载荷仍受 unsafe 开关控制。嵌套 Agent 共用当前 Context 的 telemetry 和 Trace，因此报告 token/请求总数聚合外层和子会话。`scripts/evaluate_test_results.py` 接受 Go 的纳秒 RFC3339；测试用例同步覆盖。`scripts/manage_dev_db.py` 改为带 uv 内嵌依赖的 Python 3.11 脚本，`just/db.just` 用 `uv run --script` 调用。
- 测试配置与文档：`config/test/game-input.toml` 的路径预期更新为 `team_game_create`；更新 Harness、Tools、Conversation、Bootstrap/Config 与 Infrastructure overview。

## 影响模块

- Harness/Skill：`src/internal/harness/` 与 `skills/record-game/SKILL.md`。外层仍保留通用对话能力，比赛录入改为单次隔离的子会话；可见工具和上下文随会话阶段缩小，嵌套调用仍参与同一轮 telemetry/Trace。
- 工具层：`src/internal/tools/`。完整比赛写入不再通过 `team_modify` 的批操作协议，改为外层小参数委派和三个仅对子会话可见的工具；最终仍依赖领域 `GameModifier.CreateCompactGameRecord`。
- 应用层与入口层：`src/internal/application/conversation/`、`src/internal/bootstrap/`。会话服务提供临时消息快照和持久化过滤，组合根负责构造并连接嵌套 Runner；没有新增 application 子模块。
- 配置、开发工具与测试：`config/test/game-input.toml`、`just/db.just`、`scripts/manage_dev_db.py`、`scripts/evaluate_test_results.py` 及其测试。批量报告能够读取纳秒时间，开发数据库命令由 uv 管理脚本依赖。
- 文档：`doc/overview/` 与本文件。现状文档反映专用比赛录入、日志预览、Session 边界、uv 脚本和嵌套 token 聚合行为。

## 验证

- `env GOTOOLCHAIN=auto GOCACHE=/tmp/basetion-go-cache go test ./...`：通过。
- `env PYTHONPYCACHEPREFIX=/tmp/basetion-python-cache python3 -m unittest scripts/test_evaluate_test_results.py`：通过，7 项测试成功。
- `git diff --check` 与 `git diff --cached --check`：通过。
