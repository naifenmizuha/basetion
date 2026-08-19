# 入口模块

## 覆盖路径

- `src/cmd/basetion`
- `src/internal/entry/cli`

## 进程入口

`src/cmd/basetion/main.go` 使用后台 Context 调用 `bootstrap.Execute`，将命令行参数和标准流交给组合根。仓库根目录 `justfile` 通过 `app`、`dep`、`db`、`test`、`sqlc` 模块组织命令：应用和长期依赖命令不会隐式调整数据库，`just db reset` 是明确重建固定开发库的命令，`just sqlc generate|check` 管理生成绑定。`just test complex` 直接使用已准备的开发 fixture，不会重置数据库。

## CLI 协议

CLI 要求至少一个非空提示词。`--profile run|dev` 选择 TOML 中的数据库配置，默认 run；`--session-id <ID>` 可选。未提供 Session ID 时生成 `session-<32 位十六进制>`，显式 ID 用于续接对应 Session。参数或配置错误退出码为 2，其余初始化和会话错误为 1，成功为 0。

## 输出渲染

`Render` 顺序消费 AgentEvent：reasoning 标记为 `[思考]`，助手文本按流式块累计并以 `[回复]` 输出，工具调用和结果显示工具名、call ID 与内容，动作显示为 `[动作]`，正常关闭输出 `[完成]`。入口层不负责 Session、模型或工具业务规则。
