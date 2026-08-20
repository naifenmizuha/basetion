# Infrastructure 模块

## 覆盖路径

- `src/internal/infra/session`
- `src/internal/infra/postgres`
- `src/internal/infra/teamquery`

## Session 文件存储

`FileStore` 将每个已完成 Session 保存为独立 JSON 快照。文件名为 Session ID 的 SHA-256 摘要；加载时校验 ID。保存使用 0700 目录、0600 临时文件、同步及 rename 原子替换，失败时清理临时文件；缺失文件映射为 `ErrSessionNotFound`。

`MemoryStore` 是批量测试使用的进程内 `SessionStore`。它保留同一命名 run 的多轮上下文，但进程退出即丢弃，不会在配置的 Session 目录创建测试快照。

## PostgreSQL 存储与数据库工具

`Store` 使用 pgx 连接已准备好的数据库，提供球队、球员、训练和比赛 Repository，以及 Player/Game 的事务 UnitOfWork。`DirectGameRepositories` 将比赛写 Repository 和按名称、球队/背号批量解析的球队、球员 Repository 组合给完整比赛录入服务。删除关系由领域服务编排，Repository 将 `deleted_at` 作为默认读写边界。球员表直接保存 `team_id` 和 `jersey_number`，部分唯一索引保证未删除球员在同队背号唯一；`uq_teams_active_name` 保证未删除球队名称唯一，并把冲突映射为 `team.ErrNameOccupied`；没有 memberships 表或名单 Repository。比赛逐球明细表名为 `play_pitching_results`。

SQL 放在 `sql/queries/`，由 `sqlc.yaml` 为 pgx/v5 生成 `src/internal/infra/postgres/sqlcgen/`。`just sqlc generate` 显式生成绑定，`just sqlc check` 生成后检查该目录没有差异；应用和普通测试命令不会隐式生成。手写适配层只负责领域值与 sqlc 参数/行的转换、事务和数据库错误映射。

`scripts/manage_dev_db.py` 读取 `config/config.toml` 的固定 `database.dev.url`。`just db reset` 重建该库、按文件名顺序执行迁移和 development fixture，随后检查至少两支启用球队、一场已结束比赛、阵容和有效 Play；它会删除开发库全部数据。`just db check` 只验证现有 fixture。脚本不创建临时测试库，也不使用环境变量覆写连接配置。

`scripts/evaluate_test_results.py` 不读取配置、不连接数据库，也不调用模型。它只接受 schema version 2 的批量测试 JSONL：每个模型请求含最多一份 token usage 快照，事件保留可读调用轨迹。脚本根据事件中的函数调用归集业务工具与 `skill` 参数，将耗时、墙钟时间、token、观察路径、声明预期、警告和违规写入 `.evaluation.json`，并输出同名 `.report.md` 以展示 Prompt、可见思考、工具调用、工具结果和回复。测试失败、缺少预期工具或 Skill、禁用工具调用返回状态 1；JSONL 格式错误返回状态 2。

## Team Query Lua 运行时

`LuaExecutor` 为每次查询建立独立 GopherLua State，只开放裁剪后的 base、`table`、`string` 和 `math`；文件、系统、动态代码、模块加载、调试、协程、随机数、打印和元表修改均不可用。`data` 是只读 userdata 代理，提供 `array()`、`null` 和固定的 `team`、`player`、`game` 基础读取代理；训练服务仍可注入运行时，但不向模型目录或 Lua 代理发布。

`player.list` 调用 Player 查询服务；`game.list` 发布无 ID 的比赛投影，并将每个原始 Lua table 映射到仅当前 State 可用的内部比赛引用。`game.plays` 只解析这些原始 table，批量调用 Game 查询服务读取原子 Play；相同基础读取在同一次执行中复用缓存。默认限制还包括最多 8 次数据读取、单次 1,000 项和累计 5,000 项；调用预算在数据库读取前保留。总超时为 2 秒，源码上限 32 KiB，结果上限为 32 层、10,000 个元素和 256 KiB JSON；转换拒绝循环、稀疏/混合表和不可序列化值。
