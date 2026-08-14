package harness

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	appconfig "github.com/naifenmizuha/basetion/internal/config"
)

// NewAgenticModel builds the OpenAI Responses API implementation selected by
// the application configuration.
func NewAgenticModel(ctx context.Context) (model.AgenticModel, error) {
	cfg := appconfig.Get()
	responsesModel, err := agenticopenai.NewResponsesModel(ctx, &agenticopenai.ResponsesConfig{
		Model:   cfg.OpenAI.Model,
		APIKey:  cfg.OpenAI.APIKey,
		BaseURL: cfg.OpenAI.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create agentic OpenAI model: %w", err)
	}
	return responsesModel, nil
}

var _ model.AgenticModel = (*agenticopenai.ResponsesModel)(nil)
