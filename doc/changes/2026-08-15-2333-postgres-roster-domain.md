# PostgreSQL 名单领域与 TeamOps 查询

## 背景

项目需要在保持领域层与应用层边界的前提下，为球队、球员和效力名单建立真实持久化能力，并让 Agent 能通过受限 TeamOps Lua 接口读取开发名单数据。运行与开发数据库还需要明确、可复现且不依赖临时环境变量的生命周期配置。

## 变更

- 新增球队、球员和名单实体、值对象、领域服务、Repository 与 UnitOfWork 端口，覆盖启用状态、球衣占用、效力期、事务锁和乐观并发规则。
- 新增 PostgreSQL 存储、内嵌迁移、开发 fixture、固定/临时数据库生命周期及集成测试，并提供只读名单投影和数据一致性审计。
- 为 TeamOps Lua 执行器接入可发现的 `roster` 模块，支持列出球队和按球队、日期、守备位置读取球员名单；其余预声明模块保持不可用。
- 新增 `database.run`/`database.dev` 配置与 `--profile` 参数，移除非密钥运行配置的环境覆盖；组合根负责数据库装配和临时库清理。
- 新增只部署 PostgreSQL 依赖的 Compose 配置，并调整 justfile、Skill 与 README 中的运行、开发、测试说明。
- 更新架构约束及 overview 文档，记录领域落位、配置边界、PostgreSQL 基础设施和当前 TeamOps 能力。

## 影响模块

- `src/internal/domain/player`
- `src/internal/domain/team`
- `src/internal/domain/roster`
- `src/internal/domain/teamops`
- `src/internal/infra/postgres`
- `src/internal/infra/teamops`
- `src/internal/config`
- `src/internal/bootstrap`
- `src/internal/entry/cli`
- `src/cmd/basetion`
- `config/config.example.toml`
- `compose.yml`
- `justfile`
- `skills/`
- `README.md`
- `AGENTS.md`
- `doc/overview/`

## 验证

- `gofmt -w ...`：通过，已格式化本次涉及的 Go 文件。
- `docker compose config --quiet`：通过。
- `GOTOOLCHAIN=auto go vet ./...`：通过。
- `git diff --check`：通过。
- `GOTOOLCHAIN=auto go test ./...`：未全部通过；除 `src/internal/infra/postgres` 外的包均通过，PostgreSQL 包的 5 个集成测试因本机 `localhost:5432` 没有可连接的 PostgreSQL 服务而超时失败。
