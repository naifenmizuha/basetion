package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	infrateamquery "github.com/naifenmizuha/basetion/src/internal/infra/teamquery"
)

func newTestTeamQueryTool(t *testing.T) tool.InvokableTool {
	t.Helper()
	executor, err := infrateamquery.NewLuaExecutor(infrateamquery.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	service, err := domain.NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	teamQueryTool, err := NewTeamQuery(service)
	if err != nil {
		t.Fatal(err)
	}
	return teamQueryTool
}

func TestTeamQueryToolExecutesQuery(t *testing.T) {
	t.Parallel()
	teamQueryTool := newTestTeamQueryTool(t)

	queryJSON, err := teamQueryTool.InvokableRun(context.Background(), `{"program":"function main(data) return {total = 2 + 3} end"}`)
	if err != nil {
		t.Fatal(err)
	}
	var query struct {
		Result struct {
			Total float64 `json:"total"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(queryJSON), &query); err != nil {
		t.Fatal(err)
	}
	if query.Result.Total != 5 {
		t.Fatalf("unexpected query output: %#v", query)
	}
}

func TestTeamQueryToolSchemaIsExecuteOnly(t *testing.T) {
	t.Parallel()
	teamQueryTool := newTestTeamQueryTool(t)
	info, err := teamQueryTool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != TeamQueryToolName || !strings.Contains(info.Desc, "只读") {
		t.Fatalf("unexpected tool info: %#v", info)
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := parameters.Properties.Get("program"); !ok {
		t.Fatalf("program field is missing from schema: %#v", parameters)
	}
	for _, field := range []string{"mode", "topics"} {
		if _, ok := parameters.Properties.Get(field); ok {
			t.Fatalf("unexpected %q in schema: %#v", field, parameters)
		}
	}
}

func TestTeamQueryToolValidation(t *testing.T) {
	t.Parallel()
	teamQueryTool := newTestTeamQueryTool(t)
	tests := []string{
		`{}`,
		`{"program":""}`,
		`{"program":"not valid Lua"}`,
	}
	for _, input := range tests {
		if _, err := teamQueryTool.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func TestTeamQueryToolNodePreservesCallID(t *testing.T) {
	t.Parallel()
	teamQueryTool := newTestTeamQueryTool(t)
	node, err := compose.NewAgenticToolsNode(context.Background(), &compose.ToolsNodeConfig{Tools: []tool.BaseTool{teamQueryTool}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := node.Invoke(context.Background(), &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{
			CallID:    "team-query-call-1",
			Name:      TeamQueryToolName,
			Arguments: `{"program":"function main() return {count = 4} end"}`,
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := results[0].ContentBlocks[0].FunctionToolResult
	if result == nil || result.CallID != "team-query-call-1" || result.Name != TeamQueryToolName {
		t.Fatalf("unexpected tool result: %#v", result)
	}
	if !strings.Contains(result.Content[0].String(), `"count":4`) {
		t.Fatalf("unexpected tool result content: %#v", result.Content)
	}
}

func TestNewTeamQueryRequiresService(t *testing.T) {
	t.Parallel()
	if _, err := NewTeamQuery(nil); err == nil {
		t.Fatal("nil team query service was accepted")
	}
}
