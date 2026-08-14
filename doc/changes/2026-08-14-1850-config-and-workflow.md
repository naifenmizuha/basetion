# 完善本地运行配置与提交流程

时间：2026-08-14 18:50（Asia/Shanghai）

## 背景

项目需要更便捷的本地启动入口和更安全的 API Key 配置方式，同时仓库文档规范应仅在用户明确要求提交时执行，避免按修改任务拆分记录或提交。

## 变更

- 新增默认 Just 启动配方，用固定 Session 和提示词运行 CLI demo。
- 可选读取启动工作目录下的 `.env`，保持进程环境优先于 `.env`、二者优先于 TOML 的覆盖顺序。
- 用 `openai.api_key_env` 替换 `openai.api_key`，运行时按配置的变量名解析真实密钥；旧字段由严格解码拒绝。
- 增加 `.env`、密钥解析、优先级、错误边界和 Harness 装配测试，并更新配置与环境变量模板。
- 将文档维护时机统一为用户明确要求提交时；一次提交只生成一份 changes 记录，不再按修改任务拆分。
- 明确 Git 提交和 GitHub 发布命令必须直接在沙箱外运行，同时保留提交范围确认要求。
- 同步更新入口、配置 overview、运行说明和 changes 目录说明。

## 影响模块

- `justfile`
- `internal/config/`
- `internal/harness/runtime_test.go`
- `go.mod`
- `config/config.example.toml`
- `.env.example`
- `README.md`
- `AGENTS.md`
- `doc/overview/entry.md`
- `doc/overview/bootstrap-and-config.md`
- `doc/changes/README.md`
- `doc/changes/2026-08-14-1850-config-and-workflow.md`

## 验证

- `just --list`：通过，列出 `default` 和 `run` 配方。
- `just --dry-run` 与 `just --dry-run run`：通过，均展开为预期的 demo 启动命令。
- `gofmt -w internal/config/config.go internal/config/config_test.go internal/harness/runtime_test.go`：完成，修改后的 Go 文件已格式化。
- `go test ./...`：通过，所有现有 Go 包测试成功。
- `go vet ./...`：通过，未发现静态检查问题。
- `go mod tidy -diff`：通过，模块依赖文件无需进一步整理。
- `git diff --check`：通过，未发现空白错误。
