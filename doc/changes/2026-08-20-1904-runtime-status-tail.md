# 运行时状态栏尾置

## 背景

Harness 每次模型调用都会附加动态状态栏。该状态原先使用 system 角色；部分模型服务会合并或前置 system 内容，使随请求变化的状态破坏可复用的提示词前缀缓存。批量测试 Trace 还会保存完整输入和输出，其中包含临时状态栏与可能很长的工具载荷，既放大测试结果文件，也不符合运行态状态不进入持久化历史的边界。

## 变更

- `src/internal/harness/status_model.go` 的 `statusModel.withStatus` 仍复制调用方消息切片并在末尾附加当前状态，但改为 user 角色。状态文字将其标记为只读运行时元数据，不是用户指令或对话历史，且不得覆盖既有系统规则。每次 `Generate`、`Stream` 重新生成当前用户、时间、上一请求 prompt token 和上下文窗口；状态不写回调用方输入或 Session。
- `src/internal/harness/trace.go` 将 `TurnTrace` 的单次请求记录收窄为序号、token usage 与错误。`recordOutput` 只保留流式消息中 total token 最大的一份 usage，取消完整模型输入、状态栏和输出的 JSON 序列化与存储。
- `src/internal/harness/status_model_test.go` 覆盖状态栏始终为输入末尾的 user 消息、连续调用不夹带上次状态、时间及 token 实时更新，以及工具调用和工具结果之后仍保持原类型和内容、再追加状态栏。Trace 测试验证只保留 token usage 的最大快照。

## 影响模块

### Harness

- `src/internal/harness/status_model.go`：模型调用入口的临时状态从 system 消息改为末尾 user 元数据消息。它继续依赖 `TurnTelemetry` 取得上一请求的 prompt token，并在交给底层 Eino `AgenticModel` 前构造一次性输入，调用方消息、业务 Session 和工具循环都不接收状态栏写回。
- `src/internal/harness/trace.go`：`TurnTrace`、`ModelRequestTrace` 与 `ModelTokenUsage` 组成批量测试的诊断协议。原协议会序列化 `Input`、`StatusBar` 与 `Output`；现在只向入口层提供 token usage 和错误，避免测试 JSONL 持有完整对话或工具参数、结果。
- `src/internal/harness/status_model_test.go`：覆盖上述两条运行时边界，并确保已有的 `FunctionToolCall`、`FunctionToolResult` 专用消息不会被状态装饰器改写或压缩。

### 文档

- `doc/overview/harness.md`：更新状态栏角色、只读语义、尾置位置和 Trace 保留字段，使模块现状说明与 Harness 实现一致。

### 配置、数据库与公开工具协议

无。`openai.context_window_tokens`、Session 格式、工具调用参数与工具结果协议均未修改；本次不引入状态快照、`assistant.Extra` 或 `previous_response_id`。

## 验证

- `env GOCACHE=/tmp/basetion-go-cache go test ./src/internal/harness`：通过。
- `env GOCACHE=/tmp/basetion-go-cache go test ./...`：通过。
- `git diff --check`：通过。
- 未发起 DeepSeek 付费 A/B 请求；缓存命中是否由 512 恢复，留待后续正常测试报告确认。
