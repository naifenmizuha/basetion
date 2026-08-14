package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	domain "github.com/naifenmizuha/basetion/internal/domain/teamops"
)

const TeamOpsToolName = "teamops"

type teamOpsInput struct {
	Mode    string   `json:"mode" jsonschema:"required,description=操作模式：describe 按需发现只读能力，query 执行受限 Lua 5.1 查询,enum=describe,enum=query"`
	Modules []string `json:"modules,omitempty" jsonschema:"description=需要发现或使用的能力模块名称；省略时 describe 返回目录，query 仅运行纯 Lua"`
	Program string   `json:"program,omitempty" jsonschema:"description=query 模式必填的 Lua 5.1 程序，必须定义 main(teamops)；describe 模式禁止提供"`
}

type teamOpsOutput struct {
	Mode    string                     `json:"mode"`
	Runtime *domain.RuntimeDescription `json:"runtime,omitempty"`
	Modules []domain.ModuleDescription `json:"modules,omitempty"`
	Result  json.RawMessage            `json:"result,omitempty"`
}

// NewTeamOps exposes progressive discovery and read-only Lua queries as one
// model-visible tool.
func NewTeamOps(service *domain.Service) (tool.InvokableTool, error) {
	if service == nil {
		return nil, errors.New("teamops service is required")
	}
	return toolutils.InferTool(
		TeamOpsToolName,
		"发现并组合 TeamOps 的棒球队只读数据能力。先用 describe 按需了解模块；再用 query 执行定义了 main(teamops) 的受限 Lua 5.1 程序。该工具不能写入或修改任何球队数据。",
		func(ctx context.Context, input teamOpsInput) (teamOpsOutput, error) {
			switch strings.TrimSpace(input.Mode) {
			case "describe":
				if input.Program != "" {
					return teamOpsOutput{}, errors.New("teamops describe does not accept program")
				}
				description, err := service.Describe(ctx, input.Modules)
				if err != nil {
					return teamOpsOutput{}, err
				}
				return teamOpsOutput{
					Mode:    "describe",
					Runtime: &description.Runtime,
					Modules: description.Modules,
				}, nil
			case "query":
				result, err := service.Query(ctx, input.Modules, input.Program)
				if err != nil {
					return teamOpsOutput{}, err
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					return teamOpsOutput{}, fmt.Errorf("encode teamops result: %w", err)
				}
				return teamOpsOutput{Mode: "query", Result: encoded}, nil
			default:
				return teamOpsOutput{}, fmt.Errorf("teamops mode must be describe or query, got %q", input.Mode)
			}
		},
	)
}
