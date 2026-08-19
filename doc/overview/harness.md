# Harness 模块

## 覆盖路径

- `src/internal/harness`
- `skills`

## 模型与运行时

`NewAgenticModel` 从已初始化的全局配置快照读取模型名、API Key、可选 Base URL、reasoning effort 和 reasoning summary，创建 Eino OpenAI Responses `AgenticModel`。

`NewRuntime` 从同一配置快照读取最大迭代次数和调试开关，构造一个 `TypedChatModelAgent[*schema.AgenticMessage]`，注册工具、嵌入式系统指令和 Skill Middleware，再包装为启用流式输出的 `TypedRunner`。两个构造函数均不接收配置参数，要求 Bootstrap 先完成配置初始化。Harness 直接组合 Eino 的具体类型，不额外定义第二套运行时抽象。

系统指令通过 `go:embed` 从 `prompts/system.md` 编译进二进制，只保留身份、按需加载 Skill、禁止编造、事实与推断区分、默认语言和不输出私有推理等全局不变量。加载 Skill 的调用必须独立完成并等待结果，模型不得在同一响应中提前调用该 Skill 管理的业务工具。

Skill Middleware 使用 Eino Ext 本地文件 Backend，从进程当前工作目录的 `skills/` 扫描一级子目录中的 `SKILL.md`。启动时完整加载 frontmatter，拒绝目录缺失、格式错误、空名称或描述、重复名称以及必需 Skill 缺失；失败会终止 Runtime 初始化。Backend 仅交给 Skill Middleware，不向模型注册通用文件或 Shell 工具。

当前必需 Skill 为：

- `project-knowledge`：提供 Basetion 定位、当前能力、六层职责、Session/Checkpoint 区别和明确限制。
- `manage-team`：区分 `team_fetch` 的完整比赛读取和 `team_query` 的基础事实组合。前者直接读取摘要、记录、阵容或表现；后者在一次 Lua 执行中读取球队、球员、比赛和按实际比赛对象批量取回的 Play，并完成关联、过滤、聚合与最小结果投影。修改时先查询事实，一次加载所需修改说明并获得用户对完整修改批次的确认；多个可独立确定参数的修改合并为一次有序批量调用，并按非原子、遇错中止的结果语义处理失败与未执行步骤。

Skill 中间件动态提供模型可见的 `skill` 工具；业务工具列表显式注册 `team_fetch`、`team_query` 和 `team_modify`。两个 Skill 均为自包含单文件，当前不使用引用文件或通用文件读取能力。

## 生命周期回调

Runtime 同时提供 Agent、AgenticModel 和 Tool 生命周期回调。默认日志只记录组件、事件、名称以及消息或工具数量等元数据。

仅当 `agent.unsafe_debug_data=true`（或由 `BASETION_UNSAFE_DEBUG_DATA=true` 覆盖）时，回调才记录模型输入输出和工具完整载荷。logger 为空时使用丢弃输出的 logger，避免 nil 引用。
