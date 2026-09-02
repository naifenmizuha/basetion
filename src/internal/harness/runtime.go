package harness

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
)

//go:embed prompts/system.md
var defaultInstruction string

// Runtime groups the concrete Eino types needed by the application. It does
// not define a second agent runtime abstraction.
type Runtime struct {
	Runner    *adk.TypedRunner[*schema.AgenticMessage]
	Callback  callbacks.Handler
	Telemetry *TurnTelemetry
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
	skillMiddleware, err := newSkillMiddleware(ctx)
	if err != nil {
		return nil, err
	}
	cfg := appconfig.Get()
	telemetry := NewTurnTelemetry(logger)
	agent, err := adk.NewTypedChatModelAgent(ctx, &adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
		Name:          "basetion",
		Description:   "按需加载项目知识并使用球队查询与修改能力的 Basetion 助手",
		Instruction:   defaultInstruction,
		Model:         newStatusModel(agenticModel, cfg.User.Name, cfg.OpenAI.ContextWindowTokens, time.Now),
		MaxIterations: cfg.Agent.MaxIterations,
		Handlers: []adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage]{
			skillMiddleware,
			newGameCreateVisibility(),
		},
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
		Callback:  NewLifecycleCallback(logger, cfg.Agent.UnsafeDebugData),
		Telemetry: telemetry,
	}, nil
}
