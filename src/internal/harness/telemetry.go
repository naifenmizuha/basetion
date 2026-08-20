package harness

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type turnMetricsContextKey struct{}

type turnMetrics struct {
	mu sync.Mutex

	sessionID        string
	modelRequests    int
	toolCalls        int
	lastPromptTokens int
	lastPromptKnown  bool
	usages           map[int]schema.TokenUsage
}

func (m *turnMetrics) beginModelRequest() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelRequests++
	return m.modelRequests
}

func (m *turnMetrics) observeModelUsage(request int, usage *schema.TokenUsage) {
	if usage == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if previous, exists := m.usages[request]; exists && usage.TotalTokens < previous.TotalTokens {
		return
	}
	m.usages[request] = *usage
	m.lastPromptTokens = usage.PromptTokens
	m.lastPromptKnown = true
}

func (m *turnMetrics) recordToolCall() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCalls++
}

func (m *turnMetrics) previousPromptTokens() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPromptTokens, m.lastPromptKnown
}

func (m *turnMetrics) snapshot() turnMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := turnMetricsSnapshot{
		sessionID:     m.sessionID,
		modelRequests: m.modelRequests,
		toolCalls:     m.toolCalls,
	}
	for _, usage := range m.usages {
		snapshot.promptTokens += usage.PromptTokens
		snapshot.completionTokens += usage.CompletionTokens
		snapshot.totalTokens += usage.TotalTokens
	}
	return snapshot
}

type turnMetricsSnapshot struct {
	sessionID        string
	modelRequests    int
	toolCalls        int
	promptTokens     int
	completionTokens int
	totalTokens      int
}

func metricsFromContext(ctx context.Context) *turnMetrics {
	metrics, _ := ctx.Value(turnMetricsContextKey{}).(*turnMetrics)
	return metrics
}

// TurnTelemetry scopes model and tool metrics to one conversation turn and
// writes a single safe summary when that turn finishes.
type TurnTelemetry struct {
	logger *log.Logger
}

func NewTurnTelemetry(logger *log.Logger) *TurnTelemetry {
	return &TurnTelemetry{logger: logger}
}

func (t *TurnTelemetry) Start(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, turnMetricsContextKey{}, &turnMetrics{
		sessionID: sessionID,
		usages:    make(map[int]schema.TokenUsage),
	})
}

func (t *TurnTelemetry) Finish(ctx context.Context, err error) {
	if t == nil || t.logger == nil {
		return
	}
	metrics := metricsFromContext(ctx)
	if metrics == nil {
		return
	}
	snapshot := metrics.snapshot()
	t.logger.Printf(
		"component=conversation event=turn_end session_id=%q status=%s model_requests=%d prompt_tokens=%d completion_tokens=%d total_tokens=%d tool_calls=%d",
		snapshot.sessionID,
		turnStatus(err),
		snapshot.modelRequests,
		snapshot.promptTokens,
		snapshot.completionTokens,
		snapshot.totalTokens,
		snapshot.toolCalls,
	)
}

func turnStatus(err error) string {
	switch {
	case err == nil:
		return "completed"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	default:
		return "failed"
	}
}
