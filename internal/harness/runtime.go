package harness

import (
	"context"
	"errors"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	appconfig "github.com/naifenmizuha/basetion/internal/config"
)

const defaultInstruction = `你是 Basetion 助手。请基于当前对话上下文回答问题；需要已有项目知识时调用 search_knowledge 工具。明确区分模型显式提供的 reasoning 摘要与最终回答。`

// Runtime groups the concrete Eino types needed by the application. It does
// not define a second agent runtime abstraction.
type Runtime struct {
	Runner   *adk.TypedRunner[*schema.AgenticMessage]
	Callback callbacks.Handler
}

// NewRuntime constructs the typed Eino agent, runner, and lifecycle callback.
func NewRuntime(
	ctx context.Context,
	agenticModel model.AgenticModel,
	registeredTools []tool.BaseTool,
	logger *log.Logger,
) (*Runtime, error) {
	if agenticModel == nil {
		return nil, errors.New("agentic model is required")
	}
	cfg := appconfig.Get()
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:          "basetion",
		Description:   "使用项目知识工具回答问题的 Basetion 助手",
		Instruction:   defaultInstruction,
		Model:         agenticModel,
		MaxIterations: cfg.Agent.MaxIterations,
		ToolsConfig: adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools: registeredTools,
		}},
	})
	if err != nil {
		return nil, err
	}
	return &Runtime{
		Runner: adk.NewTypedRunner(adk.TypedRunnerConfig[*schema.AgenticMessage]{
			Agent:           agent,
			EnableStreaming: true,
		}),
		Callback: NewLifecycleCallback(logger, cfg.Agent.UnsafeDebugData),
	}, nil
}
