# Tools 模块

## 覆盖路径

- `internal/tools`

工具层把模型可见协议适配到领域服务，不实现检索规则或存储逻辑。

当前注册两个工具，均由 Eino `InferTool` 从 Go 输入输出类型生成协议：

- `search_knowledge`：输入必填 `query` 和可选 `limit`，输出领域 `Document` 列表。
- `teamops`：以单一工具提供 `describe` 和 `query` 两种模式。`describe` 按需返回 Lua 运行时和 TeamOps 模块目录；`query` 执行定义了 `main(teamops)` 的受限 Lua 5.1 程序。工具协议将 `mode` 限定为两个枚举值，所有校验和执行交给 TeamOps 领域服务。

构造工具时对应领域服务不能为空。领域错误直接返回给 Agent 工具运行链路，由上层事件与回调机制处理。
