# CLI 自动创建 Session

时间：2026-08-15 11:14（Asia/Shanghai）

## 背景

CLI 原先强制要求 `--session-id`，即使用户只想开始一次新对话也必须手工指定 ID。默认 demo 应能直接创建独立 Session，同时保留显式 ID 的续聊能力。

## 变更

- 将 `--session-id` 改为可选参数；省略时使用密码安全随机数生成带 `session-` 前缀的 ID，并在输出中告知用户。
- 保留显式 Session ID 的历史对话续接行为，并增加无 ID 新建与持久化测试。
- 调整默认 Just 配方，使每次 demo 运行自动创建新 Session。
- 更新入口现状文档，并修正入口退出码与 TeamOps 工具层校验职责的旧描述。

## 影响模块

- 代码：`src/internal/entry/cli/`
- 工作流：`justfile`
- 文档：`doc/overview/entry.md`、`doc/overview/tools.md`、`doc/changes/2026-08-15-1114-optional-session-id.md`

## 验证

- `just --dry-run run`：通过，展开为不带 `--session-id` 的新 Session demo 命令。
- `GOCACHE=/tmp/basetion-session-cli-go-cache GOTOOLCHAIN=auto go test ./...`：通过，所有 Go 包测试成功。
- `GOCACHE=/tmp/basetion-session-cli-go-cache GOTOOLCHAIN=auto go vet ./...`：通过，未发现静态检查问题。
- `git diff --check`：通过，未发现空白错误。
