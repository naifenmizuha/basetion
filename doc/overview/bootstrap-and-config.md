# Bootstrap 与配置模块

## 覆盖路径

- `src/internal/bootstrap`
- `src/internal/config`

## 配置

配置使用 Viper 从进程当前工作目录下的 `config/config.toml` 读取，并使用 gotenv 可选解析进程当前工作目录下的 `.env`。`.env` 不存在时继续启动；文件无法读取或格式错误、TOML 不存在或无法解析、字段校验失败时启动终止。配置分为三个职责组：

- `openai.model`、`openai.api_key_env` 必填，`openai.base_url` 可选。`api_key_env` 保存提供真实 API Key 的环境变量名称；该变量不存在或值为空时配置无效，真实密钥不从 TOML 解码。
- `session.dir` 默认 `.basetion/sessions`。
- `agent.max_iterations` 默认 20，必须为正整数；`agent.unsafe_debug_data` 默认关闭。

进程环境变量优先于 `.env`，两者都优先于 TOML。`OPENAI_MODEL`、`OPENAI_API_KEY_ENV`、`OPENAI_BASE_URL`、`BASETION_SESSION_DIR`、`BASETION_MAX_ITERATIONS` 和 `BASETION_UNSAFE_DEBUG_DATA` 覆盖相应配置键；解析 `api_key_env` 后，再按同样的进程环境优先规则读取它指定的 API Key。`config/config.example.toml` 与 `.env.example` 提供无密钥模板；实际 `config/config.toml` 和 `.env` 被 Git 忽略。

`Init` 只允许一次初始化尝试，成功后发布进程级只读配置快照；`InitFile` 允许测试从隔离路径初始化。`Get` 返回配置值副本，初始化成功前调用会 panic。配置不支持运行时修改或热加载。`Validate` 汇总多个配置问题；`DiagnosticFields` 不包含 API Key。

## 组合根

`bootstrap.Execute` 是具体实现的装配位置，依次完成：全局配置和中文语言初始化、文件 Session 存储、TeamOps Lua 执行器、TeamOps 领域服务与工具、AgenticModel、Harness Runtime、会话服务和 CLI 调用。存储装配读取初始化后的 Session 配置；Harness 内部通过全局配置快照读取 OpenAI 和 Agent 参数，并从当前工作目录的 `skills/` 初始化必需 Skill，构造函数不接收 `Config` 参数。

任一步初始化失败都会向 stderr 输出带阶段语义的错误并返回非零退出码。启动日志使用脱敏诊断字段；具体依赖关系不下沉到入口或领域模块。
