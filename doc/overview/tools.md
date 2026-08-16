# Tools 模块

## 覆盖路径

- `src/internal/tools`

工具层把模型可见协议适配到领域服务，不实现领域规则或存储逻辑。

当前显式注册两个由 Eino `InferTool` 从 Go 输入输出类型生成协议的业务工具：

- `team_query`：提供 `describe` 和 `query` 两种模式。`describe` 渐进式返回 Lua 运行时与只读模块 topic；`query` 执行定义了 `main(team)` 的受限 Lua 5.1 程序。
- `team_modify`：提供 `describe` 和 `execute` 两种模式。`describe` 渐进式返回 `team`、`player`、`roster` 操作目录；`execute` 只接受预定义 operation、严格结构化 arguments 与 `confirmed=true`，不接受脚本或数据库语句。

`team_modify` 当前支持球队创建、球员创建/更新/启停以及名单入队/改号/离队；创建 ID 由工具生成 UUID。工具层只处理目录、参数枚举转换和分派，业务规则、事务与版本冲突仍由现有领域服务负责。领域错误直接返回 Agent 工具运行链路。`skill` 工具由 Harness 中的 Skill Middleware 动态提供，不属于本工具适配目录。
