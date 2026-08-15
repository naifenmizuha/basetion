# Basetion

Basetion 是一个以 Go 和 CloudWeGo Eino ADK 为核心的 Agent 应用骨架。当前提供 CLI 入口；HTTP/QQ 机器人入口暂不实现，但未来可复用同一个会话应用服务。

## 职责划分

代码按六个职责区域组织，依赖方向始终从外向内：

1. `src/internal/entry/cli`：解析入口协议并渲染 Eino AgentEvent，不承载业务规则。
2. `src/internal/application/conversation`：作为当前面向入口的应用用例门面，管理业务 Session、并发锁和一轮对话的事务边界。
3. `src/internal/harness`：用 Eino `AgenticModel`、Skill Middleware、工具与 `TypedRunner` 编排 Agent 运行。
4. `src/internal/tools`：把模型工具协议适配到领域服务，校验并转换工具输入输出。
5. `src/internal/domain`：表达球员、球队、名单和 TeamOps 只读查询等领域能力，并定义领域服务、Repository 与事务端口。
6. `src/internal/infra`：实现本地 Session 文件、PostgreSQL 球队数据存储和受限 Lua 查询运行时。

入口只调用会话应用服务；会话服务使用 Eino Runner，但不理解模型、Skill 或工具内部实现；Harness 按需加载仓库根目录 `skills/` 下的项目知识与 TeamOps 操作说明，并调用工具；工具只调用领域服务；基础设施实现应用层或领域层定义的接口。Skill 说明“怎么做”，TeamOps 工具和领域服务提供并约束“能做什么”。

## 运行 CLI

需要 Go 1.26.6+。先复制示例配置：

```shell
cp config/config.example.toml config/config.toml
cp .env.example .env
```

程序固定从当前工作目录读取 `config/config.toml`，并在存在时读取当前工作目录下的 `.env`。配置按 `[openai]`、`[database.run]`、`[database.dev]`、`[session]` 和 `[agent]` 分组。run profile 连接固定主库；dev profile 可连接固定开发库，或按进程创建临时数据库、执行迁移、载入演示名单，并在退出时删除。`api_key_env` 保存 API Key 所在环境变量的名称，真实密钥只写入被 Git 忽略的 `.env` 或进程环境，不写入 TOML。

除 API Key 外，运行配置只从 TOML 读取，不接受环境变量覆盖。进程环境中的 API Key 优先于 `.env` 中的同名值。也可以不创建 `.env`，直接由进程环境提供密钥，例如：

```shell
export OPENAI_API_KEY=your-api-key
```

先部署 PostgreSQL 依赖，再运行一轮并在下一轮复用同一 Session：

```shell
just deps-up
go run ./src/cmd/basetion --profile run --session-id demo "介绍一下这个项目的骨架"
go run ./src/cmd/basetion --profile run --session-id demo "继续说明会话层"
```

`just dev` 使用 dev profile；`just testplayer` 在 dev 数据库中执行球员查询演示。应用命令不会自动管理依赖容器，完成开发后可用 `just deps-down` 停止服务并保留数据。

CLI 会依次显示模型明确返回的可见思考摘要、工具调用、工具结果、最终回答和完成状态。默认 Callback 日志不记录提示词、私有推理或工具完整载荷；仅在 TOML 中设置 `agent.unsafe_debug_data=true` 才会输出调试载荷，请勿在生产环境开启。

`skills/` 是必需的核心运行资源。启动时会校验 `project-knowledge` 和 `manage-teamops`；目录缺失、Skill 格式错误或必需 Skill 缺失都会导致启动失败。项目知识问题按需加载 `project-knowledge`，球队结构化数据任务先加载 `manage-teamops`，再通过只读 `teamops` 工具执行。

## Session 与 Checkpoint

业务 Session 和 Eino Checkpoint 解决不同问题：

- 业务 Session 保存已经成功完成的对话历史，当前以摘要文件名存放在 TOML 的 `session.dir`，采用临时文件加原子重命名提交。失败或取消的轮次不会覆盖上一次快照。
- Eino Checkpoint 保存一次被中断、未来可能恢复的运行时状态。本骨架暂未启用 Checkpoint，也不会用它代替业务 Session。

测试与检查：

```shell
just test
go vet ./...
```
