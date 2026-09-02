package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
)

const TeamGameCreateToolName = "team_game_create"

// gameCreateInput exposes the complete compact game record as a native tool
// argument. It deliberately does not wrap the record in a JSON string.
type gameCreateInput struct {
	Confirmed bool `json:"confirmed" jsonschema:"required,description=用户已确认写入时必须为 true"`
}

type gameCreateOutput struct {
	Status         string                  `json:"status"`
	Progress       game.GameCreateProgress `json:"progress"`
	ContextReceipt *toolContextReceipt     `json:"context_receipt,omitempty"`
}

// GameRecordingRunner owns one isolated, ephemeral game-recording run.
// Its implementation belongs to Harness because it drives a nested Agent.
type GameRecordingRunner interface {
	Run(context.Context) (game.GameCreateProgress, error)
}

type gameCreateHandler struct{ runner GameRecordingRunner }

func newTeamGameCreateTool(runner GameRecordingRunner) (tool.InvokableTool, error) {
	if runner == nil {
		return nil, errors.New("game recording runner is required")
	}
	return toolutils.InferTool(TeamGameCreateToolName,
		"启动一次独立的已结束比赛录入会话。用户确认后传 confirmed=true；比赛资料从当前对话继承，不要传 record、payload、草稿或分区参数。",
		(&gameCreateHandler{runner: runner}).invoke)
}

func (h *gameCreateHandler) invoke(ctx context.Context, input gameCreateInput) (gameCreateOutput, error) {
	if !input.Confirmed {
		return gameCreateOutput{}, errors.New("team_game_create requires confirmed=true")
	}
	progress, err := h.runner.Run(ctx)
	if err != nil {
		return gameCreateOutput{}, err
	}
	output := gameCreateOutput{Status: progress.Status, Progress: progress}
	if progress.Status == "succeeded" {
		output.ContextReceipt = &toolContextReceipt{Kind: "completed_write", Operation: "game.create", Summary: fmt.Sprintf("已记录 %s（主）对 %s（客）的比赛；已创建 %d 套阵容和 %d 个 Play。", progress.HomeTeamName, progress.AwayTeamName, progress.CompletedLineups, progress.CompletedPlays)}
	}
	return output, nil
}
