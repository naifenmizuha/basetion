# 模型状态栏与轮次运行指标

时间：2026-08-20 10:24（本地时间）

## 背景

模型在工具循环中缺少当前用户、时间和上下文用量等运行时信息，命令行进程也只有 stderr 生命周期日志，无法按配置留存。一次回复往往包含多次模型请求和工具调用，结束后没有总 token 和工具次数记录。

## 变更

### 配置与组合根

- `src/internal/config/config.go` 和 `config/config.example.toml` 增加 `openai.context_window_tokens`、`user.name`、`log.file`。前三者的默认值分别为 128000、`Basetion 用户`、`.basetion/basetion.log`；窗口必须为正数，用户名和日志路径不能为空。配置诊断字段会显示这些非密钥运行值，仍不包含 API Key。
- `src/internal/bootstrap/app.go` 按 `log.file` 创建父目录并以追加模式打开 0640 日志文件。文件打开失败时以配置错误退出；成功后启动配置、Harness 生命周期和轮次汇总写入文件，CLI 错误继续输出到 stderr。数据库 profile、连接配置和迁移行为没有变化。

### Harness 与应用层

- `src/internal/harness/status_model.go` 用 `statusModel` 装饰 Eino `AgenticModel`。它复制每个模型请求输入，在末尾加入 system 状态栏，列出固定用户、RFC3339 时间、上一模型请求的精确 input token 和上下文窗口。第一请求显示“未知”；状态栏不修改输入切片，也不会写入业务 Session。
- `src/internal/harness/telemetry.go` 增加 `TurnTelemetry`。它在 Context 中保存一轮对话的模型请求、每次返回的 token usage 与工具次数，结束时将 prompt、completion、total token 的聚合值和 `completed`、`failed`、`cancelled` 状态写入 logger。`src/internal/harness/callbacks.go` 在每个工具开始调用时计数，包括动态 `skill` 工具。
- `src/internal/application/conversation/service.go` 定义可选 `TurnTelemetry` 端口并在每个已校验轮次前后调用它。Runner、Session 加载、Session 保存都使用该派生 Context；执行错误、取消和保存失败同样生成一条轮次汇总。既有 Session 成功后才提交的语义保持不变。

### 测试

- `src/internal/harness/status_model_test.go` 覆盖状态栏的临时注入、首次“未知”、后续模型调用读取上一请求 token，以及多次模型与工具指标汇总。
- `src/internal/harness/runtime_test.go` 覆盖真实工具循环中 `skill` 和 `team_query` 都计入轮次工具次数。
- `src/internal/config/config_test.go` 覆盖新增配置的读取、默认值和非法窗口、空用户、空日志路径的拒绝；`src/internal/bootstrap/app_test.go` 覆盖日志父目录创建和文件写入。

## 影响模块

### 配置与入口层

- `config/config.example.toml`、`src/internal/config/config.go`、`src/internal/bootstrap/app.go`：新增状态栏和日志所需的非密钥配置，Bootstrap 在配置初始化后依赖日志文件完成运行期观测。现有 API Key 环境变量、数据库 profile 和 CLI 参数不变；新增字段缺失时采用默认值，已有配置可继续加载。

### 应用层

- `src/internal/application/conversation/service.go`：会话服务新增可选运行遥测协作端口。它只传递轮次边界与结果，不持有 Harness 类型，也不改变 `SessionStore`、消息历史或同 Session 串行锁的职责。

### Harness 与模型工具

- `src/internal/harness/status_model.go`、`telemetry.go`、`callbacks.go`、`runtime.go`：Runtime 把模型输入装饰、token 汇总与工具计数结合到同一轮 Context。状态栏作为请求临时数据发给模型，但不进入 Session 持久化；聚合日志依赖 Bootstrap 提供的文件 logger。

### 测试与文档

- `src/internal/config/config_test.go`、`src/internal/bootstrap/app_test.go`、`src/internal/harness/status_model_test.go`、`src/internal/harness/runtime_test.go`：验证配置边界、文件日志、流式 usage 传播、工具循环计数和轮次汇总。
- `doc/overview/bootstrap-and-config.md`、`doc/overview/harness.md`、`doc/overview/conversation.md`：更新配置、日志、状态栏与遥测的现状说明；本文件记录本提交的可见行为和验证结果。

## 验证

- `go test ./...`：通过。
- `git diff --check`：通过，无空白错误。
