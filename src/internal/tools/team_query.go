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
	Program string `json:"program" jsonschema:"required,description=必填的 Lua 5.1 程序，必须定义 main(data)；先通过 team_describe 查询可用基础 topic"`
}

type teamQueryOutput struct {
	Result json.RawMessage `json:"result,omitempty"`
}

// NewTeamQuery executes read-only Lua queries. Its discovery protocol is
// provided by team_describe so this tool only executes programs.
func NewTeamQuery(service *domain.Service) (tool.InvokableTool, error) {
	if service == nil {
		return nil, errors.New("team query service is required")
	}
	return toolutils.InferTool(
		TeamQueryToolName,
		"执行基于球队、球员、比赛和原子 Play 的只读 Lua 查询。先通过 team_describe 获取基础 topic；再执行定义了 main(data) 的 Lua 5.1 程序。Lua 可根据实际筛选出的比赛对象批量读取 Play，不会得到内部 ID。比赛摘要、完整记录、阵容和球员表现请使用 team_fetch。该工具不能修改任何球队数据。",
		func(ctx context.Context, input teamQueryInput) (teamQueryOutput, error) {
			result, err := service.Query(ctx, strings.TrimSpace(input.Program))
			if err != nil {
				return teamQueryOutput{}, err
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return teamQueryOutput{}, fmt.Errorf("encode team query result: %w", err)
			}
			return teamQueryOutput{Result: encoded}, nil
		},
	)
}
