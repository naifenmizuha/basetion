package conversation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type memoryStore struct {
	mu       sync.Mutex
	sessions map[string]Session
	saveErr  error
}

func newMemoryStore() *memoryStore { return &memoryStore{sessions: make(map[string]Session)} }

func (s *memoryStore) Load(_ context.Context, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	v.Messages = cloneMessages(v.Messages)
	return v, nil
}

func (s *memoryStore) Save(_ context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	session.Messages = cloneMessages(session.Messages)
	s.sessions[session.ID] = session
	return nil
}

type scriptedAgent struct {
	mu       sync.Mutex
	inputs   [][]*schema.AgenticMessage
	outputs  func(context.Context, int) []*adk.TypedAgentEvent[*schema.AgenticMessage]
	running  int
	maxAlive int
}

func (a *scriptedAgent) Name(context.Context) string        { return "test" }
func (a *scriptedAgent) Description(context.Context) string { return "test agent" }
func (a *scriptedAgent) Run(ctx context.Context, input *adk.TypedAgentInput[*schema.AgenticMessage], _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	a.mu.Lock()
	call := len(a.inputs)
	a.inputs = append(a.inputs, cloneMessages(input.Messages))
	a.running++
	if a.running > a.maxAlive {
		a.maxAlive = a.running
	}
	a.mu.Unlock()
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	go func() {
		defer gen.Close()
		defer func() { a.mu.Lock(); a.running--; a.mu.Unlock() }()
		for _, event := range a.outputs(ctx, call) {
			gen.Send(event)
		}
	}()
	return iter
}

func newTestService(t *testing.T, agent adk.TypedAgent[*schema.AgenticMessage], store SessionStore) *Service {
	t.Helper()
	runner := adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent, EnableStreaming: true})
	service, err := NewService(runner, store, NewSessionLocker(), nil)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Unix(100, 0) }
	return service
}

func consume(t *testing.T, iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]]) ([]*schema.AgenticMessage, error) {
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
		if event.Output != nil && event.Output.MessageOutput != nil {
			message, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				return messages, err
			}
			messages = append(messages, message)
		}
	}
}

func TestServicePersistsStreamingToolLoopAndContinuesHistory(t *testing.T) {
	store := newMemoryStore()
	agent := &scriptedAgent{outputs: func(_ context.Context, call int) []*adk.TypedAgentEvent[*schema.AgenticMessage] {
		if call == 0 {
			assistant := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{
				schema.NewContentBlockChunk(&schema.Reasoning{Text: "查资料"}, &schema.StreamingMeta{Index: 0}),
				schema.NewContentBlockChunk(&schema.FunctionToolCall{CallID: "c1", Name: "search", Arguments: `{}`}, &schema.StreamingMeta{Index: 1}),
			}}
			result := &schema.AgenticMessage{Role: schema.AgenticRoleTypeUser, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolResult{
				CallID: "c1",
				Name:   "search",
				Content: []*schema.FunctionToolResultContentBlock{{
					Type: schema.FunctionToolResultContentBlockTypeText,
					Text: &schema.UserInputText{Text: "资料"},
				}},
			})}}
			return []*adk.TypedAgentEvent[*schema.AgenticMessage]{
				adk.EventFromAgenticMessage(nil, schema.StreamReaderFromArray([]*schema.AgenticMessage{assistant}), schema.AgenticRoleTypeAssistant),
				adk.EventFromAgenticMessage(result, nil, schema.AgenticRoleTypeUser),
			}
		}
		answer := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: "继续回答"})}}
		return []*adk.TypedAgentEvent[*schema.AgenticMessage]{adk.EventFromAgenticMessage(answer, nil, schema.AgenticRoleTypeAssistant)}
	}}
	service := newTestService(t, agent, store)
	if _, err := consume(t, service.Run(context.Background(), "s1", "第一问")); err != nil {
		t.Fatal(err)
	}
	if _, err := consume(t, service.Run(context.Background(), "s1", "第二问")); err != nil {
		t.Fatal(err)
	}
	got := store.sessions["s1"]
	if len(got.Messages) != 5 {
		t.Fatalf("persisted messages = %d, want 5", len(got.Messages))
	}
	if len(agent.inputs[1]) != 4 || agent.inputs[1][3].ContentBlocks[0].UserInputText.Text != "第二问" {
		t.Fatalf("second input does not contain persisted history: %#v", agent.inputs[1])
	}
}

func TestServiceDoesNotCommitFailedOrCancelledTurn(t *testing.T) {
	store := newMemoryStore()
	store.sessions["s1"] = Session{ID: "s1", Messages: []*schema.AgenticMessage{schema.UserAgenticMessage("old")}}
	wantErr := errors.New("agent failed")
	agent := &scriptedAgent{outputs: func(ctx context.Context, call int) []*adk.TypedAgentEvent[*schema.AgenticMessage] {
		if call == 0 {
			return []*adk.TypedAgentEvent[*schema.AgenticMessage]{{Err: wantErr}}
		}
		<-ctx.Done()
		return nil
	}}
	service := newTestService(t, agent, store)
	if _, err := consume(t, service.Run(context.Background(), "s1", "failed")); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := consume(t, service.Run(ctx, "s1", "cancelled")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	if len(store.sessions["s1"].Messages) != 1 {
		t.Fatalf("failed turns changed snapshot: %#v", store.sessions["s1"].Messages)
	}
}

func TestServiceSerializesSameSessionAndReportsValidation(t *testing.T) {
	store := newMemoryStore()
	release := make(chan struct{})
	agent := &scriptedAgent{outputs: func(_ context.Context, call int) []*adk.TypedAgentEvent[*schema.AgenticMessage] {
		if call == 0 {
			<-release
		}
		message := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: "ok"})}}
		return []*adk.TypedAgentEvent[*schema.AgenticMessage]{adk.EventFromAgenticMessage(message, nil, schema.AgenticRoleTypeAssistant)}
	}}
	service := newTestService(t, agent, store)
	done1 := make(chan error, 1)
	done2 := make(chan error, 1)
	go func() { _, err := consume(t, service.Run(context.Background(), "same", "one")); done1 <- err }()
	time.Sleep(20 * time.Millisecond)
	go func() { _, err := consume(t, service.Run(context.Background(), "same", "two")); done2 <- err }()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-done1; err != nil {
		t.Fatal(err)
	}
	if err := <-done2; err != nil {
		t.Fatal(err)
	}
	if agent.maxAlive != 1 {
		t.Fatalf("same-session concurrent runs = %d, want 1", agent.maxAlive)
	}
	if _, err := consume(t, service.Run(context.Background(), "", "prompt")); err == nil {
		t.Fatal("expected validation error")
	}
}
