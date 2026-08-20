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
	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
	domainteamquery "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	infrateamquery "github.com/naifenmizuha/basetion/src/internal/infra/teamquery"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

type scriptedAgenticModel struct {
	mu        sync.Mutex
	responses [][]*schema.AgenticMessage
	inputs    [][]*schema.AgenticMessage
	err       error
	calls     int
}

func (m *scriptedAgenticModel) Generate(_ context.Context, input []*schema.AgenticMessage, _ ...model.Option) (*schema.AgenticMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs = append(m.inputs, append([]*schema.AgenticMessage(nil), input...))
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

func (m *scriptedAgenticModel) Stream(_ context.Context, input []*schema.AgenticMessage, _ ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs = append(m.inputs, append([]*schema.AgenticMessage(nil), input...))
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

[database.run]
url = "postgres://test.invalid/basetion"

[database.dev]
url = "postgres://test.invalid/basetion_dev"

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
	repositoryRoot := filepath.Clean(filepath.Join(workingDir, "..", "..", ".."))
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}
	if err := appconfig.InitFile(path); err != nil {
		panic(err)
	}
	if err := os.Chdir(repositoryRoot); err != nil {
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

func TestRuntimeLoadsProjectKnowledgeSkill(t *testing.T) {
	t.Parallel()

	model := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{
		{assistantMessage(
			schema.NewContentBlockChunk(&schema.Reasoning{Text: "需要加载项目知识"}, &schema.StreamingMeta{Index: 0}),
			schema.NewContentBlockChunk(&schema.FunctionToolCall{CallID: "skill-call", Name: "skill", Arguments: `{"skill":"project-knowledge"}`}, &schema.StreamingMeta{Index: 1}),
		)},
		{assistantMessage(schema.NewContentBlockChunk(&schema.AssistantGenText{Text: "Basetion 是 Agent 应用骨架。"}, &schema.StreamingMeta{Index: 0}))},
	}}
	var logs bytes.Buffer
	runtime, err := NewRuntime(context.Background(), model, nil, log.New(&logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	messages, err := collectRunnerMessages(t, runtime.Runner.Run(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("Basetion 是什么？")}, adk.WithCallbacks(runtime.Callback)))
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
	model.mu.Lock()
	firstInput := model.inputs[0]
	model.mu.Unlock()
	if len(firstInput) == 0 || firstInput[0].Role != schema.AgenticRoleTypeSystem || len(firstInput[0].ContentBlocks) == 0 || firstInput[0].ContentBlocks[0].UserInputText == nil {
		t.Fatalf("system instruction was not injected: %#v", firstInput)
	}
	systemText := firstInput[0].ContentBlocks[0].UserInputText.Text
	if !strings.Contains(systemText, "你是 Basetion") || !strings.Contains(systemText, "必须等待 Skill 结果返回后") || len(systemText) <= len(defaultInstruction) {
		t.Fatalf("system instruction does not include the base and Skill middleware prompts: %q", systemText)
	}
	if strings.Contains(logs.String(), "Basetion 是什么") || strings.Contains(logs.String(), "六个职责区域") {
		t.Fatalf("safe callbacks leaked payloads: %s", logs.String())
	}
}

func TestRuntimeUsesTeamQueryTool(t *testing.T) {
	t.Parallel()

	executor, err := infrateamquery.NewLuaExecutor(infrateamquery.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	service, err := domainteamquery.NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	teamQueryTool, err := basetiontools.NewTeamQuery(service)
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{
		{assistantMessage(schema.NewContentBlockChunk(&schema.FunctionToolCall{
			CallID:    "skill-call",
			Name:      "skill",
			Arguments: `{"skill":"manage-team"}`,
		}, &schema.StreamingMeta{Index: 0}))},
		{assistantMessage(schema.NewContentBlockChunk(&schema.FunctionToolCall{
			CallID:    "team-query-call",
			Name:      basetiontools.TeamQueryToolName,
			Arguments: `{"mode":"query","program":"function main(data) return {total = 6 + 7} end"}`,
		}, &schema.StreamingMeta{Index: 0}))},
		{assistantMessage(schema.NewContentBlockChunk(&schema.AssistantGenText{Text: "结果是 13。"}, &schema.StreamingMeta{Index: 0}))},
	}}
	var logs bytes.Buffer
	runtime, err := NewRuntime(context.Background(), model, []tool.BaseTool{teamQueryTool}, log.New(&logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	ctx := runtime.Telemetry.Start(context.Background(), "tool-loop")
	messages, err := collectRunnerMessages(t, runtime.Runner.Query(ctx, "计算结果", adk.WithCallbacks(runtime.Callback)))
	if err != nil {
		t.Fatal(err)
	}
	runtime.Telemetry.Finish(ctx, nil)
	var sawSkillResult, sawTeamQueryResult bool
	for _, message := range messages {
		for _, block := range message.ContentBlocks {
			if result := block.FunctionToolResult; result != nil {
				sawSkillResult = sawSkillResult || result.Name == "skill" && result.CallID == "skill-call" && strings.Contains(result.Content[0].String(), "调用相应工具的 `describe`")
				sawTeamQueryResult = sawTeamQueryResult || result.Name == basetiontools.TeamQueryToolName && result.CallID == "team-query-call" && strings.Contains(result.Content[0].String(), `"total":13`)
			}
		}
	}
	if !sawSkillResult || !sawTeamQueryResult {
		t.Fatalf("expected Skill and team query results, skill=%v query=%v messages=%#v", sawSkillResult, sawTeamQueryResult, messages)
	}
	if !strings.Contains(logs.String(), "tool_calls=2") {
		t.Fatalf("tool loop summary did not count both calls: %s", logs.String())
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
