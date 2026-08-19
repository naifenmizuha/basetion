# Team Query 领域模块

## 覆盖路径

- `src/internal/domain/teamquery`

## 查询契约

Team Query 是模型可见的可编程只读查询边界。`Describe` 返回 Lua 5.1 运行时约定和渐进式 topic 目录；根目录包含 `runtime`、`team`、`player`、`game`，精确请求父 topic 返回子项，叶子 topic 返回参数、结果字段和示例。叶子的 `available` 同时要求所属模块和 Executor 声明的具体 topic 可用，因此已注册模块不会把未实现能力误报为可调用。

当前叶子为 `team.list`、`player.list`、`game.list`、`game.summaries`、`game.records`、`game.lineups` 和 `game.performances`。球员和比赛接口使用姓名化投影；比赛详情、阵容和表现都接受统一的参与球队、日期范围和数量限制。训练查询尚未接入这个姓名化 Lua 协议。

`Query` 校验模块名、可用性、32 KiB 程序上限和非空程序，再将模块与程序交给 `Executor`。执行错误会附加球队查询上下文。领域模块只定义模型工具协议，不依赖 Lua 实现、PostgreSQL 或 Eino。
