# Tools 模块

## 覆盖路径

- `internal/tools`

工具层把模型可见协议适配到领域服务，不实现检索规则或存储逻辑。

当前只注册 `search_knowledge`：输入包含必填 `query` 和可选 `limit`，JSON Schema 向模型说明字段含义；输出为领域 `Document` 列表。工具由 Eino `InferTool` 从 Go 输入输出类型生成协议，调用时将参数交给知识领域服务校验和执行。

构造工具时领域服务不能为空。领域错误直接返回给 Agent 工具运行链路，由上层事件与回调机制处理。
