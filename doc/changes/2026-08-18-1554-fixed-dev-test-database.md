# 固定开发测试库与脚本化数据库管理

## 背景

开发 profile 原先使用临时 PostgreSQL 数据库。每次通过 `--profile dev` 启动应用或运行 PostgreSQL 集成测试时，Go 基础设施都会创建随机数据库、执行内嵌迁移、选择性载入 fixture，并在结束时断开遗留连接后删除数据库。这让测试和调试产生大量建库、迁移、删库操作，开发数据也无法保留。

本次把开发与集成测试统一切换到 `database.dev.url` 指向的固定数据库。数据库创建、迁移、清空和示例数据载入脱离 Basetion Go 程序，由仓库内 Python 脚本显式执行。

## 变更

- `database.run` 与 `database.dev` 现在都只声明 `url`。删除 `mode`、`admin_url`、`temporary_prefix` 以及对应的 Go 配置类型、校验和诊断字段。旧配置含这些字段时会因严格 TOML 解码失败，需按新的 `config/config.example.toml` 改为固定 URL。
- `src/internal/bootstrap/app.go` 依据 `--profile` 选择连接 URL 后直接调用 `postgres.Open`；`Store` 只负责连接池、Repository 和 UnitOfWork，进程结束时只关闭连接池。应用不再创建数据库、执行迁移、加载 fixture、终止连接或删除数据库。
- 删除 `src/internal/infra/postgres/database.go`、`database_test.go`，移除 `ManagedStore`、`OpenTemporary`、随机库名、临时库清理和运行时 fixture 加载能力。`src/internal/infra/postgres/store.go` 同时删除嵌入式迁移与 `Migrate`，因此业务持久化层不再持有 schema 变更职责。
- PostgreSQL 迁移从 `src/internal/infra/postgres/migrations/` 迁到 `sql/migrations/`；开发示例数据从 `src/internal/infra/postgres/fixtures/` 迁到 `sql/development/`；`sql/test/reset.sql` 通过 `TRUNCATE ... RESTART IDENTITY` 清空固定测试库的业务表，不删除数据库或迁移账本。
- 新增 `scripts/manage_dev_db.py` 和 `scripts/requirements.txt`。脚本用 `tomli` 解析 `config/config.toml` 的 `database.dev.url`，用 `psycopg` 参数化查询 PostgreSQL：`create` 创建不存在的开发库，`migrate` 维护 `schema_migrations` 并按文件名应用迁移，`init` 负责建库、迁移、清空和载入 development SQL，`reset` 恢复示例数据，`check` 校验库与迁移状态，`test-reset` 仅清空测试数据。脚本不调用 Go 配置或持久化代码。
- `just/db.just` 将数据库维护入口改为 Python 脚本的 `create`、`init`、`reset`、`check`、`test-reset`；`just/test.just` 的 `all` 在运行 Go 测试前调用 `just db check`。Docker Compose 的启停职责仍在 `just dep up|down`，数据库脚本不隐式管理容器。
- `src/internal/infra/postgres/store_integration_test.go` 保持固定数据库测试不并行：每个用例前后调用 Python 脚本的 `test-reset`，fixture 用例调用 `test-load-development`。测试代码不再直接执行清空或 fixture SQL。
- 更新 `doc/designs/test-database-provisioning.md`，使设计与最终 Python/TOML/psycopg 实现一致；更新 infrastructure 和 bootstrap/config overview，明确 Go 运行时与数据库管理工具的边界。

## 影响模块

### 配置与入口层

- `config/config.example.toml`、`src/internal/config/config.go`、`src/internal/config/config_test.go`：数据库 profile 从可选临时生命周期收敛为固定连接 URL。`bootstrap.Execute` 仍保留 `run|dev` profile 协议，但 profile 仅选择连接目标。
- `src/internal/bootstrap/app.go`：删除临时库模式分支和清理超时逻辑。它依赖已准备好的 schema；若未执行 `just db init` 或新增迁移未应用，连接后的业务调用会失败，不会自动修改数据库。

### 基础设施与数据库

- `src/internal/infra/postgres/store.go`：保留业务读写、事务和连接池实现；不再嵌入或执行 SQL migration，依赖方向从 Go 基础设施指向数据库变为外部工具先准备数据库、基础设施后连接。
- `sql/migrations/`：承接原有五份 schema migration，按文件名作为迁移账本版本。
- `sql/development/`：承接球队、比赛和训练 demo 数据；`sql/test/reset.sql` 只用于固定测试基础设施库的物理清空，不改变领域业务数据必须软删除的规则。

### 工具与测试

- `scripts/manage_dev_db.py`、`scripts/requirements.txt`：新增独立 Python 工具链。`tomli` 解析 TOML，`psycopg` 建库、迁移、执行 SQL 和检查迁移状态；该工具只针对 `database.dev`，不修改 `database.run`。
- `justfile`、`just/db.just`、`just/test.just`、`just/app.just`：将 Python 数据库维护脚本纳入模块化 Just 命令；应用和测试命令不再创建或停止依赖容器。
- `src/internal/infra/postgres/store_integration_test.go`：测试保留业务断言，测试数据准备改经 Python 脚本完成，避免 Go 测试本身承担数据库管理逻辑。

### 文档与仓库协作

- `doc/designs/test-database-provisioning.md`：记录固定库与 Python 管理方案的最终落点。
- `doc/overview/infrastructure.md`、`doc/overview/bootstrap-and-config.md`：描述当前实际运行边界。
- 本提交新增设计文档、同步 overview，并提供单独的 changes 记录，便于后续维护数据库工具与运行时边界。

## 验证

- `python3 -m pip install --user -r scripts/requirements.txt`：成功安装 `psycopg[binary]==3.2.10`、`tomli==2.2.1` 及其依赖。
- `python3 -m py_compile scripts/manage_dev_db.py`：通过。
- `just db init`：通过，使用 Python 脚本完成固定开发库的建库检查、迁移、清空和 development SQL 载入。
- `just test all`：通过，包含固定 PostgreSQL 集成测试。
- `GOTOOLCHAIN=auto go vet ./...`：通过。
- `git diff --check`：通过。
