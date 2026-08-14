# 入口模块

## 覆盖路径

- `cmd/basetion`
- `internal/entry/cli`

## 进程入口

`cmd/basetion/main.go` 使用后台 Context 调用 `bootstrap.Execute`，传入命令行参数、标准输出和标准错误，并以返回值作为进程退出码。业务依赖不在 main 包中构造。

仓库根目录的 `justfile` 提供默认的 `run` 配方；执行 `just` 或 `just run` 会通过 `go run ./cmd/basetion --session-id demo "介绍一下这个项目"` 启动一个固定提示词的 demo 会话。

## CLI 协议

CLI 要求 `--session-id <ID>` 和至少一个非空提示词参数。参数错误返回退出码 2；初始化或运行错误返回 1；成功返回 0。提示词由剩余位置参数用空格连接并去除首尾空白。

`Conversation` 接口是入口层所需的最小应用契约：按 Session ID 和提示词启动一轮对话，返回 Eino typed AgentEvent 异步迭代器。

## 输出渲染

`Render` 顺序消费事件且保留 `AgenticMessage.ContentBlocks` 的语义：

- reasoning 摘要标记为 `[思考]`。
- 助手文本直接输出。
- 工具调用与结果显示工具名、call ID 和对应内容。
- 中断、退出、循环结束、Agent 转交等动作显示为 `[动作]`。
- 事件流正常关闭后输出 `[完成]`；事件错误立即返回给 CLI。

入口层不负责 Session、模型或工具业务规则。
