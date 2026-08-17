# 入口模块

## 覆盖路径

- `src/cmd/basetion`
- `src/internal/entry/cli`

## 进程入口

`src/cmd/basetion/main.go` 使用后台 Context 调用 `bootstrap.Execute`，传入命令行参数、标准输出和标准错误，并以返回值作为进程退出码。业务依赖不在 main 包中构造。

仓库根目录的 `justfile` 通过 `app`、`dep` 与 `test` 三个 Just 模块分别组织应用、长期运行依赖和测试命令；模块源文件位于 `just/`。可使用 `just app run|dev`、`just dep up|down` 与 `just test all|player|player-add|game|training|training-mod|temp` 调用；其中训练命令使用开发 profile 演示近期自训分析与新增自训记录，`temp` 用一次跨模块比赛查询观察精确 describe、组合 Lua query 与最小结果投影。模块命令会切换到仓库根目录后再执行；应用命令不会隐式启动或停止依赖。

## CLI 协议

CLI 要求至少一个非空提示词参数，`--profile run|dev` 选择启动数据库配置并默认 run，`--session-id <ID>` 可选。profile 在组合根初始化配置前提取，其余参数由 CLI 解析。未提供 Session ID 时，入口层使用密码安全随机数生成 `session-<32 位十六进制>` ID，将新 ID 输出为 `[会话] 新建 <ID>`，再运行会话；显式提供 ID 时用于继续对应 Session。参数错误或进程配置初始化失败返回退出码 2；其他依赖初始化错误、会话 ID 生成失败或会话运行错误返回 1；成功返回 0。

`Conversation` 接口是入口层所需的最小应用契约：按 Session ID 和提示词启动一轮对话，返回 Eino typed AgentEvent 异步迭代器。

## 输出渲染

`Render` 顺序消费事件且保留 `AgenticMessage.ContentBlocks` 的语义：

- reasoning 摘要标记为 `[思考]`。
- 助手文本先按流式块累计；终端输出使用 Glamour 渲染 Markdown，非终端 writer 保留原始 Markdown，并统一加 `[回复]` 标记。
- 工具调用与结果显示工具名、call ID 和对应内容。
- 中断、退出、循环结束、Agent 转交等动作显示为 `[动作]`。
- 事件流正常关闭后输出 `[完成]`；事件错误立即返回给 CLI。

入口层不负责 Session、模型或工具业务规则。
