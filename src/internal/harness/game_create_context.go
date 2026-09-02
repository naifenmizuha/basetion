package harness

import (
	"strings"

	"github.com/cloudwego/eino/schema"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

// compactGameCreateMessages removes the complete record after it has been
// submitted, retaining only the tool's short result for the final response.
func compactGameCreateMessages(messages []*schema.AgenticMessage) []*schema.AgenticMessage {
	hasGameCreate := false
	for _, message := range messages {
		for _, block := range message.ContentBlocks {
			if (block.FunctionToolCall != nil && (block.FunctionToolCall.Name == basetiontools.TeamGameCreateToolName || block.FunctionToolCall.Name == basetiontools.TeamGameAppendPlaysToolName)) ||
				(block.FunctionToolResult != nil && (block.FunctionToolResult.Name == basetiontools.TeamGameCreateToolName || block.FunctionToolResult.Name == basetiontools.TeamGameAppendPlaysToolName)) {
				hasGameCreate = true
				break
			}
		}
	}
	if !hasGameCreate {
		return messages
	}
	result := make([]*schema.AgenticMessage, 0, len(messages))
	for _, message := range messages {
		copyMessage := *message
		copyMessage.ContentBlocks = make([]*schema.ContentBlock, 0, len(message.ContentBlocks))
		for _, block := range message.ContentBlocks {
			if call := block.FunctionToolCall; call != nil && (call.Name == basetiontools.TeamGameCreateToolName || call.Name == basetiontools.TeamGameAppendPlaysToolName) {
				continue
			}
			if toolResult := block.FunctionToolResult; toolResult != nil && (toolResult.Name == basetiontools.TeamGameCreateToolName || toolResult.Name == basetiontools.TeamGameAppendPlaysToolName) {
				parts := make([]string, 0, len(toolResult.Content))
				for _, content := range toolResult.Content {
					if content.Text != nil {
						parts = append(parts, content.Text.Text)
					}
				}
				copyMessage.ContentBlocks = append(copyMessage.ContentBlocks, schema.NewContentBlock(&schema.UserInputText{Text: toolResult.Name + " 结果：" + strings.Join(parts, "\n")}))
				continue
			}
			copyMessage.ContentBlocks = append(copyMessage.ContentBlocks, block)
		}
		if len(copyMessage.ContentBlocks) > 0 {
			result = append(result, &copyMessage)
		}
	}
	return result
}
