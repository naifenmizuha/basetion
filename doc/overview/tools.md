# Tools 模块

## 覆盖路径

- `src/internal/tools`

工具层把模型可见协议适配到领域服务，不实现领域规则或存储逻辑。

`team_query` 提供 `describe` 和 `query`：前者返回精确 Lua topic 协议及可用性，后者执行定义 `main(team)` 的受限 Lua 5.1 程序。可查询球队、当前球员、比赛目录、摘要、完整记录、阵容和逐场表现；训练不在当前姓名化查询协议内。

`team_modify` 提供 `describe` 和 `execute`。目录包含 `team`、`player`、`match`、`lineup`、`training`：球员创建时必须指定球队和背号，可更新资料、启用状态或背号，但没有入队、离队或转队操作。执行只接受已声明的结构化 operation，并要求用户确认后的 `confirmed=true`；拒绝脚本和 SQL。

单项调用返回领域服务结果。批量调用最多 50 项，先校验全部参数后按数组顺序执行；它不是原子操作，某项运行时失败会停止后续项，返回已成功、失败和未执行项及 `stopped_at`。业务规则、事务和冲突处理保持在领域服务中。
