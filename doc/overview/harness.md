# Harness 模块

## 覆盖路径

- `src/internal/harness`
- `skills`

## 模型与运行时

`NewAgenticModel` 从已初始化的全局配置快照读取模型名、API Key、可选 Base URL、reasoning effort 和 reasoning summary，创建 Eino OpenAI Responses `AgenticModel`。

`NewRuntime` 从同一配置快照读取最大迭代次数、调试开关、状态栏用户和上下文窗口，构造一个 `TypedChatModelAgent[*schema.AgenticMessage]`，注册工具、嵌入式系统指令和 Skill Middleware，再包装为启用流式输出的 `TypedRunner`。两个构造函数均不接收配置参数，要求 Bootstrap 先完成配置初始化。Harness 直接组合 Eino 的具体类型，不额外定义第二套运行时抽象。

模型实例在 Runtime 中由状态栏装饰器包裹。每次 `Generate` 或 `Stream` 都复制本次输入，并在末尾追加一条 user 角色的只读运行时元数据消息，内容包括 `user.name`、RFC3339 当前时间、同一业务轮次上一模型请求返回的精确 input token 与 `openai.context_window_tokens`。文案明确该消息不是用户指令或对话历史，不能覆盖系统规则；首次请求的已用上下文为“未知”。装饰器不改写调用方输入；状态栏只存在于本次模型请求，Session 不会持久化它。

系统指令通过 `go:embed` 从 `prompts/system.md` 编译进二进制，只保留身份、按需加载 Skill、禁止编造、事实与推断区分、默认语言和不输出私有推理等全局不变量。加载 Skill 的调用必须独立完成并等待结果，模型不得在同一响应中提前调用该 Skill 管理的业务工具。

Skill Middleware 使用 Eino Ext 本地文件 Backend，从进程当前工作目录的 `skills/` 扫描一级子目录中的 `SKILL.md`。启动时完整加载 frontmatter，拒绝目录缺失、格式错误、空名称或描述、重复名称以及必需 Skill 缺失；失败会终止 Runtime 初始化。Backend 仅交给 Skill Middleware，不向模型注册通用文件或 Shell 工具。

当前必需 Skill 为：

- `project-knowledge`：提供 Basetion 定位、当前能力、六层职责、Session/Checkpoint 区别和明确限制。
- `query-team-data`：只读球队、球员和比赛事实；完整比赛投影优先 `team_fetch`，自由组合基础对象和原子 Play 时使用 `team_query`。
- `manage-roster`：球队与球员资料、启用状态和背号。
- `manage-game-setup`：比赛安排、状态和阵容。
- `record-game`：一次 `game.create` 录入已结束比赛、双方首发和连续 Play。
- `manage-training`：球员每日自训记录。

Skill 中间件动态提供模型可见的 `skill` 工具；业务工具列表显式注册 `team_describe`、`team_fetch`、`team_query` 和 `team_modify`。各 Skill 均为自包含单文件，当前不使用引用文件或通用文件读取能力。

## 生命周期回调

Runtime 同时提供 Agent、AgenticModel 和 Tool 生命周期回调。默认日志只记录组件、事件、名称以及消息或工具数量等元数据。

`TurnTelemetry` 由会话服务为每个有效轮次创建并放入 Context。状态栏装饰器按模型请求保存最后一次 token usage，Tool 回调在工具开始时计数。轮次结束后，Telemetry 向 Runtime logger 写入 session ID、`completed`、`failed` 或 `cancelled` 状态、模型请求数、prompt/completion/total token 总数和工具调用数；失败与取消保留结束前已采集的指标。

仅当 `agent.unsafe_debug_data=true`（或由 `BASETION_UNSAFE_DEBUG_DATA=true` 覆盖）时，回调才记录模型输入输出和工具完整载荷。logger 为空时使用丢弃输出的 logger，避免 nil 引用。

批量测试可通过 Context 显式附加 `TurnTrace`。状态栏装饰器会为每次模型请求记录序号、该请求中 total token 最大的一份 token usage 快照、其中的 `cached_tokens` 与模型调用错误；不保存完整输入、临时状态栏或模型输出。普通运行未附加 Trace 时不会保留这些数据。Trace 由入口在轮次结束后写入测试 JSONL，不写入业务 Session、运行日志或 Telemetry 汇总。
