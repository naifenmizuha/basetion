# Team Query 领域模块

## 覆盖路径

- `src/internal/domain/teamquery`

## 查询契约

Team Query 是模型可见的可编程只读查询边界。`Describe` 返回 Lua 5.1 运行时约定和渐进式 topic 目录；根目录包含 `runtime`、`team`、`player`、`game`，精确请求父 topic 返回子项，叶子 topic 返回参数、结果字段和示例。可用性由 Executor 声明的具体 topic 决定。

当前叶子为 `team.list`、`player.list`、`game.list` 和 `game.plays`。程序入口是 `main(data)`；不再选择模块。`game.list` 返回不含稳定 ID 的比赛投影，`game.plays` 只接受同一次执行中保留的原始比赛对象，基础设施按对象身份恢复内部比赛引用并批量取回 Play。伪造、复制或跨执行传递的对象不能充当引用。

`Query` 校验 32 KiB 程序上限和非空程序，再将程序交给 `Executor`。执行错误会附加球队查询上下文。领域模块只定义模型工具协议，不依赖 Lua 实现、PostgreSQL 或 Eino。摘要、完整记录、阵容和逐场表现属于 `team_fetch` 的预定义组合读取，不在本模块的 Lua topic 中。
