# Conversation 应用模块

## 覆盖路径

- `src/internal/application/conversation`

## 业务 Session

`Session` 保存 ID、完整的 `AgenticMessage` 历史以及创建/更新时间。它表示已成功完成的业务对话，与用于恢复中断运行的 Eino Checkpoint 是不同概念。

`SessionStore` 由应用层定义，只要求 `Load` 和 `Save`；不存在的 Session 通过 `ErrSessionNotFound` 表达。具体文件实现位于基础设施层。

## 一轮对话

`Service.Run` 先规范化并校验 Session ID 与提示词，然后异步执行：

1. 获取该 Session ID 的独占锁。
2. 加载历史；不存在时创建空 Session。
3. 将本轮用户消息追加到历史副本，并调用 typed Eino Runner。
4. 向调用方转发 AgentEvent，同时从流式消息副本收集待持久化消息。
5. 遇到 Runner 错误、流合并错误或 Context 取消时停止，不覆盖旧快照。
6. 正常结束后更新时间并保存完整消息历史。

`Service` 可接收 `TurnTelemetry` 端口。每个通过 ID 和提示词校验的轮次先以相同 Context 开始遥测，再运行 Runner；无论加载、执行、取消或保存在哪个阶段结束，都会传递最终错误给遥测端口。该端口不读取或修改消息，因此状态栏和运行指标不会写入 Session。

流式 `MessageStream` 通过 `Copy(2)` 分成公开消费流和内部收集流，避免入口渲染与持久化争用同一个流。

## 并发边界

`SessionLocker` 对相同 Session ID 的轮次串行化，不同 Session 可以并行。锁条目带引用计数，最后一个等待者释放后从注册表删除；返回的 unlock 函数可安全重复调用。
