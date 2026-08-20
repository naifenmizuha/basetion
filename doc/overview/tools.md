# Tools 模块

## 覆盖路径

- `src/internal/tools`

工具层把模型可见协议适配到领域服务，不实现领域规则或存储逻辑。

`team_fetch` 提供 `describe` 和 `fetch`：读取比赛摘要、完整记录、阵容或逐场球员表现这四类服务端组合投影。它只接受声明的 operation 和比赛筛选参数，不接受 Lua、SQL 或写操作；统计口径继续由 `game.QueryService` 实现。

`team_query` 提供 `describe` 和 `query`。`query` 执行定义 `main(data)` 的受限 Lua 5.1 程序，不再要求或接受模块选择；可读取球队、当前球员、比赛基础对象和原子 Play。Lua 能把同一次 `data.game.list` 返回的原始比赛对象传给 `data.game.plays`，由运行时解析不透明引用并批量读取 Play；引用不会出现在工具结果。摘要、完整记录、阵容和表现不在 Lua 目录中。训练不在当前姓名化 Lua 协议内。

`team_modify` 提供 `describe` 和 `execute`。目录包含 `team`、`player`、`game`、`match`、`lineup`、`training`：球员创建时必须指定球队和背号，可更新资料、启用状态或背号，但没有入队、离队或转队操作。`game.create` 使用主客队精确名称及阵容、Play 内的当前背号，一次写入一场已结束比赛；服务端推导球队归属、球数、垒包、出局数和比分。它先批量解析球队和球员、校验整场记录，再按比赛、两套阵容、Play 顺序写入；不使用数据库事务，写入阶段失败返回带完成进度的 `partial` 结果。执行只接受已声明的结构化 operation，并要求用户确认后的 `confirmed=true`；拒绝脚本和 SQL。

单项调用返回领域服务结果。批量调用最多 50 项，先校验全部参数后按数组顺序执行；它不是原子操作，某项运行时失败会停止后续项，返回已成功、失败和未执行项及 `stopped_at`。业务规则、事务和冲突处理保持在领域服务中。
