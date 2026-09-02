package harness

import (
	"context"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

// gameCreateVisibility exposes the large game record schema only immediately
// after the record-game Skill has been loaded in the current agent run.
type gameCreateVisibility struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
	mu   sync.RWMutex
	info *schema.ToolInfo
}

func newGameCreateVisibility() *gameCreateVisibility {
	return &gameCreateVisibility{TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]{}}
}

func (m *gameCreateVisibility) BeforeModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.AgenticMessage], _ *adk.TypedModelContext[*schema.AgenticMessage]) (context.Context, *adk.TypedChatModelAgentState[*schema.AgenticMessage], error) {
	ctx = context.WithValue(ctx, gameRecordingSourceKey{}, userMessageSnapshot(state.Messages))
	visible := make([]*schema.ToolInfo, 0, len(state.ToolInfos))
	for _, info := range state.ToolInfos {
		if info.Name != basetiontools.TeamGameCreateToolName {
			visible = append(visible, info)
			continue
		}
		m.mu.Lock()
		if m.info == nil {
			m.info = info
		}
		m.mu.Unlock()
	}
	if recordGameJustLoaded(state.Messages) {
		m.mu.RLock()
		info := m.info
		m.mu.RUnlock()
		if info != nil {
			visible = append(visible, info)
		}
	}
	state.ToolInfos = visible
	return ctx, state, nil
}

func userMessageSnapshot(messages []*schema.AgenticMessage) []*schema.AgenticMessage {
	result := make([]*schema.AgenticMessage, 0, len(messages))
	for _, message := range messages {
		if message.Role != schema.AgenticRoleTypeUser {
			continue
		}
		copyMessage := *message
		copyMessage.ContentBlocks = make([]*schema.ContentBlock, 0, len(message.ContentBlocks))
		for _, block := range message.ContentBlocks {
			if block.UserInputText != nil {
				copyMessage.ContentBlocks = append(copyMessage.ContentBlocks, schema.NewContentBlock(&schema.UserInputText{Text: block.UserInputText.Text}))
			}
		}
		if len(copyMessage.ContentBlocks) > 0 {
			result = append(result, &copyMessage)
		}
	}
	return result
}

func recordGameJustLoaded(messages []*schema.AgenticMessage) bool {
	for index := len(messages) - 1; index >= 0; index-- {
		for blockIndex := len(messages[index].ContentBlocks) - 1; blockIndex >= 0; blockIndex-- {
			block := messages[index].ContentBlocks[blockIndex]
			result := block.FunctionToolResult
			if result == nil {
				continue
			}
			if result.Name != "skill" {
				return false
			}
			for _, content := range result.Content {
				if content.Text != nil && strings.Contains(content.Text.Text, "正在启动 Skill：record-game") {
					return true
				}
			}
			return false
		}
	}
	return false
}
