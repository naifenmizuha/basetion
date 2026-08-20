package harness

import (
	"testing"

	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/openai/openai-go/v3/responses"
)

func TestResponsesConfigAppliesReasoningSettings(t *testing.T) {
	cfg := responsesConfig(appconfig.OpenAIConfig{
		Model:                  "deepseek-v4-flash",
		APIKey:                 "test-key",
		BaseURL:                "https://api.deepseek.com",
		CustomHeaders:          map[string]string{"x-context-provider": "Azure"},
		DisableResponseStorage: true,
		ReasoningEffort:        "high",
		ReasoningSummary:       "detailed",
	})

	if cfg.Model != "deepseek-v4-flash" || cfg.APIKey != "test-key" || cfg.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("unexpected base model config: %#v", cfg)
	}
	if cfg.Reasoning == nil || cfg.Reasoning.Effort != responses.ReasoningEffortHigh || cfg.Reasoning.Summary != responses.ReasoningSummaryDetailed {
		t.Fatalf("unexpected reasoning config: %#v", cfg.Reasoning)
	}
	if cfg.CustomHeaders["x-context-provider"] != "Azure" || cfg.Store == nil || *cfg.Store {
		t.Fatalf("unexpected provider settings: %#v", cfg)
	}
}
