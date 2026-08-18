# 固定开发测试库改造设计

## 背景与目标

当前 `database.dev` 使用 `temporary` 模式。`bootstrap.Execute` 每次以 `--profile dev` 启动时，都会由 `src/internal/infra/postgres/database.go` 创建随机后缀 PostgreSQL 数据库、执行迁移、载入内嵌开发 fixture，并在进程退出时终止连接和删除数据库。PostgreSQL 集成测试也沿用同一套生命周期，每个测试创建并删除一个数据库。

这会带来大量 `CREATE DATABASE`、迁移和 `DROP DATABASE` 操作；测试或调试期间的数据库状态也无法在进程结束后保留。本次改造将开发测试库改为由开发者显式初始化、长期保留的固定数据库。运行时和测试运行时只连接已配置的数据库，不再拥有、创建或删除数据库。

本次范围：

- 把开发示例数据移到仓库根目录的 `sql/` 中，作为可直接审阅和执行的 SQL 文件。
- 在 `just` 中提供显式创建和重置固定开发测试库的指令；指令读取 `config/config.toml` 中选定 profile 的连接信息。
- 把 `database.dev` 改为固定库配置；运行 `--profile dev` 与 PostgreSQL 集成测试连接这个已创建的库。
- 删除应用和基础设施中的临时数据库生命周期管理代码，以及依赖该行为的测试。
- 保持运行时迁移、领域 Repository、事务和软删除语义不变。

不在本次范围：

- 修改生产 `database.run` 的建库、备份或运维流程。
- 把 PostgreSQL 集成测试降级为 mock 或内存测试。
- 修改 Docker Compose 的依赖生命周期；Compose 仍只提供长期运行的 PostgreSQL 服务。

## 目标行为

### 配置与运行时

`config/config.toml` 和 `config/config.example.toml` 都只为 `database.run` 与 `database.dev` 声明固定连接：

```toml
[database.dev]
url = "postgres://basetion:basetion@localhost:5432/basetion_dev?sslmode=disable&connect_timeout=5"
```

不再存在 `mode`、`admin_url` 或 `temporary_prefix`。`database.dev.url` 是开发和集成测试共同使用的固定库；开发者在第一次使用前显式执行初始化命令。真实 `config/config.toml` 仍被 Git 忽略，示例文件只给出可替换的本地地址。

`basetion --profile dev` 只连接 `database.dev.url`、创建 Repository 并在退出时关闭连接池。它不加载示例数据，不创建数据库，不执行迁移，也不会终止其他连接或删除数据库。`--profile run` 具有相同的连接池生命周期，只是选择 `database.run.url`。

版本化迁移由 Python 数据库管理脚本执行。应用进程不会修改 schema 或迁移账本，因此部署或启动前必须显式完成迁移。

### SQL 目录与固定库初始化

新增仓库根目录 `sql/`，用于存放可由 PostgreSQL 客户端执行的开发测试数据，而不是由 Go 二进制嵌入的运行时资源：

```text
sql/
  migrations/
    001_team_roster.sql
    002_game_soft_delete.sql
    003_game_status_score.sql
    004_training_records.sql
    005_game_plays.sql
  development/
    001_roster_demo.sql
    002_game_demo.sql
    003_training_demo.sql
  test/
    reset.sql
```

现有 `src/internal/infra/postgres/fixtures/001_roster_demo.sql`、`002_game_demo.sql`、`003_training_demo.sql` 原样迁移到 `sql/development/`；原先内嵌在 Go 二进制中的迁移文件迁移到 `sql/migrations/`。Python 脚本按文件名顺序应用未记录在 `schema_migrations` 中的迁移，再载入三份 development SQL。这样 schema 与示例数据都不再依赖进程启动，同时仍能为 `just test player`、`just test game` 和 `just test training` 提供相同的球队、比赛和训练记录。

`sql/test/reset.sql` 以显式 `TRUNCATE ... RESTART IDENTITY` 清空全部持久化领域表和 `schema_migrations` 以外的业务数据，且不包含 `DROP DATABASE`。它只供测试辅助逻辑和手动重置固定开发库使用；必须按外键依赖包含 `teams`、`players`、`memberships`、`matches`、`lineups`、`plays`、`pitches`、`play_runner_results`、`play_fielding_results`、`training_records` 等当前表。后续新增持久化领域表或迁移时，必须同步更新此文件。测试库是基础设施测试资源，因此这里的物理清空不改变领域层“业务数据只能软删除”的约束。

在 `just/db.just` 新增并由根 `justfile` 引入数据库维护配方：

- `create`：基于 `config/config.toml` 的 `database.dev.url` 显式创建目标数据库；目标已存在时报告成功而不删除已有内容。
- `init`：执行建库、按文件名顺序应用 `sql/migrations/*.sql`，清空业务表并按名称顺序执行 `sql/development/*.sql`。该命令用于首次准备可交互的开发测试数据。
- `reset`：执行 `sql/test/reset.sql` 后重新应用 development SQL，用于开发者主动恢复演示基线。
- `check`：在 `just test all` 开始 Go 测试前仅检查固定库可连接且全部迁移已记录；不创建和删除数据库。若库未初始化，输出包含 `just db init` 的可操作错误。
- `test-reset`：只执行测试业务数据清空，不载入 development SQL。

这些配方不硬编码主机、端口、用户名、密码或数据库名，也不以临时环境变量覆盖连接配置。`scripts/manage_dev_db.py` 使用 `tomli` 读取 TOML 的 `database.dev.url`，使用 `psycopg` 通过参数化 SQL 创建数据库、维护迁移账本并执行 SQL 文件；依赖版本固定在 `scripts/requirements.txt`。脚本不调用 `basetion` 或 Go 数据库基础设施，连接信息不作为命令参数传递。

`just` 只编排 Python 脚本和测试命令。它不会启动或停止 Compose 服务，继续要求开发者显式使用现有 `just dep up` / `just dep down` 管理 PostgreSQL 依赖。

### 集成测试隔离

固定库不再天然为空。`src/internal/infra/postgres/store_integration_test.go` 的测试辅助函数会从 `database.dev.url` 打开 `Store`，通过 Python 脚本在每个用例开始前执行 `test-reset`，并在用例结束后再次执行它，确保重复运行不会残留领域记录。当前该文件没有 `t.Parallel`，本次继续禁止该固定库测试并行执行，避免多个测试同时重置同一数据库。

`TestPostgresDevelopmentFixtures` 不再调用运行时的 `WithDevelopmentFixtures`。它在已清空的固定库上，通过 Python 脚本执行 `test-load-development`，然后继续断言球队、比赛和训练记录。这样 fixture 的内容、排序和可执行性仍受自动化验证，但 production binary 不再包含 fixture 加载功能。

`TestPostgresFixedStoreDoesNotOwnDatabase` 的语义在新模型中不再成立：没有 `ManagedStore`，所有调用方都只持有 `*Store`，`Store.Close()` 恒为关闭本连接池。删除该测试，并以集成测试辅助函数的关闭后重连检查覆盖“关闭连接池不会影响固定库可继续连接”的必要行为。

如果固定库不可连接、缺少权限、未完成迁移或初始化 SQL 执行失败，集成测试应直接失败并给出配置路径或对应 `just` 配方；不得因为固定库未准备而 `Skip`，避免 CI 或本地测试出现伪通过。

## 文件级修改清单

| 路径 | 修改 | 原因与依赖关系 |
| --- | --- | --- |
| `config/config.toml` | 将 `[database.dev]` 改为固定 `url`，删除临时库字段。 | 开发运行时和集成测试连接由开发者预先创建的库。该文件被忽略，仅影响本机。 |
| `config/config.example.toml` | 同步固定 `database.dev.url` 示例。 | 保持可复制的无密钥配置模板与解析结构一致。 |
| `src/internal/config/config.go` | 删除 `DatabaseMode`、`DatabaseProfileConfig.Mode/AdminURL/TemporaryPrefix`、临时前缀校验及对应诊断字段；将 run/dev 校验收敛为各自必填 URL。 | 配置层不再表达数据库所有权或生命周期，供 bootstrap、初始化命令和集成测试读取固定 URL。 |
| `src/internal/config/config_test.go` | 将 temporary 配置样例和校验用例替换为 run/dev URL 必填、未知字段拒绝及 `LoadDatabaseFile` 固定 URL 行为的断言。 | 与精简后的 TOML 契约一致，保留配置解码和错误聚合覆盖。 |
| `src/internal/bootstrap/app.go` | 移除按 mode 分支、`ManagedStore`、临时 fixture 选项和带超时的数据库清理；以选定 profile URL 调用 `postgres.Open`，defer 仅 `Store.Close()`。 | 组合根只装配已存在的 PostgreSQL，不能再创建、删除或拥有数据库。 |
| `src/internal/infra/postgres/database.go` | 删除整个文件，包括 `ManagedStore`、`OpenFixed`、`OpenTemporary`、随机命名、连接终止、删库、fixture embed 与 `LoadDevelopmentFixtures`。 | 清除原有自动化测试数据库生命周期管理和运行时数据导入。 |
| `src/internal/infra/postgres/database_test.go` | 删除临时数据库名生成测试。 | 被测能力随临时数据库生命周期一起删除。 |
| `src/internal/infra/postgres/store.go` | 保持 `Open`、`Close` 和 Repository/UnitOfWork 实现，删除 `Migrate` 和 SQL embed。 | `Store` 只负责业务持久化连接，不能修改 schema、迁移账本或测试数据。 |
| `src/internal/infra/postgres/store_integration_test.go` | 改用固定 dev URL，并通过 Python 脚本在测试前后 reset、为 fixture 用例载入外部数据；删除临时库创建、清理验证与固定库所有权测试。 | 在不创建/删除数据库的前提下保持用例独立、可重复且验证 demo SQL。 |
| `src/internal/infra/postgres/fixtures/*.sql`、`migrations/*.sql` | 删除原位置文件。 | schema 和数据从 Go 基础设施目录迁到仓库根 SQL 资源。 |
| `sql/migrations/*.sql` | 新建为原内嵌迁移的迁移目标。 | 由 Python 脚本维护 `schema_migrations` 并按文件名顺序应用。 |
| `sql/development/001_roster_demo.sql`、`002_game_demo.sql`、`003_training_demo.sql` | 新建为原 development fixture 的迁移目标。 | 供 Python 初始化、重置和 fixture 集成测试执行。 |
| `sql/test/reset.sql` | 新建固定测试库的业务表清空脚本。 | 为共享固定库提供无删库的可重复测试与手动恢复基础。 |
| `scripts/manage_dev_db.py`、`scripts/requirements.txt` | 新建 Python 数据库管理脚本及固定依赖。 | 使用 `tomli` 读取配置、使用 `psycopg` 直接管理固定开发测试库。 |
| `justfile` | 引入新增 `just/db.just`。 | 把显式数据库维护命令纳入项目统一入口。 |
| `just/db.just` | 新建 `create`、`init`、`reset`、`check`、`test-reset` 等配方。 | 开发者手动管理固定库，不通过 app 或测试自动创建/删除。 |
| `just/test.just` | 让 `all` 先调用 `just db check` 再运行 Go 测试，并改写说明与失败引导，移除“fresh development database”假设。 | 反映固定库前置条件。 |
| `just/app.just` | 把 `dev` 注释改为“固定开发测试库”，不再称 fresh database。 | 避免使用者误解每次运行会得到新库。 |
| `doc/overview/infrastructure.md` | 实现后更新 PostgreSQL 存储说明，删除临时 `ManagedStore` 描述，说明固定库、外部 SQL 和显式 just 初始化。 | 长期现状文档需与新基础设施边界一致。 |
| `doc/overview/bootstrap-and-config.md` | 实现后更新 database 配置和组合根行为。 | 说明 run/dev 都是固定 URL，bootstrap 不拥有数据库。 |

## 错误、权限与兼容性

- 对外 CLI 的 `--profile run|dev` 协议不变；改变的是 `dev` 的数据库生命周期，从“每次创建并清理”改为“连接固定库”。
- TOML 是不兼容配置变更：存在 `mode`、`admin_url` 或 `temporary_prefix` 的旧本地配置会因严格解码失败，开发者必须按新的 example 改为 `url`。
- `just db create` 的数据库创建要求目标 PostgreSQL 用户具有连接管理库和 `CREATE DATABASE` 权限；应用和测试日常运行仅需要目标库的业务表读写权限。
- 初始化脚本只在开发者显式调用时创建库；已有库不隐式删除或覆盖。需要清空演示数据时只能显式执行 `just db reset`。
- 领域数据在正常业务路径仍仅软删除；`sql/test/reset.sql` 的物理清空只作用于固定测试基础设施库，不能由 application、domain 或业务 Repository 调用。

## 验证方案

实现完成后执行：

1. 安装 `scripts/requirements.txt` 后，使用新的 `config/config.toml` 执行 `just db init`，确认固定库只创建一次、迁移账本完整且 development SQL 可重复按 reset 流程载入。
2. 连续两次执行 `just test all`，确认 PostgreSQL 集成测试均通过，并检查 PostgreSQL 中没有随机 `basetion_test_*` 数据库产生。
3. 执行 `just app dev` 及现有 `just test player`、`just test game`、`just test training`，确认它们读取固定开发库中的示例数据；进程退出后库和数据仍保留。
4. 执行 `just db reset` 后再次检查 demo 数据的球队、比赛和训练记录数量，确认仅业务数据被恢复，数据库本身未删除。
5. 执行 `python3 -m py_compile scripts/manage_dev_db.py`、`go test ./...`、`go vet ./...` 并检查已修改 Go 文件的静态诊断。

提交本改造时，再按仓库规范同步更新上述 overview 文档，并新建一份 `doc/changes/` 记录；本设计草案本身不构成提交记录。
