# Bootstrap 与配置模块

## 覆盖路径

- `src/internal/bootstrap`
- `src/internal/config`

## 配置

配置使用 Viper 从当前工作目录的 `config/config.toml` 读取，并可选解析 `.env`。`openai` 配置保存模型、请求选项和 API Key 的环境变量名；`database.run` 与 `database.dev` 只保存各自固定连接 URL；Session 和 Agent 配置也由 TOML 管理。真实 API Key 不写入 TOML，按进程环境优先于 `.env` 读取。配置不支持环境变量覆盖数据库连接、热加载或运行时修改。

## 组合根

`bootstrap.Execute` 提取 `--profile run|dev`，初始化配置和 Eino 中文语言，连接选定 profile 的 PostgreSQL，然后组装文件 Session 存储、球队/球员/比赛/训练领域服务、Lua Team Query Executor、Team Query/Modify 工具、模型、Harness 和会话服务。Player 查询服务从 PostgreSQL 的姓名化 Reader 构造，Game 查询服务直接使用 Store 的五类只读投影。

组合根不创建数据库、执行迁移、重置数据或生成 sqlc 代码；这些动作由显式 Just 配方完成。任一初始化失败都会按阶段输出错误并返回非零退出码，连接池在进程结束时关闭。
