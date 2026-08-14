# TeamOps 领域模块

## 覆盖路径

- `internal/domain/teamops`

## 查询契约

TeamOps 领域服务定义模型可见的可编程只读查询边界。`Describe` 返回 Lua 5.1 运行时约定和模块目录；未指定模块时返回完整目录，指定模块时按请求顺序过滤并去重，未知模块返回领域错误。

当前预声明 `roster`、`game`、`lineup`、`training`、`analysis` 五个能力模块，均处于不可用状态。`Query` 只允许不声明数据模块的纯 Lua 程序；请求任一占位模块时明确返回尚未接入，不生成演示数据。

查询程序不能为空且最多 32 KiB。校验通过后服务委托只读 `Executor` 抽象，并为执行错误补充 TeamOps 查询上下文。领域模块不依赖 Lua 实现、Eino 工具协议或外部 TeamOps 内部包。
