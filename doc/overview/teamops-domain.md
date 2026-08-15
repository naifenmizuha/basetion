# TeamOps 领域模块

## 覆盖路径

- `src/internal/domain/teamops`

## 查询契约

TeamOps 领域服务定义模型可见的可编程只读查询边界。`Describe` 返回 Lua 5.1 运行时约定和模块目录；未指定模块时返回完整目录，指定模块时按请求顺序过滤并去重，未知模块返回领域错误。

当前预声明 `roster`、`game`、`lineup`、`training`、`analysis` 五个能力模块。服务从 Executor 的能力列表标记可用性；当前组合根注入 PostgreSQL 名单读取器，因此 `roster` 可用，其余模块仍明确返回尚未接入。`Query` 验证请求模块后，将模块名与程序封装为 `Query` 交给 Executor。

`RosterReader` 是领域定义的只读投影端口，可列出球队，并按必填 `team_id`、可选日期和任一守备位置过滤名单球员。返回视图包含球衣号码、打击/投球手、守备位置、效力区间和当前有效状态，不向 TeamOps 查询暴露持久化实体。

查询程序不能为空且最多 32 KiB。校验通过后服务委托只读 `Executor` 抽象，并为执行错误补充 TeamOps 查询上下文。领域模块不依赖 Lua 实现、Eino 工具协议或外部 TeamOps 内部包。
