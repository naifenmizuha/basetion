# Team Modify 批量执行

## 背景

多个彼此独立的球队数据修改原先只能逐项调用工具，重复增加模型与工具之间的交互次数，也缺少统一的执行顺序和部分失败报告。

## 变更

- 为 `team_modify execute` 增加最多 50 项的 `operations` 批量输入，每项要求唯一非空 key。
- 在任何写入前校验完整批次，并按输入顺序执行；运行时失败后停止执行剩余步骤，不回滚已经成功的步骤。
- 返回批次整体状态、逐项结果和停止位置，明确区分成功、失败与跳过步骤，同时保留原有单项调用协议。
- 更新 `manage-team` Skill，要求合并可独立确定参数的多个修改并正确解释非原子失败结果。
- 更新 Tools 与 Harness 现状文档以反映批量协议和执行语义。

## 影响模块

- `src/internal/tools/team_modify.go`
- `src/internal/tools/team_modify_test.go`
- `skills/manage-team/SKILL.md`
- `doc/overview/tools.md`
- `doc/overview/harness.md`
- `doc/changes/2026-08-16-2249-team-modify-batch.md`

## 验证

- `GOTOOLCHAIN=auto go test ./src/internal/tools ./src/internal/architecture`：`src/internal/tools` 通过；仓库不存在 `src/internal/architecture` 目录，因此命令整体失败。
- `GOTOOLCHAIN=auto go test ./...`：除 PostgreSQL 集成测试外其余包通过；在沙箱外重试后，五项 PostgreSQL 集成测试仍因本地 `localhost:5432` 无可连接服务而失败。
