# 优化 Team Query Skill 编排与结果投影

## 背景

`team_query` 已经允许一个 Lua 程序调用多个只读模块。原有 `manage-team` Skill 只写了 describe 和 query 的基本顺序，没有稳定的叶子 topic 目录，也没说清跨模块依赖应该留在同一个程序里处理。实际运行时，模型可能查完球队拿到 ID，随后另起一次查询，平白增加推理轮次和工具调用成本。

另一个浪费出现在 Lua 返回值上。原有规则没有约束 `main` 的结构，Repository 转换出的完整领域对象可能被原样带回模型上下文，其中包括 UUID、版本、时间戳、关联键、候补阵容和逐条重复字段。最终回答只会用到一小部分。

## 变更

### 查询编排

- `manage-team` 直接列出已接入的八个查询叶子及其用途。模型根据用户意图选齐必要 topic，用一次精确 `team_query.describe` 取得参数和返回协议，默认流程不再绕到根目录。
- 对不需要模型根据中间结果重新作语义判断的查询，要求在一次 `team_query.query` 中启用全部必要模块，并在同一个 Lua `main(team)` 内完成 ID 查找、跨模块关联、过滤、排序、聚合和去重。仅为取得下一次调用所需 ID 不再是拆分查询的理由。
- 根目录 describe 仍可使用，适用场景收窄为三种：Skill 目录无法确定能力、精确 topic 未找到、用户主动探索能力。工具协议和 Lua API 没有改动。

### 查询结果投影

- Lua 查询取得的原始领域对象留在脚本内部参与计算，不能直接作为 `main` 返回值。返回 table 需要单独构造，只复制当前回答或紧随其后的修改操作会用到的字段。
- 没有后续修改时，结果省略实体 ID、关联 ID、版本及创建/更新时间；ID 已经映射成球队名、球员名或业务标签后，不再同时输出原始 ID。
- 大量明细在 Lua 中完成过滤、分组、聚合和重复模式压缩。若用户要求完整明细，每条业务事实照常保留，无关的重复字段不跟着返回。
- `manage-team` 中加入两个组合查询示例。名单与训练示例把球队名关联到球员和当日训练记录，返回球队名、球员名、日期、内容和感想。比赛示例过滤候补阵容，打席部分压缩成顺序、局次、上下半局、打者姓名和结果说明。

### 修改编排

- `manage-team` 补齐 `team_modify` 当前支持的操作目录。查询叶子与修改 operation 都能从 Skill 确定时，允许两个精确 describe 在同一模型轮次并行执行。
- 修改确认、批量上限、非原子执行、遇错中止和禁止自动重试等既有语义保持不变；本次没有修改 `team_modify` 的输入输出协议或领域写服务。

## 影响模块

### Harness 与 Skill 层

- `skills/manage-team/SKILL.md`
  - `查询流程` 原本只规定 describe 与 query 的调用顺序，现在补上查询能力目录、精确 describe、单次组合 query 和根目录回退规则。
  - 新增 `查询结果投影`，规定 Lua 原始对象与面向回答的返回结构之间的边界。
  - 新增名单加训练、比赛加阵容/打席示例，展示如何在 Lua 内消费领域对象并返回最小证据。
  - `修改流程` 新增 operation 目录及查询/修改 describe 的并行规则，但不改变确认和执行安全要求。
- `skills/project-knowledge/SKILL.md`
  - 将“比赛、阵容、训练尚未接入”的旧描述改为 `roster`、`game`、`lineup`、`training` 均已接入，避免项目知识与实际 Bootstrap 装配能力冲突。
- `src/internal/harness/prompts/system.md`
  - 系统指令新增 Skill 加载时序：加载 Skill 的响应不得同时调用该 Skill 管理的工具，必须先等待 Skill 内容进入上下文。该规则确保后续 describe/query 能遵循刚加载的目录与投影要求。

这些规则作用于模型编排和 Lua 返回结构。`team_query` 工具 Schema、Lua 模块函数、领域查询服务、Repository、数据库结构均未改动，也没有引入固定的 Agent Result Schema 或结果字节上限。

### 入口与人工轨迹用例

- `just/test.just`
  - 新增 `just test temp`，使用“最近一场已结束比赛、双方首发和全部打席”的跨模块问题运行开发 profile。
  - 该提示会同时用到 `roster`、`game`、`lineup`，从轨迹里可以直接检查组合 query 的次数，也能看出返回值是否夹带候补阵容和内部字段。

### 测试保障

- `src/internal/harness/skills_test.go`
  - `TestRepositorySkillsContainRequiredGuidance` 新增查询叶子、修改 operation、精确批量 describe、禁止 ID 接力查询和根目录回退规则的内容断言。
  - 新增结果投影断言，要求 Skill 包含原始对象不得直返、最小证据、无修改时省略 ID、聚合压缩等规则，并检查两个示例创建新的投影 table。
  - 增加反向断言，防止示例重新出现 `player = target_player` 或直接返回 `team.game.plates(...)`。
- `src/internal/harness/runtime_test.go`
  - Runtime 系统提示断言改为检查“等待 Skill 结果”时序规则。
  - Skill 工具结果断言同步为新的“一次批量 describe”措辞。测试仍验证 Runtime 注入系统提示、加载 Skill 并注册 `team_query` 的既有链路。

### 文档与协作规范

- `doc/overview/harness.md`
  - 现状文档补充 Skill 调用时序，以及 `manage-team` 的精确 topic 选择、单次 Lua 组合查询和最小结果投影职责。
- `doc/overview/entry.md`
  - 测试命令目录加入 `just test temp`，并说明它用于观察跨模块查询编排和结果投影。
- `AGENTS.md`
  - 强化后续 `doc/changes` 规范：记录必须按架构层或职责分组，写明具体路径、关键落点、行为差异及层间依赖，不能只罗列路径或给出泛泛总结。
  - 数据库、公开协议、错误语义、事务并发、配置或兼容性变化必须单独说明；提交前增加“不看 diff 也能理解影响范围”的检查项。
  - changes 初稿写完后调用 `remove-ai-flavor` Skill 审校中文表达。审校只处理模板化句式、机械排比和泛泛总结，路径、类型、协议、验证结果及必要分层不得删减；Skill 不可用时需要在提交前说明。
- `doc/changes/2026-08-17-1654-team-query-skill-projection.md`
  - 本文件按新规范详细记录本次提交的分层落点、行为差异、依赖关系与保持不变的边界。

## 验证

- `go test ./src/internal/harness`：通过，Harness 定向测试覆盖新的系统提示、Skill 编排规则和投影示例约束。
- `go test ./...`：通过，应用、领域、入口、Harness、PostgreSQL 基础设施及工具层测试均无回归。
- `python3 /Users/wuzihou/.codex/skills/remove-ai-flavor-writing/scripts/audit_ai_flavor.py doc/changes/2026-08-17-1654-team-query-skill-projection.md`：通过，`score=0`、`findings=0`、`blockers=0`。
- `git diff --check`：通过，没有空白错误。
- `just test temp`：通过。真实模型轨迹独立加载 `manage-team`；一次 describe 精确加载 `roster.teams`、`roster.players`、`game.matches`、`game.match`、`game.plates`、`game.score`、`lineup.list`；一次 query 启用 `roster`、`game`、`lineup`。
- 该真实 query 在同一个 Lua 程序内完成球队与比赛定位、球队/球员名称映射、比分读取、首发过滤和打席投影。工具结果没有 UUID、版本、创建/更新时间、候补阵容、投手 ID、跑者 ID、投球序列或逐打席比分快照；用户要求的 57 条打席仍全部保留，每条仅包含顺序、局次、上下半局、打者姓名和结果说明。
