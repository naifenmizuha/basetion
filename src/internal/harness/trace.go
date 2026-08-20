package harness

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type turnTraceContextKey struct{}

// ModelRequestTrace is an immutable JSON-ready capture of one request made to
// the underlying model during a conversation turn.
type ModelRequestTrace struct {
	Index      int              `json:"index"`
	TokenUsage *ModelTokenUsage `json:"token_usage,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type ModelTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// TurnTrace collects optional diagnostic data for one conversation turn. It
// is intentionally opt-in through Context so production runs retain their
// current logging and data-retention behaviour.
type TurnTrace struct {
	mu       sync.Mutex
	requests []ModelRequestTrace
}

func NewTurnTrace() *TurnTrace { return &TurnTrace{} }

// WithTurnTrace enables capture for the context and its descendants.
func WithTurnTrace(ctx context.Context, trace *TurnTrace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, turnTraceContextKey{}, trace)
}

func traceFromContext(ctx context.Context) *TurnTrace {
	trace, _ := ctx.Value(turnTraceContextKey{}).(*TurnTrace)
	return trace
}

func (t *TurnTrace) begin() int {
	if t == nil {
		return -1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests = append(t.requests, ModelRequestTrace{Index: len(t.requests) + 1})
	return len(t.requests) - 1
}

func (t *TurnTrace) recordOutput(index int, message *schema.AgenticMessage) {
	if t == nil || index < 0 || message == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if index >= len(t.requests) {
		return
	}
	if message.ResponseMeta == nil || message.ResponseMeta.TokenUsage == nil {
		return
	}
	usage := message.ResponseMeta.TokenUsage
	total := usage.TotalTokens
	if total == 0 {
		total = usage.PromptTokens + usage.CompletionTokens
	}
	current := t.requests[index].TokenUsage
	if current == nil || total > current.TotalTokens {
		t.requests[index].TokenUsage = &ModelTokenUsage{
			PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: total,
		}
	}
}

func (t *TurnTrace) recordError(index int, err error) {
	if t == nil || index < 0 || err == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if index < len(t.requests) {
		t.requests[index].Error = err.Error()
	}
}

// Snapshot returns a copy safe for JSON encoding by an entry adapter.
func (t *TurnTrace) Snapshot() []ModelRequestTrace {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]ModelRequestTrace, len(t.requests))
	for index, request := range t.requests {
		result[index] = request
		if request.TokenUsage != nil {
			usage := *request.TokenUsage
			result[index].TokenUsage = &usage
		}
	}
	return result
}
