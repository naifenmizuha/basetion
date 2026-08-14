# Basetion

Basetion 是一个以 Go 和 CloudWeGo Eino ADK 为核心的 Agent 应用骨架。当前提供 CLI 入口；HTTP/QQ 机器人入口暂不实现，但未来可复用同一个会话应用服务。

## 职责划分

代码按六个职责区域组织，依赖方向始终从外向内：

1. `internal/entry/cli`：解析入口协议并渲染 Eino AgentEvent，不承载业务规则。
2. `internal/application/conversation`：管理业务 Session、并发锁和一轮对话的事务边界。
3. `internal/harness`：用 Eino `AgenticModel`、`TypedChatModelAgent[*schema.AgenticMessage]`、工具与 `TypedRunner` 编排 Agent 运行。
4. `internal/tools`：把模型工具协议适配到领域服务，校验并转换工具输入输出。
5. `internal/domain`：表达知识检索等领域能力，只依赖抽象的存储接口。
6. `internal/infra`：实现本地 Session 文件、内存检索器；将来可替换为数据库或向量库适配器。

入口只调用会话应用服务；会话服务使用 Eino Runner，但不理解模型或工具内部实现；Harness 调用工具；工具只调用领域服务；基础设施实现应用层或领域层定义的接口。智能匹配已有上下文若属于一次 Agent 运行策略，应放在 Harness；若决定业务 Session 中哪些历史可见，则由会话应用层制定策略、Harness 执行。

## 运行 CLI

需要 Go 1.22+。先复制示例配置：

```shell
cp config/config.example.toml config/config.toml
```

程序固定从当前工作目录读取 `config/config.toml`。配置按 `[openai]`、`[session]` 和 `[agent]` 分组；`openai.model` 与 `openai.api_key` 必填。实际配置文件可能包含密钥，已被 Git 忽略，请勿提交。

现有环境变量仍受支持并优先于 TOML 中的对应值：`OPENAI_MODEL`、`OPENAI_API_KEY`、`OPENAI_BASE_URL`、`BASETION_SESSION_DIR`、`BASETION_MAX_ITERATIONS`、`BASETION_UNSAFE_DEBUG_DATA`。建议使用 `OPENAI_API_KEY` 注入密钥，例如：

```shell
export OPENAI_API_KEY=your-api-key
```

运行一轮并在下一轮复用同一 Session：

```shell
go run ./cmd/basetion --session-id demo "介绍一下这个项目的骨架"
go run ./cmd/basetion --session-id demo "继续说明会话层"
```

CLI 会依次显示模型明确返回的可见思考摘要、工具调用、工具结果、最终回答和完成状态。默认 Callback 日志不记录提示词、私有推理或工具完整载荷；仅在 TOML 中设置 `agent.unsafe_debug_data=true` 或显式设置 `BASETION_UNSAFE_DEBUG_DATA=true` 才会输出调试载荷，请勿在生产环境开启。

## Session 与 Checkpoint

业务 Session 和 Eino Checkpoint 解决不同问题：

- 业务 Session 保存已经成功完成的对话历史，当前以摘要文件名存放在 `session.dir`（可由 `BASETION_SESSION_DIR` 覆盖），采用临时文件加原子重命名提交。失败或取消的轮次不会覆盖上一次快照。
- Eino Checkpoint 保存一次被中断、未来可能恢复的运行时状态。本骨架暂未启用 Checkpoint，也不会用它代替业务 Session。

测试与检查：

```shell
go test ./...
go vet ./...
```
