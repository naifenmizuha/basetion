package harness

import (
	"bytes"
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestStatusModelAddsTransientStatusAndUsesPreviousPromptTokens(t *testing.T) {
	base := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{
		{{
			Role: schema.AgenticRoleTypeAssistant,
			ResponseMeta: &schema.AgenticResponseMeta{TokenUsage: &schema.TokenUsage{
				PromptTokens: 42, CompletionTokens: 7, TotalTokens: 49,
			}},
		}},
		{{Role: schema.AgenticRoleTypeAssistant}},
	}}
	telemetry := NewTurnTelemetry(log.New(io.Discard, "", 0))
	ctx := telemetry.Start(context.Background(), "session-1")
	decorated := newStatusModel(base, "测试用户", 128000, func() time.Time {
		return time.Date(2026, 8, 20, 12, 34, 56, 0, time.FixedZone("CST", 8*60*60))
	})

	input := []*schema.AgenticMessage{schema.UserAgenticMessage("你好")}
	firstStream, err := decorated.Stream(ctx, input)
	consumeAgenticStream(t, mustStream(t, firstStream, err))
	secondStream, err := decorated.Stream(ctx, input)
	consumeAgenticStream(t, mustStream(t, secondStream, err))

	base.mu.Lock()
	defer base.mu.Unlock()
	if len(input) != 1 {
		t.Fatalf("caller input was mutated: %#v", input)
	}
	if len(base.inputs) != 2 || len(base.inputs[0]) != 2 || len(base.inputs[1]) != 2 {
		t.Fatalf("model inputs = %#v", base.inputs)
	}
	firstStatus := agenticText(base.inputs[0][1])
	secondStatus := agenticText(base.inputs[1][1])
	if !strings.Contains(firstStatus, "当前用户：测试用户") || !strings.Contains(firstStatus, "已用上下文：未知 / 128000 tokens") || !strings.Contains(firstStatus, "2026-08-20T12:34:56+08:00") {
		t.Fatalf("first status = %q", firstStatus)
	}
	if !strings.Contains(secondStatus, "已用上下文：42 / 128000 tokens") {
		t.Fatalf("second status = %q", secondStatus)
	}
	if strings.Contains(agenticText(base.inputs[1][0]), "状态栏信息") {
		t.Fatalf("status leaked into durable input: %#v", base.inputs[1])
	}
}

func TestTurnTelemetrySummarizesAllModelAndToolUsage(t *testing.T) {
	var logs bytes.Buffer
	telemetry := NewTurnTelemetry(log.New(&logs, "", 0))
	ctx := telemetry.Start(context.Background(), "session-2")
	metrics := metricsFromContext(ctx)
	first := metrics.beginModelRequest()
	metrics.observeModelUsage(first, &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13})
	second := metrics.beginModelRequest()
	metrics.observeModelUsage(second, &schema.TokenUsage{PromptTokens: 20, CompletionTokens: 5, TotalTokens: 25})
	metrics.recordToolCall()
	metrics.recordToolCall()

	telemetry.Finish(ctx, nil)
	got := logs.String()
	for _, want := range []string{"session_id=\"session-2\"", "status=completed", "model_requests=2", "prompt_tokens=30", "completion_tokens=8", "total_tokens=38", "tool_calls=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary %q does not contain %q", got, want)
		}
	}
}

func mustStream(t *testing.T, stream *schema.StreamReader[*schema.AgenticMessage], err error) *schema.StreamReader[*schema.AgenticMessage] {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func consumeAgenticStream(t *testing.T, stream *schema.StreamReader[*schema.AgenticMessage]) {
	t.Helper()
	defer stream.Close()
	for {
		_, err := stream.Recv()
		if err == nil {
			continue
		}
		if err == io.EOF {
			return
		}
		t.Fatal(err)
	}
}

func agenticText(message *schema.AgenticMessage) string {
	if message == nil || len(message.ContentBlocks) == 0 || message.ContentBlocks[0].UserInputText == nil {
		return ""
	}
	return message.ContentBlocks[0].UserInputText.Text
}

var _ model.AgenticModel = (*statusModel)(nil)
