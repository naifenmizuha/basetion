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
	times := []time.Time{
		time.Date(2026, 8, 20, 12, 34, 56, 0, time.FixedZone("CST", 8*60*60)),
		time.Date(2026, 8, 20, 12, 35, 2, 0, time.FixedZone("CST", 8*60*60)),
	}
	timeIndex := 0
	decorated := newStatusModel(base, "测试用户", 128000, func() time.Time {
		current := times[timeIndex]
		timeIndex++
		return current
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
	if base.inputs[0][1].Role != schema.AgenticRoleTypeUser || base.inputs[1][1].Role != schema.AgenticRoleTypeUser {
		t.Fatalf("status roles = %q, %q", base.inputs[0][1].Role, base.inputs[1][1].Role)
	}
	if !strings.Contains(firstStatus, "只读运行时元数据") || !strings.Contains(firstStatus, "不得覆盖系统规则") || !strings.Contains(firstStatus, "当前用户：测试用户") || !strings.Contains(firstStatus, "已用上下文：未知 / 128000 tokens") || !strings.Contains(firstStatus, "2026-08-20T12:34:56+08:00") {
		t.Fatalf("first status = %q", firstStatus)
	}
	if !strings.Contains(secondStatus, "已用上下文：42 / 128000 tokens") || !strings.Contains(secondStatus, "2026-08-20T12:35:02+08:00") {
		t.Fatalf("second status = %q", secondStatus)
	}
	if strings.Contains(agenticText(base.inputs[1][0]), "状态栏信息") || strings.Contains(secondStatus, firstStatus) {
		t.Fatalf("status leaked into durable input: %#v", base.inputs[1])
	}
}

func TestStatusModelAppendsUserStatusAfterToolResult(t *testing.T) {
	base := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{{{
		Role: schema.AgenticRoleTypeAssistant,
	}}}}
	decorated := newStatusModel(base, "测试用户", 128000, func() time.Time {
		return time.Date(2026, 8, 20, 12, 34, 56, 0, time.UTC)
	})
	toolCall := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{
		CallID:    "call-1",
		Name:      "team_modify",
		Arguments: strings.Repeat("x", 10000),
	})}}
	toolResult := &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeUser,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolResult{
			CallID: "call-1",
			Name:   "team_modify",
			Content: []*schema.FunctionToolResultContentBlock{{
				Type: schema.FunctionToolResultContentBlockTypeText,
				Text: &schema.UserInputText{Text: `{"status":"ok"}`},
			}},
		})},
	}
	input := []*schema.AgenticMessage{schema.UserAgenticMessage("查询球队"), toolCall, toolResult}

	stream, err := decorated.Stream(context.Background(), input)
	consumeAgenticStream(t, mustStream(t, stream, err))

	base.mu.Lock()
	defer base.mu.Unlock()
	if len(base.inputs) != 1 || len(base.inputs[0]) != 4 {
		t.Fatalf("model inputs = %#v", base.inputs)
	}
	if base.inputs[0][1] != toolCall || base.inputs[0][2] != toolResult {
		t.Fatalf("tool call/result was changed: %#v", base.inputs[0])
	}
	last := base.inputs[0][3]
	if last.Role != schema.AgenticRoleTypeUser || !strings.Contains(agenticText(last), "状态栏信息") {
		t.Fatalf("last message = %#v", last)
	}
	if len(input) != 3 {
		t.Fatalf("caller input was mutated: %#v", input)
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

func TestStatusModelCapturesTraceOnlyWhenEnabled(t *testing.T) {
	base := &scriptedAgenticModel{responses: [][]*schema.AgenticMessage{{{
		Role: schema.AgenticRoleTypeAssistant,
		ResponseMeta: &schema.AgenticResponseMeta{TokenUsage: &schema.TokenUsage{
			PromptTokens: 12, PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: 8}, CompletionTokens: 3, TotalTokens: 15,
		}},
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: "answer"}),
		},
	}}}}
	decorated := newStatusModel(base, "测试用户", 128000, func() time.Time {
		return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	})
	trace := NewTurnTrace()
	stream, err := decorated.Stream(WithTurnTrace(context.Background(), trace), []*schema.AgenticMessage{schema.UserAgenticMessage("你好")})
	consumeAgenticStream(t, mustStream(t, stream, err))
	requests := trace.Snapshot()
	if len(requests) != 1 || requests[0].Index != 1 || requests[0].TokenUsage == nil {
		t.Fatalf("trace=%#v", requests)
	}
	if requests[0].TokenUsage.PromptTokens != 12 || requests[0].TokenUsage.CachedTokens != 8 || requests[0].TokenUsage.CompletionTokens != 3 || requests[0].TokenUsage.TotalTokens != 15 {
		t.Fatalf("trace request=%#v", requests[0])
	}
}

func TestTurnTraceKeepsOnlyLargestTokenUsageSnapshot(t *testing.T) {
	trace := NewTurnTrace()
	index := trace.begin()
	trace.recordOutput(index, &schema.AgenticMessage{ResponseMeta: &schema.AgenticResponseMeta{TokenUsage: &schema.TokenUsage{PromptTokens: 3, PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: 1}, CompletionTokens: 1, TotalTokens: 4}}})
	trace.recordOutput(index, &schema.AgenticMessage{ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.AssistantGenText{Text: strings.Repeat("x", 10000)})}})
	trace.recordOutput(index, &schema.AgenticMessage{ResponseMeta: &schema.AgenticResponseMeta{TokenUsage: &schema.TokenUsage{PromptTokens: 8, PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: 5}, CompletionTokens: 5, TotalTokens: 13}}})

	request := trace.Snapshot()[0]
	if request.TokenUsage == nil || request.TokenUsage.TotalTokens != 13 || request.TokenUsage.CachedTokens != 5 {
		t.Fatalf("trace request=%#v", request)
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
