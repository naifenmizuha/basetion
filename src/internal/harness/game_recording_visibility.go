package harness

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

// gameRecordingVisibility keeps the inner protocol linear without exposing
// its complete tool set in every model request.
type gameRecordingVisibility struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
}

func newGameRecordingVisibility() *gameRecordingVisibility {
	return &gameRecordingVisibility{TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]{}}
}

func (m *gameRecordingVisibility) BeforeModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.AgenticMessage], _ *adk.TypedModelContext[*schema.AgenticMessage]) (context.Context, *adk.TypedChatModelAgentState[*schema.AgenticMessage], error) {
	stage := lastRecordingStage(state.Messages)
	visible := make([]*schema.ToolInfo, 0, 2)
	for _, info := range state.ToolInfos {
		if (stage == "new" && info.Name == basetiontools.TeamGameBeginToolName) ||
			(stage == "recording" && (info.Name == basetiontools.TeamGameAppendPlaysToolName || info.Name == basetiontools.TeamGameFinalizeToolName)) {
			visible = append(visible, info)
		}
	}
	state.ToolInfos = visible
	return ctx, state, nil
}

func lastRecordingStage(messages []*schema.AgenticMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		for j := len(messages[i].ContentBlocks) - 1; j >= 0; j-- {
			result := messages[i].ContentBlocks[j].FunctionToolResult
			if result == nil {
				continue
			}
			switch result.Name {
			case basetiontools.TeamGameFinalizeToolName:
				return "done"
			case basetiontools.TeamGameBeginToolName, basetiontools.TeamGameAppendPlaysToolName:
				return "recording"
			}
		}
	}
	return "new"
}
