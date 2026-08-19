# 项目现状总览

本目录按模块记录 Basetion 当前代码的实际逻辑；单次变更见 [`../changes/`](../changes/)。

## 架构与依赖方向

```text
src/cmd/basetion
  -> src/internal/bootstrap
    -> src/internal/infra/postgres
      -> src/internal/domain/{team,player,game,training}
    -> src/internal/entry/cli
      -> src/internal/application/conversation
        -> src/internal/harness
          -> skills/{project-knowledge,manage-team}
          -> src/internal/tools
            -> src/internal/domain/teamquery
              <- src/internal/infra/teamquery
                -> src/internal/domain/{team,player,game}
```

入口层处理协议与展示；应用层是一次对话的业务边界；Harness 编排 Agent、Skill 与工具；工具层适配模型协议；领域层定义规则和端口；基础设施实现持久化和 Lua 执行。`bootstrap` 是唯一组合根。

## 模块索引

- [`entry.md`](entry.md)：进程入口、Just 命令和 CLI 协议。
- [`bootstrap-and-config.md`](bootstrap-and-config.md)：配置加载和依赖装配。
- [`conversation.md`](conversation.md)：业务 Session、并发控制和对话事务。
- [`harness.md`](harness.md)：模型、Skill Middleware、Runner 和回调。
- [`tools.md`](tools.md)：模型工具协议适配。
- [`roster-domain.md`](roster-domain.md)：球队与球员实体、服务和查询。
- [`game-domain.md`](game-domain.md)：比赛、阵容、比赛过程及姓名化读模型。
- [`training-domain.md`](training-domain.md)：球员每日自训记录。
- [`teamquery-domain.md`](teamquery-domain.md)：可编程只读球队查询契约。
- [`infrastructure.md`](infrastructure.md)：Session、PostgreSQL、sqlc 与 Lua 运行时。

## 当前边界

- 只有 CLI 入口，没有 HTTP 或机器人入口。
- 会话使用本地 JSON 快照；Eino Checkpoint 尚未启用。
- `team_fetch` 提供四类完整比赛投影；`team_query` 可组合球队、当前球员、比赛和原子 Play，且不会向模型暴露内部比赛 ID；训练尚未进入新的姓名化 Lua 协议。
- `team_modify` 支持球队、球员、比赛、阵容和自训记录的预定义修改；球员没有转队能力。
