# 入口模块

## 覆盖路径

- `src/cmd/basetion`
- `src/internal/entry/cli`

## 进程入口

`src/cmd/basetion/main.go` 使用后台 Context 调用 `bootstrap.Execute`，将命令行参数和标准流交给组合根。仓库根目录 `justfile` 通过 `app`、`dep`、`db` 和 `sqlc` 模块组织运行资源；应用和长期依赖命令不会隐式调整数据库，`just db reset` 是明确重建固定开发库的命令，`just sqlc generate|check` 管理生成绑定。根 `just test <cases.toml>` 依次调用 `test-run` 生成测试 JSONL、`test-evaluate` 运行离线评测；`test-run` 会先显式执行 `just db check`，`test-evaluate` 不启动应用或连接数据库。

## CLI 协议

普通 CLI 要求至少一个非空提示词。`--profile run|dev` 选择 TOML 中的数据库配置，默认 run；`--session-id <ID>` 可选。未提供 Session ID 时生成 `session-<32 位十六进制>`，显式 ID 用于续接对应 Session。参数或配置错误退出码为 2，其余初始化和会话错误为 1，成功为 0。

`basetion test --input <cases.toml> [--output <results.jsonl>] [--max-concurrency N]` 是批量入口。输入采用 `[[test.<run-name>]]` 数组表；同名表按声明顺序构成一条连续 Session，不同名称的 Session 受并发上限控制。每张表必须有 `prompt`，可选 `expect_tools`、`expect_skills` 和 `forbid_tools` 声明确定性评测的路径要求。测试 Session 使用运行期内存存储，不写入 `session.dir`；结果每轮一条 JSONL，未指定输出路径时写入 `.basetion/test-results/`。批量轮次失败、缺失预期路径或调用禁用工具都会以非零状态结束相应命令。

## 输出渲染

`Render` 顺序消费 AgentEvent：reasoning 标记为 `[思考]`，助手文本按流式块累计并以 `[回复]` 输出，工具调用和结果显示工具名、call ID 与内容，动作显示为 `[动作]`，正常关闭输出 `[完成]`。入口层不负责 Session、模型或工具业务规则。

批量入口不复用终端渲染，而是保留结构化事件、模型请求 Trace、状态栏、用量和每轮预期字段供评测脚本读取。`scripts/evaluate_test_results.py <results.jsonl>` 只读取 JSONL，输出同名 `.evaluation.json` 与终端摘要；它统计轮次与 run 的耗时、并发墙钟时间、token、工具和 Skill，且对运行失败、缺失预期和禁用工具调用返回非零。评测报告不复制提示词、模型输入、工具结果或 reasoning 内容。
