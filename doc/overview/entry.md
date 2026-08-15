# 入口模块

## 覆盖路径

- `src/cmd/basetion`
- `src/internal/entry/cli`

## 进程入口

`src/cmd/basetion/main.go` 使用后台 Context 调用 `bootstrap.Execute`，传入命令行参数、标准输出和标准错误，并以返回值作为进程退出码。业务依赖不在 main 包中构造。

仓库根目录的 `justfile` 提供默认的 `run` 配方；执行 `just` 或 `just run` 会通过 `go run ./src/cmd/basetion "介绍一下你能做什么？"` 启动一个使用新 Session 的固定提示词 demo 会话。

## CLI 协议

CLI 要求至少一个非空提示词参数，`--session-id <ID>` 可选。未提供 Session ID 时，入口层使用密码安全随机数生成 `session-<32 位十六进制>` ID，将新 ID 输出为 `[会话] 新建 <ID>`，再运行会话；显式提供 ID 时用于继续对应 Session。参数错误或进程配置初始化失败返回退出码 2；其他依赖初始化错误、会话 ID 生成失败或会话运行错误返回 1；成功返回 0。提示词由剩余位置参数用空格连接并去除首尾空白。

`Conversation` 接口是入口层所需的最小应用契约：按 Session ID 和提示词启动一轮对话，返回 Eino typed AgentEvent 异步迭代器。

## 输出渲染

`Render` 顺序消费事件且保留 `AgenticMessage.ContentBlocks` 的语义：

- reasoning 摘要标记为 `[思考]`。
- 助手文本直接输出。
- 工具调用与结果显示工具名、call ID 和对应内容。
- 中断、退出、循环结束、Agent 转交等动作显示为 `[动作]`。
- 事件流正常关闭后输出 `[完成]`；事件错误立即返回给 CLI。

入口层不负责 Session、模型或工具业务规则。
