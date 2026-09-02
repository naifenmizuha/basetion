package harness

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	basetiontools "github.com/naifenmizuha/basetion/src/internal/tools"
)

func TestGameCreateVisibilityOnlyExposesAfterRecordGameSkill(t *testing.T) {
	middleware := newGameCreateVisibility()
	state := &adk.TypedChatModelAgentState[*schema.AgenticMessage]{ToolInfos: []*schema.ToolInfo{{Name: "team_query"}, {Name: basetiontools.TeamGameCreateToolName}}}
	_, state, err := middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil || hasToolInfo(state.ToolInfos, basetiontools.TeamGameCreateToolName) {
		t.Fatalf("initial tools=%#v err=%v", state.ToolInfos, err)
	}
	state.Messages = []*schema.AgenticMessage{toolResultMessage("skill", "skill", "正在启动 Skill：record-game")}
	_, state, err = middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil || !hasToolInfo(state.ToolInfos, basetiontools.TeamGameCreateToolName) {
		t.Fatalf("record-game tools=%#v err=%v", state.ToolInfos, err)
	}
	state.Messages = []*schema.AgenticMessage{toolResultMessage("create", basetiontools.TeamGameCreateToolName, "{}")}
	_, state, err = middleware.BeforeModelRewriteState(context.Background(), state, nil)
	if err != nil || hasToolInfo(state.ToolInfos, basetiontools.TeamGameCreateToolName) {
		t.Fatalf("after create tools=%#v err=%v", state.ToolInfos, err)
	}
}

func TestGameRecordingTurnScopeCopiesOnlyUserText(t *testing.T) {
	scope := NewGameRecordingTurnScope()
	user := schema.UserAgenticMessage("比赛资料")
	assistant := &schema.AgenticMessage{Role: schema.AgenticRoleTypeAssistant, ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolCall{Name: "team_game_create", Arguments: `{"record":"secret"}`})}}
	toolResult := toolResultMessage("call", "team_game_append_plays", "secret result")
	ctx, cleanup := scope.Begin(context.Background())
	defer cleanup()
	ctx = scope.WithMessages(ctx, []*schema.AgenticMessage{user, assistant, toolResult})
	source := gameRecordingSource(ctx)
	if len(source) != 1 || source[0].Role != schema.AgenticRoleTypeUser || source[0].ContentBlocks[0].UserInputText.Text != "比赛资料" {
		t.Fatalf("source=%#v", source)
	}
}

func toolResultMessage(callID, name, text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeUser,
		ContentBlocks: []*schema.ContentBlock{schema.NewContentBlock(&schema.FunctionToolResult{
			CallID: callID,
			Name:   name,
			Content: []*schema.FunctionToolResultContentBlock{{
				Type: schema.FunctionToolResultContentBlockTypeText,
				Text: &schema.UserInputText{Text: text},
			}},
		})},
	}
}

func hasToolInfo(infos []*schema.ToolInfo, name string) bool {
	for _, info := range infos {
		if info.Name == name {
			return true
		}
	}
	return false
}
