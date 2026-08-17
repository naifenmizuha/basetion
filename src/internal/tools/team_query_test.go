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

func TestTeamQueryToolDescribeAndQuery(t *testing.T) {
	t.Parallel()
	teamQueryTool := newTestTeamQueryTool(t)

	descriptionJSON, err := teamQueryTool.InvokableRun(context.Background(), `{"mode":"describe","topics":["roster.teams","unknown","roster.teams"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var description teamQueryOutput
	if err := json.Unmarshal([]byte(descriptionJSON), &description); err != nil {
		t.Fatal(err)
	}
	if description.Mode != "describe" || len(description.Topics) != 2 || description.Topics[0].Name != "roster.teams" || !description.Topics[0].Found || description.Topics[1].Found || description.Topics[1].Error != "unknown team query topic" {
		t.Fatalf("unexpected description: %#v", description)
	}

	queryJSON, err := teamQueryTool.InvokableRun(context.Background(), `{"mode":"query","program":"function main(team) return {total = 2 + 3} end"}`)
	if err != nil {
		t.Fatal(err)
	}
	var query struct {
		Mode   string `json:"mode"`
		Result struct {
			Total float64 `json:"total"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(queryJSON), &query); err != nil {
		t.Fatal(err)
	}
	if query.Mode != "query" || query.Result.Total != 5 {
		t.Fatalf("unexpected query output: %#v", query)
	}
}

func TestTeamQueryToolSchemaAdvertisesModes(t *testing.T) {
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
	mode, ok := parameters.Properties.Get("mode")
	if !ok || len(mode.Enum) != 2 || mode.Enum[0] != "describe" || mode.Enum[1] != "query" {
		t.Fatalf("unexpected mode schema: %#v", mode)
	}
	if _, ok := parameters.Properties.Get("topics"); !ok {
		t.Fatalf("topics field is missing from schema: %#v", parameters)
	}
}

func TestTeamQueryToolValidation(t *testing.T) {
	t.Parallel()
	teamQueryTool := newTestTeamQueryTool(t)
	tests := []string{
		`{"mode":"invalid"}`,
		`{"mode":"describe","modules":["roster"]}`,
		`{"mode":"describe","modules":[]}`,
		`{"mode":"describe","program":"function main() end"}`,
		`{"mode":"describe","program":""}`,
		`{"mode":"query","topics":["roster.players"],"program":"function main() end"}`,
		`{"mode":"query","topics":[],"program":"function main() end"}`,
		`{"mode":"query"}`,
		`{"mode":"query","modules":["game"],"program":"function main() end"}`,
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
			Arguments: `{"mode":"query","program":"function main() return {count = 4} end"}`,
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
