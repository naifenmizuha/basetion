# TeamOps 可编程只读查询

时间：2026-08-14 22:46（本地时间）

## 背景

棒球队管理能力未来会由 Agent 组合 TeamOps SDK 的读取操作。为避免一次性向模型暴露大量工具协议，本次先建立单一的渐进式发现与可编程查询入口，并保留真实数据模块的接入边界。

## 变更

- 新增 `teamops` 工具，以 `describe` 和 `query` 两种模式提供能力发现与只读 Lua 5.1 程序执行。
- 新增 TeamOps 查询领域契约和五个不可用的占位模块，不提供演示数据或写入能力。
- 新增受限 GopherLua 运行时，裁剪危险标准库并限制时间、源码、栈、返回深度、元素数和结果大小。
- 将工具装配到 Agent Harness，并补充只读使用指令及端到端工具结果测试。
- 将项目最低 Go 版本升级至 1.26.6，新增 GopherLua v1.1.2；`just run` 显式启用自动工具链选择以兼容系统默认 `GOTOOLCHAIN=local` 的环境。
- 同步 README 和受影响模块的现状文档。

## 影响模块

- `README.md`
- `go.mod`
- `go.sum`
- `justfile`
- `internal/domain/teamops`
- `internal/infra/teamops`
- `internal/tools`
- `internal/harness`
- `internal/bootstrap`
- `doc/overview`
- `doc/changes/2026-08-14-2246-teamops-lua-query.md`

## 验证

- `GOTOOLCHAIN=go1.26.6 GOCACHE=/tmp/basetion-go1266-release2-cache go test ./...`：通过。
- `GOTOOLCHAIN=go1.26.6 GOCACHE=/tmp/basetion-go1266-release2-vet-cache go vet ./...`：通过。
- `GOTOOLCHAIN=go1.26.6 go version`：输出 `go version go1.26.6 linux/amd64`。
- `GOTOOLCHAIN=auto go run ./cmd/basetion --help`：成功切换到 Go 1.26.6 并启动 CLI，帮助请求按设计返回用法和退出码 2。
- `just --dry-run run`：输出包含 `GOTOOLCHAIN=auto` 的运行命令。
- `git diff --check`：通过。
