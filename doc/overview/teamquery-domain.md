# Team Query 领域模块

## 覆盖路径

- `src/internal/domain/teamquery`

## 查询契约

Team Query 领域服务定义模型可见的可编程只读查询边界。`Describe` 返回 Lua 5.1 运行时约定和渐进式 topic 目录；未指定 topic 时返回顶层目录，精确请求父 topic 时返回子项，叶子 topic 返回调用参数、结果字段和示例。

当前预声明 `roster`、`game`、`lineup`、`training`、`analysis` 五个能力模块。服务从 Executor 的能力列表标记可用性；当前组合根注入球队、名单、比赛和自训记录只读领域服务，因此除分析外均可用。`Query` 验证请求模块后，将模块名与程序封装为 `Query` 交给 Executor。

`roster.teams` 和 `roster.players` 读取球队与名单；`game.matches`、`game.match`、`game.plays`、`game.score` 读取比赛事实、完整比赛过程与比分快照；`lineup.list` 读取比赛阵容；`training.records` 按球员和可选起止日期读取每日自训记录。Team Query 不定义平行的业务 View 或 PostgreSQL Reader 端口，Lua 基础设施只负责把领域服务返回的对象转换为只读 table。

查询程序不能为空且最多 32 KiB。校验通过后服务委托只读 `Executor` 抽象，并为执行错误补充球队查询上下文。领域模块不依赖 Lua 实现或 Eino 工具协议。
