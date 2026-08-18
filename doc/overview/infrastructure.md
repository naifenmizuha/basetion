# Infrastructure 模块

## 覆盖路径

- `src/internal/infra/session`
- `src/internal/infra/postgres`
- `src/internal/infra/teamquery`

## Session 文件存储

`FileStore` 将每个已完成 Session 保存为独立 JSON 快照。文件名是 Session ID 的 SHA-256 十六进制摘要，避免直接使用外部 ID 作为路径。加载时会校验文件中的 ID 与请求 ID 一致，并把缺失文件映射为应用层的 `ErrSessionNotFound`。

保存流程会以 0700 创建目录，在同目录创建 0600 临时文件，写入格式化 JSON、同步并关闭后通过 rename 原子替换目标快照。提交前失败会清理临时文件；Context 在读取、写入和提交前均会检查。

## PostgreSQL 存储

`Store` 使用 pgx 连接已经准备好的 PostgreSQL，并实现球队、球员、名单、比赛、阵容、比赛过程和自训记录 Repository，以及名单和比赛 `UnitOfWork`。它不创建数据库、不执行迁移、不载入 fixture；关闭时只关闭自身连接池。业务读取均恢复领域对象并排除软删除数据；名单写操作维护当前球衣唯一和效力期不重叠，比赛删除在事务内显式软删除关联阵容与比赛过程，球员删除在事务内显式软删除名单关系和自训记录。`AuditRoster` 可检查名单数据异常。

比赛过程由 `plays` 保存前后局面和打击结果，`pitches`、`play_runner_results`、`play_fielding_results` 分别保存逐球、跑垒和守备明细。自训记录使用 `DATE` 保存训练日期、`TEXT` 保存内容和感想，并通过部分唯一索引保证每名球员每天最多一条有效记录；查询按日期倒序和 ID 稳定排序。

数据库初始化属于仓库工具职责。`scripts/manage_dev_db.py` 通过 `tomli` 读取 `config/config.toml` 的 `database.dev.url`，通过 `psycopg` 创建固定开发库、维护 `schema_migrations`、按文件名顺序执行 `sql/migrations/`、清空 `sql/test/reset.sql` 中列出的业务表，并载入 `sql/development/` 的示例数据。其依赖固定在 `scripts/requirements.txt`，由 `just db create|init|reset|check|test-reset` 调用。PostgreSQL 集成测试也经该脚本在每个用例前后清空数据，fixture 用例通过脚本载入示例 SQL；测试不能并行执行。开发 fixture 提供蜀汉、曹魏两支各 20 人的虚构业余球队，一场蜀汉主场 5:4 获胜的比赛记录，以及 18 名球员近 14 天内不等频的 46 条位置相关自训记录。仓库根目录 `compose.yml` 只部署持久 PostgreSQL 依赖，不与应用或测试命令隐式绑定。

## Team Query Lua 运行时

`LuaExecutor` 使用 GopherLua 为每次查询创建独立 State，只开放裁剪后的 base、`table`、`string` 和 `math` 能力。文件、系统、模块加载、动态代码、调试、协程、channel、打印、随机数和元表修改均不可用。入口参数 `team` 以只读 userdata 代理暴露 `array()` 和 `null`，脚本无法替换宿主能力。执行器按查询声明暴露 `roster`、`game`、`lineup` 和 `training` 只读代理；代理调用领域 Query Service，再把领域对象转换为 Lua table。未声明、不可用或未知模块不会进入脚本环境。

默认限制为 2 秒、32 KiB 源码、32 层返回深度、10,000 个 table 元素和 256 KiB JSON 结果。返回转换支持 nil、布尔、有限数值、UTF-8 字符串与 table；连续正整数键映射为数组，字符串键映射为对象，并拒绝循环、稀疏或混合键及不可序列化值。
