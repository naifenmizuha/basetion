# Infrastructure 模块

## 覆盖路径

- `src/internal/infra/session`
- `src/internal/infra/postgres`
- `src/internal/infra/teamquery`

## Session 文件存储

`FileStore` 将每个已完成 Session 保存为独立 JSON 快照。文件名是 Session ID 的 SHA-256 十六进制摘要，避免直接使用外部 ID 作为路径。加载时会校验文件中的 ID 与请求 ID 一致，并把缺失文件映射为应用层的 `ErrSessionNotFound`。

保存流程会以 0700 创建目录，在同目录创建 0600 临时文件，写入格式化 JSON、同步并关闭后通过 rename 原子替换目标快照。提交前失败会清理临时文件；Context 在读取、写入和提交前均会检查。

## PostgreSQL 存储

`Store` 使用 pgx 连接 PostgreSQL，并按文件名顺序执行内嵌迁移；迁移账本与 advisory lock 防止重复或并发执行。它实现球队、球员、名单、比赛、阵容、打席和自训记录 Repository，以及名单和比赛 `UnitOfWork`。业务读取均恢复领域对象并排除软删除数据；名单写操作维护当前球衣唯一和效力期不重叠，比赛删除在事务内显式软删除关联阵容与打席，球员删除在事务内显式软删除名单关系和自训记录。`AuditRoster` 可检查名单数据异常。

比赛迁移为 Match 保存状态，为 Plate 保存可空的主客队比分快照；旧 Plate 保持比分未知。自训记录使用 `DATE` 保存训练日期、`TEXT` 保存内容和感想，并通过部分唯一索引保证每名球员每天最多一条有效记录；查询按日期倒序和 ID 稳定排序。开发 fixture 提供两支球队及名单、一场完整比赛，以及全部 40 名球员最近五天共 200 条自训记录，供查询与修改演示使用。

`ManagedStore` 支持固定库和临时库两种生命周期。固定库只拥有连接池；临时模式通过管理连接创建带随机后缀的数据库，执行迁移，可选载入内嵌开发 fixture，并在关闭时终止遗留连接后删除数据库。仓库根目录 `compose.yml` 只部署持久 PostgreSQL 依赖，不与应用或测试命令隐式绑定。

## Team Query Lua 运行时

`LuaExecutor` 使用 GopherLua 为每次查询创建独立 State，只开放裁剪后的 base、`table`、`string` 和 `math` 能力。文件、系统、模块加载、动态代码、调试、协程、channel、打印、随机数和元表修改均不可用。入口参数 `team` 以只读 userdata 代理暴露 `array()` 和 `null`，脚本无法替换宿主能力。执行器按查询声明暴露 `roster`、`game`、`lineup` 和 `training` 只读代理；代理调用领域 Query Service，再把领域对象转换为 Lua table。未声明、不可用或未知模块不会进入脚本环境。

默认限制为 2 秒、32 KiB 源码、32 层返回深度、10,000 个 table 元素和 256 KiB JSON 结果。返回转换支持 nil、布尔、有限数值、UTF-8 字符串与 table；连续正整数键映射为数组，字符串键映射为对象，并拒绝循环、稀疏或混合键及不可序列化值。
