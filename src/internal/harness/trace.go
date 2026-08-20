package harness

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type turnTraceContextKey struct{}

// ModelRequestTrace is an immutable JSON-ready capture of one request made to
// the underlying model during a conversation turn.
type ModelRequestTrace struct {
	Index     int               `json:"index"`
	StatusBar string            `json:"status_bar"`
	Input     json.RawMessage   `json:"input"`
	Output    []json.RawMessage `json:"output"`
	Error     string            `json:"error,omitempty"`
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

func (t *TurnTrace) begin(status string, input []*schema.AgenticMessage) int {
	if t == nil {
		return -1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests = append(t.requests, ModelRequestTrace{
		Index:     len(t.requests) + 1,
		StatusBar: status,
		Input:     marshalTraceValue(input),
		Output:    make([]json.RawMessage, 0),
	})
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
	t.requests[index].Output = append(t.requests[index].Output, marshalTraceValue(message))
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
		result[index].Input = append(json.RawMessage(nil), request.Input...)
		result[index].Output = make([]json.RawMessage, len(request.Output))
		for outputIndex, output := range request.Output {
			result[index].Output[outputIndex] = append(json.RawMessage(nil), output...)
		}
	}
	return result
}

func marshalTraceValue(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{"trace_encoding_error":true}`)
	}
	return data
}
