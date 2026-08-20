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
	output, err := m.delegate.Generate(ctx, m.withStatus(ctx, input), options...)
	if output != nil {
		request.observe(output.ResponseMeta)
	}
	return output, err
}

func (m *statusModel) Stream(ctx context.Context, input []*schema.AgenticMessage, options ...model.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	request := m.beginRequest(ctx)
	stream, err := m.delegate.Stream(ctx, m.withStatus(ctx, input), options...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderWithConvert(stream, func(message *schema.AgenticMessage) (*schema.AgenticMessage, error) {
		if message != nil {
			request.observe(message.ResponseMeta)
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

func (m *statusModel) withStatus(ctx context.Context, input []*schema.AgenticMessage) []*schema.AgenticMessage {
	usedContext := "未知"
	if metrics := metricsFromContext(ctx); metrics != nil {
		if tokens, known := metrics.previousPromptTokens(); known {
			usedContext = fmt.Sprintf("%d", tokens)
		}
	}
	status := fmt.Sprintf(
		"状态栏信息（运行时元数据，不属于对话历史）：\n当前用户：%s\n当前时间：%s\n已用上下文：%s / %d tokens",
		m.userName,
		m.now().Format(time.RFC3339),
		usedContext,
		m.contextWindowTokens,
	)
	withStatus := make([]*schema.AgenticMessage, 0, len(input)+1)
	withStatus = append(withStatus, input...)
	return append(withStatus, schema.SystemAgenticMessage(status))
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
