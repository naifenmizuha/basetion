package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/naifenmizuha/basetion/src/internal/application/conversation"
)

type cliStore struct {
	mu       sync.Mutex
	sessions map[string]conversation.Session
}

func (s *cliStore) Load(_ context.Context, id string) (conversation.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return conversation.Session{}, conversation.ErrSessionNotFound
	}
	return v, nil
}

func (s *cliStore) Save(_ context.Context, session conversation.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

type cliAgent struct {
	mu     sync.Mutex
	inputs [][]*schema.AgenticMessage
	err    error
}

func (a *cliAgent) Name(context.Context) string        { return "cli-test" }
func (a *cliAgent) Description(context.Context) string { return "CLI test agent" }
func (a *cliAgent) Run(_ context.Context, input *adk.TypedAgentInput[*schema.AgenticMessage], _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	a.mu.Lock()
	a.inputs = append(a.inputs, append([]*schema.AgenticMessage(nil), input.Messages...))
	a.mu.Unlock()
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	if a.err != nil {
		gen.Send(&adk.TypedAgentEvent[*schema.AgenticMessage]{Err: a.err})
		gen.Close()
		return iter
	}
	message := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{
		schema.NewContentBlock(&schema.Reasoning{Text: "先检索"}),
		schema.NewContentBlock(&schema.FunctionToolCall{CallID: "call-1", Name: "skill", Arguments: `{"skill":"project-knowledge"}`}),
		schema.NewContentBlock(&schema.FunctionToolResult{CallID: "call-1", Name: "skill", Content: []*schema.FunctionToolResultContentBlock{{Type: schema.FunctionToolResultContentBlockTypeText, Text: &schema.UserInputText{Text: "六层"}}}}),
		schema.NewContentBlock(&schema.AssistantGenText{Text: "最终回答"}),
	}}
	gen.Send(adk.EventFromAgenticMessage(nil, schema.StreamReaderFromArray([]*schema.AgenticMessage{message}), schema.AgenticRoleTypeAssistant))
	gen.Close()
	return iter
}

func newCLIConversation(t *testing.T, agent adk.TypedAgent[*schema.AgenticMessage], store conversation.SessionStore) *conversation.Service {
	t.Helper()
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: true})
	service, err := conversation.NewService(runner, store, conversation.NewSessionLocker(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestExecuteEndToEndAndContinuesSession(t *testing.T) {
	store := &cliStore{sessions: make(map[string]conversation.Session)}
	agent := &cliAgent{}
	service := newCLIConversation(t, agent, store)
	var out, stderr bytes.Buffer
	if code := Execute(context.Background(), []string{"--session-id", "same", "解释", "骨架"}, service, &out, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"[思考] 先检索", "[工具调用] skill call_id=call-1", "[工具结果] skill call_id=call-1: 六层", "最终回答", "[完成]"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output %q does not contain %q", out.String(), want)
		}
	}
	out.Reset()
	if code := Execute(context.Background(), []string{"--session-id", "same", "继续"}, service, &out, &stderr); code != 0 {
		t.Fatalf("second code=%d stderr=%s", code, stderr.String())
	}
	if len(agent.inputs) != 2 || len(agent.inputs[1]) != 3 {
		t.Fatalf("continued input = %#v", agent.inputs)
	}
}

func TestExecuteCreatesSessionWhenIDIsOmitted(t *testing.T) {
	store := &cliStore{sessions: make(map[string]conversation.Session)}
	service := newCLIConversation(t, &cliAgent{}, store)
	var out, stderr bytes.Buffer
	if code := Execute(context.Background(), []string{"新会话"}, service, &out, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(out.String(), "[会话] 新建 session-") {
		t.Fatalf("output %q does not report the new session ID", out.String())
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.sessions) != 1 {
		t.Fatalf("saved sessions=%d, want 1", len(store.sessions))
	}
	for id, session := range store.sessions {
		if !strings.HasPrefix(id, "session-") || session.ID != id {
			t.Fatalf("saved session ID=%q session.ID=%q", id, session.ID)
		}
	}
}

func TestExecuteRejectsInputAndReportsAgentFailure(t *testing.T) {
	var out, stderr bytes.Buffer
	if code := Execute(context.Background(), []string{"only prompt"}, nil, &out, &stderr); code != 1 {
		t.Fatalf("nil service code=%d", code)
	}
	agent := &cliAgent{err: errors.New("boom")}
	service := newCLIConversation(t, agent, &cliStore{sessions: make(map[string]conversation.Session)})
	stderr.Reset()
	if code := Execute(context.Background(), nil, service, &out, &stderr); code != 2 || !strings.Contains(stderr.String(), usage) {
		t.Fatalf("invalid code=%d stderr=%q", code, stderr.String())
	}
	stderr.Reset()
	if code := Execute(context.Background(), []string{"--session-id", "s", "prompt"}, service, &out, &stderr); code != 1 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("failure code=%d stderr=%q", code, stderr.String())
	}
}

func TestExtractProfile(t *testing.T) {
	profile, args, err := ExtractProfile([]string{"--profile", "dev", "--session-id", "s", "prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if profile != ProfileDev || strings.Join(args, "|") != "--session-id|s|prompt" {
		t.Fatalf("profile=%q args=%q", profile, args)
	}
	profile, args, err = ExtractProfile([]string{"prompt"})
	if err != nil || profile != ProfileRun || len(args) != 1 {
		t.Fatalf("default profile=%q args=%q err=%v", profile, args, err)
	}
	if _, _, err := ExtractProfile([]string{"--profile=unknown", "prompt"}); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
