# Bootstrap 与配置模块

## 覆盖路径

- `internal/bootstrap`
- `internal/config`

## 配置

配置使用 Viper 从进程当前工作目录下的 `config/config.toml` 读取，文件不存在、TOML 无法解析或字段校验失败时启动终止。配置分为三个职责组：

- `openai.model`、`openai.api_key` 必填，`openai.base_url` 可选。
- `session.dir` 默认 `.basetion/sessions`。
- `agent.max_iterations` 默认 20，必须为正整数；`agent.unsafe_debug_data` 默认关闭。

`OPENAI_MODEL`、`OPENAI_API_KEY`、`OPENAI_BASE_URL`、`BASETION_SESSION_DIR`、`BASETION_MAX_ITERATIONS` 和 `BASETION_UNSAFE_DEBUG_DATA` 显式绑定到相应键，并优先于文件值。`config/config.example.toml` 提供无密钥模板；实际 `config/config.toml` 被 Git 忽略。

`Init` 只允许一次初始化尝试，成功后发布进程级只读配置快照；`InitFile` 允许测试从隔离路径初始化。`Get` 返回配置值副本，初始化成功前调用会 panic。配置不支持运行时修改或热加载。`Validate` 汇总多个配置问题；`DiagnosticFields` 不包含 API Key。

## 组合根

`bootstrap.Execute` 是具体实现的装配位置，依次完成：全局配置和中文语言初始化、文件 Session 存储、内存知识检索器、知识领域服务、知识工具、AgenticModel、Harness Runtime、会话服务和 CLI 调用。存储装配读取初始化后的 Session 配置；Harness 内部通过全局配置快照读取 OpenAI 和 Agent 参数，构造函数不再接收 `Config` 参数。

任一步初始化失败都会向 stderr 输出带阶段语义的错误并返回非零退出码。启动日志使用脱敏诊断字段；具体依赖关系不下沉到入口或领域模块。
