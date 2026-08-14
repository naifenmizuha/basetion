# 使用 Viper 建立 TOML 全局配置

时间：2026-08-14 17:46（Asia/Shanghai）

## 背景

应用配置此前完全依赖进程环境变量，并由 Bootstrap 将扁平 `Config` 逐层传入模型和 Harness。需要改为从固定 TOML 文件集中管理配置，同时保留环境变量覆盖能力和密钥安全边界。

## 变更

- 引入 Viper，从当前工作目录的 `config/config.toml` 读取按 OpenAI、Session 和 Agent 分组的配置。
- 增加并发安全的一次性初始化与只读全局快照，环境变量显式绑定且优先于 TOML。
- Bootstrap、模型和 Harness 改为使用全局配置，移除 Harness 构造函数中的配置参数。
- 新增无密钥配置示例，忽略实际配置文件，并更新运行说明和模块现状文档。
- 补充配置文件解析、默认值、环境覆盖、错误输入、脱敏和单例行为测试。

## 影响模块

- 代码：`internal/config/`、`internal/bootstrap/`、`internal/harness/`
- 依赖：`go.mod`、`go.sum`
- 配置：`.gitignore`、`config/config.example.toml`
- 文档：`README.md`、`doc/overview/bootstrap-and-config.md`、`doc/overview/harness.md`、`doc/changes/2026-08-14-1746-viper-global-config.md`

## 验证

- `env GOCACHE=/tmp/basetion-go-cache go test ./...`：通过，全部 Go 包测试成功。
- `env GOCACHE=/tmp/basetion-go-cache go vet ./...`：通过，无静态检查问题。
- `git diff --check`：通过，未发现空白错误。
