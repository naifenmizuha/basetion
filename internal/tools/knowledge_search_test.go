package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	domain "github.com/naifenmizuha/basetion/internal/domain/knowledge"
	infraknowledge "github.com/naifenmizuha/basetion/internal/infra/knowledge"
)

func TestKnowledgeSearchTool(t *testing.T) {
	t.Parallel()

	retriever := infraknowledge.NewMemoryRetriever([]domain.Document{{ID: "go", Title: "Go", Content: "Go language"}})
	service, _ := domain.NewService(retriever)
	searchTool, err := NewKnowledgeSearch(service)
	if err != nil {
		t.Fatal(err)
	}
	output, err := searchTool.InvokableRun(context.Background(), `{"query":"Go","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded knowledgeSearchOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(decoded.Documents) != 1 || decoded.Documents[0].ID != "go" {
		t.Fatalf("unexpected output: %#v", decoded)
	}
	if _, err := searchTool.InvokableRun(context.Background(), `{"query":" "}`); err == nil {
		t.Fatal("tool accepted empty query")
	}
}

func TestAgenticToolsNodePreservesCallID(t *testing.T) {
	t.Parallel()

	retriever := infraknowledge.NewMemoryRetriever([]domain.Document{{ID: "go", Title: "Go", Content: "Go language"}})
	service, _ := domain.NewService(retriever)
	searchTool, err := NewKnowledgeSearch(service)
	if err != nil {
		t.Fatal(err)
	}
	node, err := compose.NewAgenticToolsNode(context.Background(), &compose.ToolsNodeConfig{Tools: []tool.BaseTool{searchTool}})
	if err != nil {
		t.Fatal(err)
	}
	results, err := node.Invoke(context.Background(), &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{
			CallID:    "call-42",
			Name:      KnowledgeSearchToolName,
			Arguments: `{"query":"Go","limit":1}`,
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(results[0].ContentBlocks) != 1 {
		t.Fatalf("unexpected tool result messages: %#v", results)
	}
	result := results[0].ContentBlocks[0].FunctionToolResult
	if result == nil || result.CallID != "call-42" || result.Name != KnowledgeSearchToolName {
		t.Fatalf("tool result did not preserve association: %#v", result)
	}
	if len(result.Content) != 1 || !strings.Contains(result.Content[0].String(), "go") {
		t.Fatalf("unexpected tool result content: %#v", result.Content)
	}
}
