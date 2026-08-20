package harness

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/openai/openai-go/v3/responses"
)

// NewAgenticModel builds the OpenAI Responses API implementation selected by
// the application configuration.
func NewAgenticModel(ctx context.Context) (model.AgenticModel, error) {
	cfg := appconfig.Get()
	responsesModel, err := agenticopenai.NewResponsesModel(ctx, responsesConfig(cfg.OpenAI))
	if err != nil {
		return nil, fmt.Errorf("create agentic OpenAI model: %w", err)
	}
	return responsesModel, nil
}

func responsesConfig(cfg appconfig.OpenAIConfig) *agenticopenai.ResponsesConfig {
	result := &agenticopenai.ResponsesConfig{
		Model:         cfg.Model,
		APIKey:        cfg.APIKey,
		BaseURL:       cfg.BaseURL,
		CustomHeaders: cfg.CustomHeaders,
		Reasoning: &responses.ReasoningParam{
			Effort:  responses.ReasoningEffort(cfg.ReasoningEffort),
			Summary: responses.ReasoningSummary(cfg.ReasoningSummary),
		},
	}
	if cfg.DisableResponseStorage {
		disabled := false
		result.Store = &disabled
	}
	return result
}

var _ model.AgenticModel = (*agenticopenai.ResponsesModel)(nil)
