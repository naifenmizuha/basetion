# 球员与球队领域模块

## 覆盖路径

- `src/internal/domain/player`
- `src/internal/domain/team`

## 实体和值对象

`Player` 是球队所属球员：创建时即固定 `team_id`、球衣号码、姓名、左右手标志和守备位置；已创建球员不提供转队入口。球衣号码为 0–99，同一未删除球队内唯一。`Team` 保存名称和启用状态。两个实体恢复持久化数据时均验证 ID、时间戳和软删除时间；非法数据返回各自的损坏数据错误。

球员的资料、启停和当前背号可以更新。背号变更会在事务内检查同队占用；数据库的部分唯一索引冲突也映射为 `player.ErrJerseyOccupied`。球员和球队都使用 `deleted_at` 软删除，不再维护乐观锁版本。

## 领域服务与端口

`player.Service` 创建球员时在事务内确认所属球队存在且启用，并检查背号。删除球队时，它在同一事务中软删除该队全部球员，再软删除球队；删除球员时先软删除其训练记录，再软删除球员。`team.Service` 负责球队自身的创建与启停。

名单读能力位于 `player.QueryService`：`PlayerFilter` 可按球队精确名称、任一守备位置和背号筛选，返回包含球队名、背号、守备位置和启用状态的 `PlayerView`，不暴露内部 ID。原 `domain/roster` 的 `Membership`、日期效力期和名单服务已经移除。

领域层只定义实体、规则、Repository/UnitOfWork/Clock 端口，不依赖 PostgreSQL、CLI、Eino 或 application 包。`team_modify` 适配球员创建、资料/启用状态更新和改号；`team_query` 使用只读查询投影。

开发 fixture 包含蜀汉、曹魏两支虚构业余球队及各自当前球员，球员的左右打投、守备位置和背号用于覆盖名单查询与比赛事实。
