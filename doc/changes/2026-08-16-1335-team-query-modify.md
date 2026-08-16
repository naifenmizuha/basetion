# 球队查询与修改工具

## 背景

原 `teamops` 名称无法清楚区分可编程只读查询与业务状态修改，同时单独暴露大量写工具不利于模型按需发现。当前工作树还包含 Responses reasoning 配置、CLI Markdown 展示和开发名单 fixture 的配套改进，因此合并为一次完整提交。

## 变更

### 功能变化

- 将模型可见的原 `teamops` 工具拆分为职责明确的 `team_query` 和 `team_modify`，不保留旧工具名兼容入口。
- `team_query` 保留渐进式 `describe` 和可编程 `query`：模型先读取顶层或精确 topic，再用受限 Lua 5.1 执行只读过滤、组合与计算；Lua 入口由 `main(teamops)` 改为 `main(team)`，数据调用相应改为 `team.roster.*`。
- `team_modify` 通过 `describe` 渐进式公开修改目录，通过 `execute` 执行以下七项 operation：
  - `team.create`：创建启用状态的球队。
  - `player.create`：创建启用状态的球员，并设置打击手别、投球手别和守备位置。
  - `player.update`：更新指定球员的完整姓名、手别和守备位置资料。
  - `player.set_active`：启用或停用指定球员。
  - `roster.assign`：按球队、球员、球衣号码和加入日期创建效力关系。
  - `roster.change_jersey`：修改指定球队当前名单成员的球衣号码。
  - `roster.leave`：按离队日期结束指定球队的当前效力关系。
- 修改参数按 operation 解码到独立强类型结构，拒绝未知字段、缺失字段、非法日期以及未知手别或守备位置；球队、球员和效力关系的创建 ID 由工具生成 UUID。
- 所有修改调用都必须提供 `confirmed=true`。管理 Skill 要求先用 `team_query` 查询当前事实和稳定 ID，再加载 operation 参数、向用户复述完整修改并取得确认，成功后按需重新查询最终状态。当前版本不提供持久化幂等记录，结果未知的修改不得自动重试。
- OpenAI 配置新增 `openai.reasoning_effort` 和 `openai.reasoning_summary`，默认值分别为 `high` 和 `detailed`，并传递给 Responses API。
- CLI 聚合流式助手文本；连接终端时使用 Glamour 渲染 Markdown，非终端 writer 保留原始 Markdown。reasoning、工具调用和工具结果会先刷新待输出的助手文本，保持事件顺序。
- 开发 fixture 改为包含季汉队和二十名当前球员的完整名单；新增 `just test player-add`，演示一次确认后创建关兴、张苞并加入名单的请求。

### 架构变化

- 只读查询领域包和 Lua 基础设施从 `domain/teamops`、`infra/teamops` 重命名为 `domain/teamquery`、`infra/teamquery`，查询领域仍只依赖 `Executor` 和 `RosterReader` 抽象。
- `team_modify` 是工具适配器而不是新的应用服务或领域模块。它负责模型协议、操作目录、参数转换和调用分派，直接依赖既有 `team.Service`、`player.Service`、`roster.Service`；业务规则、Repository 访问、名单事务和乐观版本检查仍由领域层及 PostgreSQL 基础设施承担。
- Bootstrap 创建共享系统时钟，装配三个写领域服务以及 `team_query`、`team_modify` 两个工具。`src/internal/application` 仍只包含既有 conversation 外部用例边界。
- 必需 Skill 从 `manage-teamops` 重命名为 `manage-team`；Harness 同时注册两个球队业务工具，项目知识 Skill 和 README 同步使用新的能力名称。
- README 与受影响的 overview 文档同步为当前实现，`teamquery-domain.md` 替代原 `teamops-domain.md`。

## 影响模块

- `src/internal/{bootstrap,config,entry,harness,tools}`
- `src/internal/domain/{teamquery,team,player,roster}`
- `src/internal/infra/{teamquery,postgres}`
- `skills/`、`config/`、`just/`、`README.md`
- `doc/overview/`、`doc/changes/2026-08-16-1335-team-query-modify.md`

## 验证

- `GOCACHE=/tmp/basetion-go-cache GOTOOLCHAIN=auto go test ./...`：通过，包括 PostgreSQL 集成测试。
- `git diff --check`：通过。
