# 自训记录领域模块

## 覆盖路径

- `src/internal/domain/training`

## 实体和值对象

`Record` 表达球员某个自然日的一条自训记录，保存球员 ID、训练日期、训练内容、训练感想以及创建、更新、软删除时间。训练内容去除首尾空白后必须非空，训练感想允许为空；球员和训练日期创建后不可修改。同一球员每天最多存在一条未删除记录，删除后允许重新创建。

`Date` 是模块内不含时区和时刻的日历日期值对象，使用 `YYYY-MM-DD` 解析和输出。实体恢复时校验 ID、球员、日期、文本和时间戳，非法持久化数据映射为损坏数据错误。

## 领域服务与端口

写 `Service` 通过训练 Repository、球员 Repository 和 Clock 创建、更新及软删除记录。创建改为按姓名解析：`Create(ctx, id, playerName, teamName, date, content, reflection)` 先经球员 Repository 的 `ListByNameWithTeam` 拿到候选（含球队名），同名多人时返回带球队与背号的 `ErrAmbiguousPlayerName`，由调用方补 `team_name` 重试；查无此人返回 `ErrPlayerNotFound`；唯一命中后沿用原有校验写入。创建时要求球员存在但不要求启用；更新直接保存新的内容和感想，删除使用 `deleted_at`。

只读 `QueryService` 同时支持按球员 ID 或姓名读取：`Filter` 拥有 `PlayerID`（供 Lua 代理继续使用）与 `PlayerName`（按名，同名全返回），可叠加起止日期与 `limit`。`ListViews` 统一走"两次查询"：先按名或 ID 解析候选球员集，再按球员 ID 集经训练 Repository 的 `ListByPlayers` 拉记录，最后批量 `GetByIDs` 拼装球员与球队名形成 `RecordView`，遇到软删除或失效球员直接跳过。团队与球员的批量读取端口分别为 `TeamReader.GetByIDs` 与 `PlayerReader.GetByIDs`，由 `WithTeamReader`、`WithPlayerReader` 选项注入。

领域层只定义实体、规则和 Repository/Clock 端口。训练能力直接由 `team_modify` 适配写服务，并通过 `team_query` 的只读 Lua 代理查询，不新增 application 子包。

开发 fixture 只为两队中的 18 名球员提供训练记录，共 46 条，日期相对数据库 `current_date` 分布在近 14 天。内容和感想按投手、捕手、内野、外野及跑垒训练区分，未记录训练的球员也保留在名单中，用于展示空查询结果。
