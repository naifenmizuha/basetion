package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	domain "github.com/naifenmizuha/basetion/internal/domain/teamops"
	infrateamops "github.com/naifenmizuha/basetion/internal/infra/teamops"
)

func newTestTeamOpsTool(t *testing.T) tool.InvokableTool {
	t.Helper()
	executor, err := infrateamops.NewLuaExecutor(infrateamops.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	service, err := domain.NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	teamOpsTool, err := NewTeamOps(service)
	if err != nil {
		t.Fatal(err)
	}
	return teamOpsTool
}

func TestTeamOpsToolDescribeAndQuery(t *testing.T) {
	t.Parallel()
	teamOpsTool := newTestTeamOpsTool(t)

	descriptionJSON, err := teamOpsTool.InvokableRun(context.Background(), `{"mode":"describe","modules":["game","game"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var description teamOpsOutput
	if err := json.Unmarshal([]byte(descriptionJSON), &description); err != nil {
		t.Fatal(err)
	}
	if description.Mode != "describe" || description.Runtime == nil || len(description.Modules) != 1 || description.Modules[0].Name != "game" {
		t.Fatalf("unexpected description: %#v", description)
	}

	queryJSON, err := teamOpsTool.InvokableRun(context.Background(), `{"mode":"query","program":"function main(teamops) return {total = 2 + 3} end"}`)
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

func TestTeamOpsToolSchemaAdvertisesModes(t *testing.T) {
	t.Parallel()
	teamOpsTool := newTestTeamOpsTool(t)
	info, err := teamOpsTool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != TeamOpsToolName || !strings.Contains(info.Desc, "只读") {
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
}

func TestTeamOpsToolValidation(t *testing.T) {
	t.Parallel()
	teamOpsTool := newTestTeamOpsTool(t)
	tests := []string{
		`{"mode":"invalid"}`,
		`{"mode":"describe","program":"function main() end"}`,
		`{"mode":"query"}`,
		`{"mode":"query","modules":["game"],"program":"function main() end"}`,
	}
	for _, input := range tests {
		if _, err := teamOpsTool.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func TestTeamOpsToolNodePreservesCallID(t *testing.T) {
	t.Parallel()
	teamOpsTool := newTestTeamOpsTool(t)
	node, err := compose.NewAgenticToolsNode(context.Background(), &compose.ToolsNodeConfig{Tools: []tool.BaseTool{teamOpsTool}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := node.Invoke(context.Background(), &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{
			CallID:    "teamops-call-1",
			Name:      TeamOpsToolName,
			Arguments: `{"mode":"query","program":"function main() return {count = 4} end"}`,
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	result := results[0].ContentBlocks[0].FunctionToolResult
	if result == nil || result.CallID != "teamops-call-1" || result.Name != TeamOpsToolName {
		t.Fatalf("unexpected tool result: %#v", result)
	}
	if !strings.Contains(result.Content[0].String(), `"count":4`) {
		t.Fatalf("unexpected tool result content: %#v", result.Content)
	}
}

func TestNewTeamOpsRequiresService(t *testing.T) {
	t.Parallel()
	if _, err := NewTeamOps(nil); err == nil {
		t.Fatal("nil teamops service was accepted")
	}
}
