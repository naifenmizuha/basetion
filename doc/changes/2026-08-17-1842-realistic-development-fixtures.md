# 背景

PostgreSQL 开发 fixture 原本用三国人物填充了两队名单，但守备能力、球衣号码和训练安排较为整齐；比赛在首局后主要由批量生成的三振打席组成。查询比赛、名单和自训记录时，示例数据无法呈现业余棒球队常见的人员结构和赛况变化。

# 变更

- 调整蜀汉队球员的左右打投、主守备和兼项守备标志，并重新分配球衣号码；保留蜀汉、曹魏队名、三国人物、成员关系和既有 UUID。
- 将比赛 fixture 扩展为 65 条打席，覆盖安打、保送、滚地出局、三振、外野飞球和野手选择等结果。比赛保持已结束状态，末条记录为九局下半刘备的再见一垒安打，累计比分为蜀汉 5:4 曹魏。
- 将自训 fixture 改为 18 名球员在近 14 天内的 46 条不等频记录。训练内容与感想按投手、捕手、内野、外野和跑垒需求编写，名单中也保留无训练记录的球员。
- 调整 fixture 集成测试，验证两队名单数量、代表性训练记录、无记录球员、46 条训练总数、65 条比赛打席和最终比分。
- 更新 roster、game、training 与 infrastructure overview，记录当前开发数据集和测试覆盖范围。

# 影响模块

## 基础设施层

- `src/internal/infra/postgres/fixtures/001_roster_demo.sql`：蜀汉队球员的 `batting_flags`、`throwing_flags`、`position_flags` 与 `memberships.jersey_number` 从单一守备/顺序编号调整为能表达投捕、内外野和兼项的配置。数据继续由 `Store.LoadDevelopmentFixtures` 按文件名载入，供名单 Repository 与 `RosterReader` 查询。
- `src/internal/infra/postgres/fixtures/002_game_demo.sql`：保留 `matches`、`lineups` 和已有球员引用，后续打席从每半局固定三振改为 65 条不同结果的事件序列；最后一个 `plates` 记录的 `home_score`、`away_score` 为 5、4，供 `GameDetail` 的当前比分读取。
- `src/internal/infra/postgres/fixtures/003_training_demo.sql`：以 `records` CTE 明确列出球员、相对日期、训练内容和感想，插入 46 条 `training_records`。日期仍基于 `current_date`，因此固定库每次加载后都显示近期记录；同一球员同日只生成一条记录，符合部分唯一索引。

## 测试

- `src/internal/infra/postgres/store_integration_test.go`：`TestPostgresDevelopmentFixtures` 不再依赖旧球衣排序或全员五天训练，改为检查刘备、曹操的训练记录、法正的空结果、训练总数和比赛末条比分。测试仍经 `OpenTemporary` 创建临时库并加载全部 fixture。

## 文档

- `doc/overview/infrastructure.md`：将开发 fixture 说明更新为两队各 20 人、5:4 比赛、18 名球员 46 条训练记录及其集成测试覆盖。
- `doc/overview/roster-domain.md`、`doc/overview/game-domain.md`、`doc/overview/training-domain.md`：补充各领域在开发 fixture 中的当前数据行为和查询用途。

# 验证

- `GOTOOLCHAIN=auto go test ./src/internal/infra/postgres -run TestPostgresDevelopmentFixtures -count=1`：通过。
- `GOTOOLCHAIN=auto go test ./...`：通过。
