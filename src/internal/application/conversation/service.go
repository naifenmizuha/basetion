package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
)

// Service executes one durable business-conversation turn through Eino.
type Service struct {
	runner    *adk.TypedRunner[*schema.AgenticMessage]
	store     SessionStore
	locker    *SessionLocker
	callback  callbacks.Handler
	telemetry TurnTelemetry
	now       func() time.Time
}

// TurnTelemetry brackets a valid conversation turn with runtime observability
// state. Implementations must not retain or modify conversation messages.
type TurnTelemetry interface {
	Start(context.Context, string) context.Context
	Finish(context.Context, error)
}

// NewService creates the conversation application service.
func NewService(runner *adk.TypedRunner[*schema.AgenticMessage], store SessionStore, locker *SessionLocker, callback callbacks.Handler, telemetry ...TurnTelemetry) (*Service, error) {
	if runner == nil {
		return nil, errors.New("runner is required")
	}
	if store == nil {
		return nil, errors.New("session store is required")
	}
	if locker == nil {
		locker = NewSessionLocker()
	}
	var turnTelemetry TurnTelemetry
	if len(telemetry) > 0 {
		turnTelemetry = telemetry[0]
	}
	return &Service{runner: runner, store: store, locker: locker, callback: callback, telemetry: turnTelemetry, now: time.Now}, nil
}

// Run validates and asynchronously executes a turn. The returned events retain
// Eino's typed streaming contract; persistence commits only after clean finish.
func (s *Service) Run(ctx context.Context, sessionID, prompt string) *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.AgenticMessage]] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.TypedAgentEvent[*schema.AgenticMessage]]()
	sessionID = strings.TrimSpace(sessionID)
	prompt = strings.TrimSpace(prompt)
	if sessionID == "" || prompt == "" {
		gen.Send(errorEvent(errors.New("session ID and prompt are required")))
		gen.Close()
		return iter
	}

	go s.execute(ctx, gen, sessionID, prompt)
	return iter
}

func (s *Service) execute(ctx context.Context, gen *adk.AsyncGenerator[*adk.TypedAgentEvent[*schema.AgenticMessage]], sessionID, prompt string) {
	defer gen.Close()
	unlock := s.locker.Lock(sessionID)
	defer unlock()
	runContext := ctx
	if s.telemetry != nil {
		runContext = s.telemetry.Start(ctx, sessionID)
	}
	var outcome error
	defer func() {
		if s.telemetry != nil {
			s.telemetry.Finish(runContext, outcome)
		}
	}()

	session, err := s.store.Load(runContext, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		session = NewSession(sessionID, s.now())
	} else if err != nil {
		outcome = fmt.Errorf("load session: %w", err)
		gen.Send(errorEvent(outcome))
		return
	}

	userMessage := schema.UserAgenticMessage(prompt)
	input := append(cloneMessages(session.Messages), userMessage)
	var options []adk.AgentRunOption
	if s.callback != nil {
		options = append(options, adk.WithCallbacks(s.callback))
	}
	run := s.runner.Run(runContext, input, options...)
	generated := make([]*schema.AgenticMessage, 0)
	for {
		event, ok := run.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		forwarded, collected, collectErr := splitEvent(event)
		gen.Send(forwarded)
		if event.Err != nil {
			outcome = event.Err
			return
		}
		if collectErr != nil {
			outcome = fmt.Errorf("collect agent output: %w", collectErr)
			gen.Send(errorEvent(outcome))
			return
		}
		if collected != nil {
			generated = append(generated, collected)
		}
	}
	if err := runContext.Err(); err != nil {
		outcome = err
		gen.Send(errorEvent(outcome))
		return
	}

	session.Messages = append(input, generated...)
	session.UpdatedAt = s.now().UTC()
	if err := s.store.Save(runContext, session); err != nil {
		outcome = fmt.Errorf("save session: %w", err)
		gen.Send(errorEvent(outcome))
	}
}

func splitEvent(event *adk.TypedAgentEvent[*schema.AgenticMessage]) (*adk.TypedAgentEvent[*schema.AgenticMessage], *schema.AgenticMessage, error) {
	if event.Output == nil || event.Output.MessageOutput == nil {
		return event, nil, nil
	}
	variant := event.Output.MessageOutput
	if !variant.IsStreaming {
		return event, variant.Message, nil
	}
	if variant.MessageStream == nil {
		return event, nil, errors.New("streaming event has no message stream")
	}
	copies := variant.MessageStream.Copy(2)
	forwarded := *event
	output := *event.Output
	publicVariant := *variant
	publicVariant.MessageStream = copies[0]
	output.MessageOutput = &publicVariant
	forwarded.Output = &output

	privateVariant := *variant
	privateVariant.MessageStream = copies[1]
	message, err := privateVariant.GetMessage()
	return &forwarded, message, err
}

func errorEvent(err error) *adk.TypedAgentEvent[*schema.AgenticMessage] {
	return &adk.TypedAgentEvent[*schema.AgenticMessage]{Err: err}
}

func cloneMessages(messages []*schema.AgenticMessage) []*schema.AgenticMessage {
	return append([]*schema.AgenticMessage(nil), messages...)
}
