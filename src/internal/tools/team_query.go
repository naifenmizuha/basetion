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
	Mode    string    `json:"mode" jsonschema:"required,description=操作模式：describe 按需加载基础数据帮助 topic，query 执行受限 Lua 5.1 查询,enum=describe,enum=query"`
	Topics  *[]string `json:"topics,omitempty" jsonschema:"description=仅 describe 使用：要读取的精确帮助 topic，可批量提供；省略时返回顶层目录"`
	Program *string   `json:"program,omitempty" jsonschema:"description=仅 query 使用：必填的 Lua 5.1 程序，必须定义 main(data)"`
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
		"执行基于球队、球员、比赛和原子 Play 的只读 Lua 查询。先用 describe 获取基础 topic；再用 query 执行定义了 main(data) 的 Lua 5.1 程序。Lua 可根据实际筛选出的比赛对象批量读取 Play，不会得到内部 ID。比赛摘要、完整记录、阵容和球员表现请使用 team_fetch。该工具不能修改任何球队数据。",
		func(ctx context.Context, input teamQueryInput) (teamQueryOutput, error) {
			switch strings.TrimSpace(input.Mode) {
			case "describe":
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
					return teamQueryOutput{}, errors.New("team_query query does not accept topics")
				}
				result, err := service.Query(ctx, stringValue(input.Program))
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
