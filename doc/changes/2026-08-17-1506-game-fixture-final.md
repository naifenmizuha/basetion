# 背景

开发比赛 fixture 需要体现一场已结束的完整比赛，以支持比赛结果查询演示和集成测试。

# 变更

- 将示例球队名称调整为“蜀汉队”。
- 将示例比赛标记为已结束，并补齐至第九局下半的打席及最终比分。
- 更新 PostgreSQL fixture 集成测试的球队名称、比赛状态、打席数量和末个打席断言。

# 影响模块

- `src/internal/infra/postgres/fixtures/001_roster_demo.sql`
- `src/internal/infra/postgres/fixtures/002_game_demo.sql`
- `src/internal/infra/postgres/store_integration_test.go`
- `doc/changes/2026-08-17-1506-game-fixture-final.md`

# 验证

- `just test`：通过。
