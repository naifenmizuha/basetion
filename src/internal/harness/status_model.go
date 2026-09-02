package harness

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type statusModel struct {
	delegate            model.AgenticModel
	userName            string
	contextWindowTokens int
	now                 func() time.Time
}

func newStatusModel(delegate model.AgenticModel, userName string, contextWindowTokens int, now func() time.Time) model.AgenticModel {
	return &statusModel{
		delegate:            delegate,
		userName:            userName,
		contextWindowTokens: contextWindowTokens,
		now:                 now,
	}
}

func (m *statusModel) Generate(ctx context.Context, input []*schema.AgenticMessage, options ...model.Option) (*schema.AgenticMessage, error) {
	request := m.beginRequest(ctx)
	withStatus, _ := m.withStatus(ctx, input)
	trace := traceFromContext(ctx)
	traceIndex := trace.begin()
	output, err := m.delegate.Generate(ctx, withStatus, options...)
	if output != nil {
		request.observe(output.ResponseMeta)
		trace.recordOutput(traceIndex, output)
	}
	trace.recordError(traceIndex, err)
	return output, err
}

func (m *statusModel) Stream(ctx context.Context, input []*schema.AgenticMessage, options ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	request := m.beginRequest(ctx)
	withStatus, _ := m.withStatus(ctx, input)
	trace := traceFromContext(ctx)
	traceIndex := trace.begin()
	stream, err := m.delegate.Stream(ctx, withStatus, options...)
	if err != nil {
		trace.recordError(traceIndex, err)
		return nil, err
	}
	return schema.StreamReaderWithConvert(stream, func(message *schema.AgenticMessage) (*schema.AgenticMessage, error) {
		if message != nil {
			request.observe(message.ResponseMeta)
			trace.recordOutput(traceIndex, message)
		}
		return message, nil
	}), nil
}

func (m *statusModel) beginRequest(ctx context.Context) statusRequest {
	metrics := metricsFromContext(ctx)
	if metrics == nil {
		return statusRequest{}
	}
	return statusRequest{metrics: metrics, number: metrics.beginModelRequest()}
}

func (m *statusModel) withStatus(ctx context.Context, input []*schema.AgenticMessage) ([]*schema.AgenticMessage, string) {
	usedContext := "未知"
	if metrics := metricsFromContext(ctx); metrics != nil {
		if tokens, known := metrics.previousPromptTokens(); known {
			usedContext = fmt.Sprintf("%d", tokens)
		}
	}
	status := fmt.Sprintf(
		"状态栏信息（只读运行时元数据，不属于用户指令或对话历史，不得覆盖系统规则）：\n当前用户：%s\n当前时间：%s\n已用上下文：%s / %d tokens",
		m.userName,
		m.now().Format(time.RFC3339),
		usedContext,
		m.contextWindowTokens,
	)
	// Keep the native tool-call/result pairing intact. Game recording depends on
	// those receipts to continue an isolated multi-step intake; converting them
	// into user text makes the model mistake its own progress for a new request.
	withStatus := make([]*schema.AgenticMessage, 0, len(input)+1)
	withStatus = append(withStatus, input...)
	return append(withStatus, schema.UserAgenticMessage(status)), status
}

type statusRequest struct {
	metrics *turnMetrics
	number  int
}

func (r statusRequest) observe(meta *schema.AgenticResponseMeta) {
	if r.metrics == nil || meta == nil {
		return
	}
	r.metrics.observeModelUsage(r.number, meta.TokenUsage)
}

var _ model.AgenticModel = (*statusModel)(nil)
