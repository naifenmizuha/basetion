# 背景

球队需要记录和查询球员每日自训内容与训练感想，并为开发环境提供可直接分析的近期训练样例。

# 变更

- 新增自训记录领域实体、日期值对象、读写服务和查询服务，约束每名球员每天最多一条有效记录。
- 新增 PostgreSQL 自训记录迁移与 Repository，实现软删除、乐观并发、日期范围查询和重复日期错误映射。
- 在球员删除事务中显式软删除关联自训记录。
- 启用 `team_query.training.records`，并为 `team_modify` 增加自训记录创建、更新和删除操作。
- 为两队全部球员生成最近五天共 200 条开发 fixture，并增加训练查询与修改演示命令。
- 更新自训领域、工具、基础设施、查询能力、组合根和入口现状文档。

# 影响模块

- `src/internal/domain/{training,roster,teamquery}`
- `src/internal/infra/{postgres,teamquery}`
- `src/internal/{tools,bootstrap}`
- `just/test.just`
- `doc/overview/`
- `doc/changes/2026-08-17-1542-player-training-records.md`

# 验证

- `go test ./...`：通过，包括 PostgreSQL 集成测试。
- `git diff --check`：通过。
