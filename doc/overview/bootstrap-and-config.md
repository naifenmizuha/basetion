# Bootstrap 与配置模块

## 覆盖路径

- `src/internal/bootstrap`
- `src/internal/config`

## 配置

配置使用 Viper 从当前工作目录的 `config/config.toml` 读取，并可选解析 `.env`。`openai` 配置保存模型、请求选项、API Key 的环境变量名和 `context_window_tokens`；`user.name` 是运行时状态栏显示的固定用户；`log.file` 指定追加写入的运行日志文件。`database.run` 与 `database.dev` 只保存各自固定连接 URL；Session 和 Agent 配置也由 TOML 管理。`context_window_tokens` 必须为正数，`user.name` 和 `log.file` 不能为空；省略时分别使用 128000、`Basetion 用户` 和 `.basetion/basetion.log`。真实 API Key 不写入 TOML，按进程环境优先于 `.env` 读取。配置不支持环境变量覆盖数据库连接、热加载或运行时修改。

## 组合根

`bootstrap.Execute` 提取 `--profile run|dev`，初始化配置后按 `log.file` 创建父目录并以追加方式打开 0640 日志文件；该步骤失败按配置错误退出。启动配置、Harness 生命周期和对话轮次汇总写入此文件，CLI 的用户可见错误仍写入 stderr。随后它初始化 Eino 中文语言，连接选定 profile 的 PostgreSQL，并组装文件 Session 存储、球队/球员/比赛/训练领域服务、Lua Team Query Executor、Team Fetch/Query/Modify 工具、模型、Harness 和会话服务。比赛写服务同时接收 Store 提供的直接比赛、球队和球员 Repository 及 UUID 生成器，供 `game.create` 按名称和背号解析整场记录；Player 查询服务从 PostgreSQL 的姓名化 Reader 构造；Game 查询服务直接使用 Store 的组合投影和按内部比赛引用批量读取 Play 的基础投影。

组合根不创建数据库、执行迁移、重置数据或生成 sqlc 代码；这些动作由显式 Just 配方完成。普通入口装配 `FileStore`，检测到 CLI `test` 子命令时改装配 `MemoryStore`，其余领域服务、工具、模型和数据库 profile 保持一致。任一初始化失败都会按阶段输出错误并返回非零退出码，连接池在进程结束时关闭。
