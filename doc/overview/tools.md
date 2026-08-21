# Tools 模块

## 覆盖路径

- `src/internal/tools`

工具层把模型可见协议适配到领域服务，不实现领域规则或存储逻辑。

`team_describe` 是唯一的按需发现工具。它接受 `fetch.*`、`query.*`、`modify.*` 前缀的目录或精确 topic；省略 topic 时返回三个目录，可以在一次调用中混合多个来源。返回说明保留各来源的字段和可用性，并将名称及子项全部加上来源前缀，避免 `game` 等目录重名。

`team_fetch` 只执行已 describe 的组合读取，输入为 operation 与对应筛选参数；`operation` 的 JSON Schema 描述明确使用模块内叶子名（不带 `fetch.` 前缀），可读取 `game.summaries`、`game.records`、`game.lineups`、`game.performances` 与 `training.records`，未知名称的错误信息会列出可用 operation 便于模型自愈。`training.records` 按 `player_name`（精确匹配，同名全返回）与可选 `date_from`、`date_to`、`limit` 读取自训记录，返回项由领域层按两次查询拼接（先按名查球员、再按球员 ID 集查记录），不暴露内部 ID。它不接受 Lua、SQL 或写操作，统计口径继续由 `game.QueryService` 与 `training.QueryService` 实现。

`team_query` 只执行定义 `main(data)` 的受限 Lua 5.1 程序，不再接受 describe 或模块选择；可读取球队、当前球员、比赛基础对象和原子 Play。Lua 能把同一次 `data.game.list` 返回的原始比赛对象传给 `data.game.plays`，由运行时解析不透明引用并批量读取 Play；引用不会出现在工具结果。摘要、完整记录、阵容和表现不在 Lua 目录中。

`team_modify` 只执行已 describe 的预定义批量修改，输入必须有 `confirmed=true` 和非空 `operations`。目录包含 `team`、`player`、`game`、`match`、`lineup`、`training`：球员创建时必须指定球队和背号，可更新资料、启用状态或背号，但没有入队、离队或转队操作。`training.create` 接受 `player_name`（精确匹配必填）与可选 `team_name`，服务端先按姓名解析球员：唯一命中直接写入；同名多人返回带球队与背号的错误，调用方补充 `team_name` 后重试；查无此人返回 not found。`game.create` 使用主客队精确名称及阵容、Play 内的当前背号，一次写入一场已结束比赛；服务端推导球队归属、球数、垒包、出局数和比分。它先批量解析球队和球员、校验整场记录，再按比赛、两套阵容、Play 顺序写入；不使用数据库事务，写入阶段失败返回带完成进度的 `partial` 结果。它拒绝脚本和 SQL。

单项调用返回领域服务结果。批量调用最多 50 项，先校验全部参数后按数组顺序执行；它不是原子操作，某项运行时失败会停止后续项，返回已成功、失败和未执行项及 `stopped_at`。业务规则、事务和冲突处理保持在领域服务中。
