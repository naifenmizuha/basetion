# Harness 模块

## 覆盖路径

- `internal/harness`

## 模型与运行时

`NewAgenticModel` 从已初始化的全局配置快照读取模型名、API Key 和可选 Base URL，创建 Eino OpenAI Responses `AgenticModel`。

`NewRuntime` 从同一配置快照读取最大迭代次数和调试开关，构造一个 `TypedChatModelAgent[*schema.AgenticMessage]`，注册工具和系统指令，再包装为启用流式输出的 `TypedRunner`。两个构造函数均不接收配置参数，要求 Bootstrap 先完成配置初始化。Harness 直接组合 Eino 的具体类型，不额外定义第二套运行时抽象。

默认指令要求助手基于对话上下文回答，在需要项目知识时调用 `search_knowledge`，并区分模型显式 reasoning 摘要与最终回答。

## 生命周期回调

Runtime 同时提供 Agent、AgenticModel 和 Tool 生命周期回调。默认日志只记录组件、事件、名称以及消息或工具数量等元数据。

仅当 `agent.unsafe_debug_data=true`（或由 `BASETION_UNSAFE_DEBUG_DATA=true` 覆盖）时，回调才记录模型输入输出和工具完整载荷。logger 为空时使用丢弃输出的 logger，避免 nil 引用。
