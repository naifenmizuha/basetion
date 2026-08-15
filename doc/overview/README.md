# 项目现状总览

本目录按模块记录 Basetion 当前代码的实际逻辑。这里是长期维护的现状说明，不记录单次修改历史；单次变更见 [`../changes/`](../changes/)。

## 架构与依赖方向

当前程序是一条 CLI Agent 垂直链路：

```text
src/cmd/basetion
  -> src/internal/bootstrap
    -> src/internal/infra/postgres
      -> src/internal/domain/{player,team,roster}
    -> src/internal/entry/cli
      -> src/internal/application/conversation
        -> src/internal/harness (Eino Runner + Skill Middleware)
          -> skills/{project-knowledge,manage-teamops}
          -> src/internal/tools
            -> src/internal/domain/teamops
              <- src/internal/infra/teamops <- src/internal/infra/postgres
        -> src/internal/infra/session
```

入口层只处理协议与展示；应用层定义一次对话的业务边界；Harness 负责 Agent、Skill 与工具编排；工具层适配模型协议到领域服务；领域层定义业务规则和抽象；基础设施层实现持久化与执行适配器。`src/internal/bootstrap` 是唯一负责装配具体实现的组合根。

## 模块索引

- [`entry.md`](entry.md)：进程入口与 CLI 参数、事件渲染。
- [`bootstrap-and-config.md`](bootstrap-and-config.md)：配置加载及依赖装配。
- [`conversation.md`](conversation.md)：业务 Session、并发控制和对话事务。
- [`harness.md`](harness.md)：AgenticModel、Skill Middleware、Eino Agent/Runner 和生命周期回调。
- [`tools.md`](tools.md)：模型工具协议适配。
- [`roster-domain.md`](roster-domain.md)：球员、球队、名单实体、领域服务和事务端口。
- [`teamops-domain.md`](teamops-domain.md)：TeamOps 可编程只读查询契约与名单读取端口。
- [`infrastructure.md`](infrastructure.md)：Session 文件存储、PostgreSQL 存储与 Lua 查询运行时。

## 跨模块运行流程

1. `src/cmd/basetion` 将进程参数和标准流交给组合根。
2. 组合根解析 run/dev profile 并加载配置，创建或连接 PostgreSQL、执行迁移，再创建 Session 存储、TeamOps Lua 执行器、领域服务、工具、模型、Harness 和会话服务。
3. CLI 校验 `--session-id` 与提示词，调用会话服务并消费异步事件。
4. 会话服务按 Session ID 串行化运行，载入已完成历史并调用 Eino Runner。
5. Runner 流式产生 reasoning、文本、工具调用、工具结果和动作，CLI 按语义块展示。
6. 只有运行正常结束且上下文未取消时，新消息才以原子文件替换方式提交到业务 Session。

## 当前边界

- 仅实现 CLI 入口，没有 HTTP 或机器人入口。
- 项目知识和 TeamOps 操作规范由仓库根目录的两个必需 Skill 按需加载；当前不提供通用文件读取或生产知识检索。
- 业务 Session 使用本地 JSON 快照；Eino Checkpoint 尚未启用。
- 模型使用兼容 OpenAI Responses API 的 Eino `AgenticModel` 实现。
- TeamOps 当前接入只读 `roster` 模块；比赛、阵容、训练和分析模块仍为不可用占位。
