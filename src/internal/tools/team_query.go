package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
)

const TeamQueryToolName = "team_query"

type teamQueryInput struct {
	Mode    string    `json:"mode" jsonschema:"required,description=操作模式：describe 按需加载帮助 topic，query 执行受限 Lua 5.1 查询,enum=describe,enum=query"`
	Topics  *[]string `json:"topics,omitempty" jsonschema:"description=仅 describe 使用：要读取的精确帮助 topic，可批量提供；省略时返回顶层目录"`
	Modules *[]string `json:"modules,omitempty" jsonschema:"description=仅 query 使用：本次查询需要注入的能力模块名称；省略时仅运行纯 Lua"`
	Program *string   `json:"program,omitempty" jsonschema:"description=仅 query 使用：必填的 Lua 5.1 程序，必须定义 main(team)"`
}

type teamQueryOutput struct {
	Mode   string                    `json:"mode"`
	Topics []domain.TopicDescription `json:"topics,omitempty"`
	Result json.RawMessage           `json:"result,omitempty"`
}

// NewTeamQuery exposes progressive discovery and read-only Lua queries as one
// model-visible tool.
func NewTeamQuery(service *domain.Service) (tool.InvokableTool, error) {
	if service == nil {
		return nil, errors.New("team query service is required")
	}
	return toolutils.InferTool(
		TeamQueryToolName,
		"发现并查询棒球队只读数据。先用 describe 获取顶层目录或按需加载帮助 topic；再用 query 执行定义了 main(team) 的受限 Lua 5.1 程序。该工具不能修改任何球队数据。",
		func(ctx context.Context, input teamQueryInput) (teamQueryOutput, error) {
			switch strings.TrimSpace(input.Mode) {
			case "describe":
				if input.Modules != nil {
					return teamQueryOutput{}, errors.New("team_query describe does not accept modules; use topics")
				}
				if input.Program != nil {
					return teamQueryOutput{}, errors.New("team_query describe does not accept program")
				}
				description, err := service.Describe(ctx, stringSlice(input.Topics))
				if err != nil {
					return teamQueryOutput{}, err
				}
				return teamQueryOutput{
					Mode:   "describe",
					Topics: description.Topics,
				}, nil
			case "query":
				if input.Topics != nil {
					return teamQueryOutput{}, errors.New("team_query query does not accept topics; use modules")
				}
				result, err := service.Query(ctx, stringSlice(input.Modules), stringValue(input.Program))
				if err != nil {
					return teamQueryOutput{}, err
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					return teamQueryOutput{}, fmt.Errorf("encode team query result: %w", err)
				}
				return teamQueryOutput{Mode: "query", Result: encoded}, nil
			default:
				return teamQueryOutput{}, fmt.Errorf("team_query mode must be describe or query, got %q", input.Mode)
			}
		},
	)
}

func stringSlice(value *[]string) []string {
	if value == nil {
		return nil
	}
	return *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
