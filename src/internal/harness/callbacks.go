package harness

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	callbacktemplate "github.com/cloudwego/eino/utils/callbacks"
)

const maxToolLogPreviewRunes = 1600

// NewLifecycleCallback creates safe-by-default structured lifecycle logging.
func NewLifecycleCallback(logger *log.Logger, unsafeDebugData bool) callbacks.Handler {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	return callbacktemplate.NewHandlerHelper().
		AgenticAgent(&callbacktemplate.AgenticAgentCallbackHandler{
			OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *adk.TypedAgentCallbackInput[*schema.AgenticMessage]) context.Context {
				messageCount := 0
				if input != nil && input.Input != nil {
					messageCount = len(input.Input.Messages)
				}
				logger.Printf("component=agent event=start name=%q messages=%d", info.Name, messageCount)
				return ctx
			},
		}).
		AgenticModel(&callbacktemplate.AgenticModelCallbackHandler{
			OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *model.AgenticCallbackInput) context.Context {
				messages, tools := 0, 0
				if input != nil {
					messages, tools = len(input.Messages), len(input.Tools)
				}
				logger.Printf("component=agentic_model event=start name=%q messages=%d tools=%d", info.Name, messages, tools)
				if unsafeDebugData && input != nil {
					logger.Printf("component=agentic_model event=input name=%q payload=%v", info.Name, input.Messages)
				}
				return ctx
			},
			OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.AgenticCallbackOutput) context.Context {
				logger.Printf("component=agentic_model event=end name=%q", info.Name)
				if output != nil {
					reasoning, answer := modelOutputPreview(output.Message)
					if reasoning != "" {
						logger.Printf("component=agentic_model event=reasoning name=%q text=%q", info.Name, reasoning)
					}
					if answer != "" {
						logger.Printf("component=agentic_model event=answer name=%q text=%q", info.Name, answer)
					}
				}
				if unsafeDebugData && output != nil {
					logger.Printf("component=agentic_model event=output name=%q payload=%v", info.Name, output.Message)
				}
				return ctx
			},
			OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
				logger.Printf("component=agentic_model event=error name=%q error=%q", info.Name, err)
				return ctx
			},
		}).
		Tool(&callbacktemplate.ToolCallbackHandler{
			OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
				if metrics := metricsFromContext(ctx); metrics != nil {
					metrics.recordToolCall()
				}
				if input == nil {
					logger.Printf("component=tool event=start name=%q", info.Name)
				} else {
					logger.Printf("component=tool event=start name=%q arguments=%q", info.Name, toolLogPreview(info.Name, input.ArgumentsInJSON))
				}
				if unsafeDebugData && input != nil {
					logger.Printf("component=tool event=input name=%q payload=%s", info.Name, input.ArgumentsInJSON)
				}
				return ctx
			},
			OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
				if output == nil {
					logger.Printf("component=tool event=end name=%q", info.Name)
				} else {
					logger.Printf("component=tool event=end name=%q result=%q", info.Name, toolLogPreview(info.Name, output.Response))
				}
				if unsafeDebugData && output != nil {
					logger.Printf("component=tool event=output name=%q payload=%s", info.Name, output.Response)
				}
				return ctx
			},
			OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
				logger.Printf("component=tool event=error name=%q error=%q", info.Name, err)
				return ctx
			},
		}).
		Handler()
}

// modelOutputPreview emits human-readable thought summaries and final text,
// never raw extensions or signatures. It shares the tool preview limit.
func modelOutputPreview(message *schema.AgenticMessage) (string, string) {
	if message == nil {
		return "", ""
	}
	var reasoning, answer []string
	for _, block := range message.ContentBlocks {
		if block.Reasoning != nil && strings.TrimSpace(block.Reasoning.Text) != "" {
			reasoning = append(reasoning, block.Reasoning.Text)
		}
		if block.AssistantGenText != nil && strings.TrimSpace(block.AssistantGenText.Text) != "" {
			answer = append(answer, block.AssistantGenText.Text)
		}
	}
	return genericLogPreview(strings.Join(reasoning, "")), genericLogPreview(strings.Join(answer, ""))
}

func toolLogPreview(name, value string) string {
	if name == "team_game_create" {
		var arguments map[string]any
		if json.Unmarshal([]byte(value), &arguments) == nil {
			if _, exists := arguments["record"]; exists {
				arguments["record"] = "<omitted>"
			}
			if encoded, err := json.Marshal(arguments); err == nil {
				value = string(encoded)
			}
		}
	}
	return genericLogPreview(value)
}

func genericLogPreview(value string) string {
	runes := []rune(value)
	if len(runes) <= maxToolLogPreviewRunes {
		return value
	}
	return string(runes[:maxToolLogPreviewRunes]) + "… (truncated; original_runes=" + strconv.Itoa(len(runes)) + ")"
}
