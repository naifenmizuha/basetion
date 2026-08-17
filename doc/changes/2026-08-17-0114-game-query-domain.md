# 比赛查询领域化与工具接入

## 背景

比赛、阵容和打席已有持久化雏形，但模型工具尚未接入；既有名单 Lua 查询直接扫描为 Team Query View，没有经过领域模型。比赛也缺少明确状态和可直接读取的比分快照。

## 变更

- 增加比赛、阵容和打席领域模型、事务服务、只读服务、软删除 Repository 与开发 fixture；Match 增加状态，Plate 增加可空历史比分和新写入必填的比分快照。
- 普通名单与比赛查询统一先恢复领域对象，再由 Lua 适配器暴露 `roster`、`game` 和 `lineup`；训练和分析仍不可用。
- `team_modify` 增加比赛、阵容和打席操作，保持确认、批量顺序执行和失败不回滚语义。
- 增加领域数据软删除协作规范，并同步现状文档与测试命令。

## 影响模块

- `AGENTS.md`、`just/test.just`
- `src/internal/domain/{team,player,roster,game,teamquery}`
- `src/internal/infra/{postgres,teamquery}`
- `src/internal/{bootstrap,tools}`
- `doc/overview/`、`doc/changes/2026-08-17-0114-game-query-domain.md`

## 验证

- `just test all`：通过，包含 PostgreSQL 迁移、fixture、Repository 集成测试以及全部领域、Lua、工具和 Harness 测试。
- `go vet ./...`：通过。
- `git diff --check`：通过。
