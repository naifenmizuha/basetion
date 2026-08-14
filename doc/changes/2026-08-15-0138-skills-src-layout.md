# Basetion Skills 与源码目录重构

## 背景

Basetion 的固定项目知识原由开发期内存检索器提供，TeamOps 操作规则则直接写在全局系统指令中。随着 Agent 能力扩展，这种方式会持续增大全局提示词，也不利于按任务渐进加载领域说明。同时，仓库需要将 Go 源码统一收纳到 `src/`。

## 变更

- 将 `cmd/` 和 `internal/` 下的 Go 源码迁移到 `src/`，同步更新模块 import、CLI 命令、测试与 README。
- 在 Harness 中接入 Eino Skill Middleware，从当前工作目录的 `skills/` 加载并校验必需 Skill；缺失目录、非法 frontmatter、空元数据、重复名称或必需 Skill 缺失时拒绝启动。
- 将全局系统提示词拆为嵌入式 Markdown，只保留身份、真实性、Skill 使用和输出边界。
- 新增 `project-knowledge` 与 `manage-teamops` 两个自包含 Skill，分别承载项目现状和 TeamOps 只读操作流程。
- 移除开发期 Knowledge 领域、内存 Retriever 和 `search_knowledge` 工具；保留 `teamops` 作为球队结构化事实的权威执行工具。
- 更新模块现状文档，删除已失效的 Knowledge 领域文档和索引。

## 影响模块

- `src/cmd/basetion`
- `src/internal/`
- `skills/`
- `README.md`
- `justfile`
- `go.mod`
- `go.sum`
- `doc/overview/`
- `doc/changes/2026-08-15-0138-skills-src-layout.md`

## 验证

- `GOCACHE=/tmp/basetion-go-cache GOTOOLCHAIN=auto go test ./...`：通过，所有源码包测试成功。
- `GOCACHE=/tmp/basetion-go-cache GOTOOLCHAIN=auto go vet ./...`：通过，无静态检查错误。
- `git diff --check`：通过，无空白错误。
- 搜索旧模块 import、`search_knowledge` 和 `go run ./cmd/basetion`：在 README、justfile、源码和 Skills 中均无残留。
