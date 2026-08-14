package harness

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	appconfig "github.com/naifenmizuha/basetion/internal/config"
	"github.com/naifenmizuha/basetion/internal/domain/knowledge"
	domainteamops "github.com/naifenmizuha/basetion/internal/domain/teamops"
	infraknowledge "github.com/naifenmizuha/basetion/internal/infra/knowledge"
	infrateamops "github.com/naifenmizuha/basetion/internal/infra/teamops"
	basetiontools "github.com/naifenmizuha/basetion/internal/tools"
)

type scriptedAgenticModel struct {
	mu        sync.Mutex
	responses [][]*schema.AgenticMessage
	err       error
	calls     int
}

func (m *scriptedAgenticModel) Generate(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.responses) || len(m.responses[m.calls]) == 0 {
		return nil, errors.New("script exhausted")
	}
	response := m.responses[m.calls][0]
	m.calls++
	return response, nil
}

func (m *scriptedAgenticModel) Stream(_ context.Context, _ []*schema.AgenticMessage, _ ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	if m.calls >= len(m.responses) {
		return nil, errors.New("script exhausted")
	}
	response := m.responses[m.calls]
	m.calls++
	return schema.StreamReaderFromArray(response), nil
}

var _ model.AgenticModel = (*scriptedAgenticModel)(nil)

func TestMain(m *testing.M) {
	for _, name := range []string{
		"OPENAI_MODEL",
		"OPENAI_API_KEY",
		"OPENAI_API_KEY_ENV",
		"OPENAI_BASE_URL",
		"BASETION_SESSION_DIR",
		"BASETION_MAX_ITERATIONS",
		"BASETION_UNSAFE_DEBUG_DATA",
	} {
		_ = os.Unsetenv(name)
	}
	dir, err := os.MkdirTemp("", "basetion-harness-config-")
	if err != nil {
		panic(err)
	}
	path := filepath.Join(dir, "config.toml")
	contents := []byte(`[openai]
model = "test-model"
api_key_env = "BASETION_TEST_OPENAI_API_KEY"

[agent]
max_iterations = 4
`)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		panic(err)
	}
	if err := os.Setenv("BASETION_TEST_OPENAI_API_KEY", "test-key"); err != nil {
		panic(err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	if err := appconfig.InitFile(path); err != nil {
		panic(err)
	}
	if err := os.Chdir(workingDir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.Unsetenv("BASETION_TEST_OPENAI_API_KEY")
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func assistantMessage(blocks ...*schema.ContentBlock) *schema.AgenticMessage {
	return &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: blocks}
}

func collectRunnerMessages(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) ([]*schema.AgenticMessage, error) {
	t.Helper()
	var messages []*schema.AgenticMessage
	for {
		event, ok := iter.Next()
		if !ok {
			return messages, nil
		}
		if event.Err != nil {
			return messages, event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			return messages, err
		}
		if message != nil {
			messages = append(messages, message)
		}
	}
}

func TestRuntimeStreamsAgenticContentAndUsesTools(t *testing.T) {
	t.Parallel()

	service, _ := knowledge.NewService(infraknowledge.NewMemoryRetriever([]knowledge.Document{{ID: "go", Title: "Go", Content: "Go language"}}))
	searchTool, err := basetiontools.NewKnowledgeSearch(service)
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{
		{assistantMessage(
			schema.NewContentBlockChunk(&schema.Reasoning{Text: "需要检索"}, &schema.StreamingMeta{Index: 0}),
			schema.NewContentBlockChunk(&schema.FunctionToolCall{CallID: "call-1", Name: basetiontools.KnowledgeSearchToolName, Arguments: `{"query":"Go"}`}, &schema.StreamingMeta{Index: 1}),
		)},
		{assistantMessage(schema.NewContentBlockChunk(&schema.AssistantGenText{Text: "Go 是"}, &schema.StreamingMeta{Index: 0})), assistantMessage(schema.NewContentBlockChunk(&schema.AssistantGenText{Text: "编程语言。"}, &schema.StreamingMeta{Index: 0}))},
	}}
	var logs bytes.Buffer
	runtime, err := NewRuntime(context.Background(), model, []tool.BaseTool{searchTool}, log.New(&logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	messages, err := collectRunnerMessages(t, runtime.Runner.Run(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("介绍 Go")}, adk.WithCallbacks(runtime.Callback)))
	if err != nil {
		t.Fatal(err)
	}
	var sawReasoning, sawCall, sawResult, sawText bool
	for _, message := range messages {
		for _, block := range message.ContentBlocks {
			sawReasoning = sawReasoning || block.Reasoning != nil
			sawCall = sawCall || block.FunctionToolCall != nil
			sawResult = sawResult || block.FunctionToolResult != nil
			sawText = sawText || block.AssistantGenText != nil
		}
	}
	if !sawReasoning || !sawCall || !sawResult || !sawText {
		t.Fatalf("missing event content: reasoning=%v call=%v result=%v text=%v messages=%#v", sawReasoning, sawCall, sawResult, sawText, messages)
	}
	if strings.Contains(logs.String(), "介绍 Go") || strings.Contains(logs.String(), "Go language") {
		t.Fatalf("safe callbacks leaked payloads: %s", logs.String())
	}
}

func TestRuntimeUsesTeamOpsTool(t *testing.T) {
	t.Parallel()

	executor, err := infrateamops.NewLuaExecutor(infrateamops.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	service, err := domainteamops.NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	teamOpsTool, err := basetiontools.NewTeamOps(service)
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{
		{assistantMessage(schema.NewContentBlockChunk(&schema.FunctionToolCall{
			CallID:    "teamops-call",
			Name:      basetiontools.TeamOpsToolName,
			Arguments: `{"mode":"query","program":"function main() return {total = 6 + 7} end"}`,
		}, &schema.StreamingMeta{Index: 0}))},
		{assistantMessage(schema.NewContentBlockChunk(&schema.AssistantGenText{Text: "结果是 13。"}, &schema.StreamingMeta{Index: 0}))},
	}}
	runtime, err := NewRuntime(context.Background(), model, []tool.BaseTool{teamOpsTool}, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	messages, err := collectRunnerMessages(t, runtime.Runner.Query(context.Background(), "计算结果", adk.WithCallbacks(runtime.Callback)))
	if err != nil {
		t.Fatal(err)
	}
	var sawResult bool
	for _, message := range messages {
		for _, block := range message.ContentBlocks {
			if result := block.FunctionToolResult; result != nil && result.Name == basetiontools.TeamOpsToolName {
				sawResult = result.CallID == "teamops-call" && strings.Contains(result.Content[0].String(), `"total":13`)
			}
		}
	}
	if !sawResult {
		t.Fatalf("teamops result not observed: %#v", messages)
	}
}

func TestTypedAgentSupportsNonStreamingAndErrors(t *testing.T) {
	t.Parallel()

	plainModel := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{{assistantMessage(schema.NewContentBlock(&schema.AssistantGenText{Text: "done"}))}}}
	agent, err := adk.NewTypedChatModelAgent(context.Background(), &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name: "test", Model: plainModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: false})
	messages, err := collectRunnerMessages(t, runner.Query(context.Background(), "hello"))
	if err != nil || len(messages) != 1 || messages[0].ContentBlocks[0].AssistantGenText.Text != "done" {
		t.Fatalf("non-streaming run messages=%#v error=%v", messages, err)
	}

	wantErr := errors.New("model unavailable")
	errorModel := &scriptedAgenticModel{err: wantErr}
	errorAgent, err := adk.NewTypedChatModelAgent(context.Background(), &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{Name: "error", Model: errorModel})
	if err != nil {
		t.Fatal(err)
	}
	errorRunner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: errorAgent, EnableStreaming: true})
	_, err = collectRunnerMessages(t, errorRunner.Query(context.Background(), "hello"))
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("runner error = %v, want %v", err, wantErr)
	}
}

func TestUnsafeCallbackCanIncludePayload(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	handler := NewLifecycleCallback(log.New(&logs, "", 0), true)
	plainModel := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{{assistantMessage(schema.NewContentBlock(&schema.AssistantGenText{Text: "answer"}))}}}
	agent, _ := adk.NewTypedChatModelAgent(context.Background(), &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{Name: "debug", Model: plainModel})
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: false})
	_, err := collectRunnerMessages(t, runner.Query(context.Background(), "debug-payload", adk.WithCallbacks(handler)))
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "debug-payload") {
		t.Fatalf("unsafe callback did not include payload: %s", logs.String())
	}
}
