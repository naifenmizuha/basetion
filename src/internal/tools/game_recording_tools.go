package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
)

const (
	TeamGameBeginToolName       = "team_game_begin"
	TeamGameAppendPlaysToolName = "team_game_append_plays"
	TeamGameFinalizeToolName    = "team_game_finalize"
)

// GameRecordingIntake is deliberately process-local to one nested Agent run.
// It is not a draft protocol and has no persistence or recovery surface.
type GameRecordingIntake struct {
	mu        sync.Mutex
	id        string
	record    gameCreateArguments
	begun     bool
	finalized bool
	progress  *game.GameCreateProgress
}

func NewGameRecordingIntake(id string) *GameRecordingIntake { return &GameRecordingIntake{id: id} }

func (i *GameRecordingIntake) Progress() (game.GameCreateProgress, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.progress == nil {
		return game.GameCreateProgress{}, false
	}
	return *i.progress, true
}

type gameBeginInput struct {
	HomeTeamName string                 `json:"home_team_name" jsonschema:"required,description=主队精确名称"`
	AwayTeamName string                 `json:"away_team_name" jsonschema:"required,description=客队精确名称"`
	ScheduledAt  string                 `json:"scheduled_at" jsonschema:"required,description=RFC3339 开赛时间"`
	Location     string                 `json:"location,omitempty" jsonschema:"description=比赛地点；未知时省略"`
	HomeLineup   compactLineupArguments `json:"home_lineup" jsonschema:"required,description=主队首发阵容"`
	AwayLineup   compactLineupArguments `json:"away_lineup" jsonschema:"required,description=客队首发阵容"`
}
type gameBeginOutput struct {
	IntakeID string `json:"intake_id"`
	Summary  string `json:"summary"`
}
type gameAppendPlaysInput struct {
	IntakeID string                 `json:"intake_id" jsonschema:"required,description=team_game_begin 返回的 intake_id"`
	Plays    []compactPlayArguments `json:"plays" jsonschema:"required,description=连续 Play。每项必填 inning、half(top|bottom)、batting_order、batter_jersey_number、pitcher_jersey_number、batting_result、result_description、非空 pitches；batting_result 只能是 single|double|triple|home_run|walk|intentional_walk|hit_by_pitch|strikeout|ground_out|fly_out|line_out|fielders_choice|reached_on_error|sacrifice_bunt|sacrifice_fly|interference|other，必须保留下划线，禁止 flyout、groundout、homerun。pitches 每项用 result=ball|called_strike|swinging_strike|foul|foul_tip|in_play|hit_by_pitch|intentional_ball|pitchout|other。runner_outcomes 每项必填 jersey_number、result、from_base；result 只能是 advance|score|force_out|tag_out|caught_stealing|picked_off|error_advance，禁止 safe/out。fielding_outcomes 的 position 只能是 pitcher|catcher|first_base|second_base|shortstop|third_base|outfielder，result 只能是 putout|assist|error|double_play|triple_play|passed_ball|catcher_interference|other。未知可选字段省略，所有未定义字段都会被拒绝"`
}
type gameAppendPlaysOutput struct {
	IntakeID      string `json:"intake_id"`
	ReceivedPlays int    `json:"received_plays"`
	TotalPlays    int    `json:"total_plays"`
}
type gameFinalizeInput struct {
	IntakeID string `json:"intake_id" jsonschema:"required,description=team_game_begin 返回的 intake_id"`
}
type gameFinalizeOutput struct {
	Status   string                  `json:"status"`
	Progress game.GameCreateProgress `json:"progress"`
}

type gameRecordingTools struct {
	intake *GameRecordingIntake
	games  GameModifier
}

type strictInvokableTool struct {
	inner    tool.InvokableTool
	validate func(string) error
}

func (t strictInvokableTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.inner.Info(ctx)
}
func (t strictInvokableTool) InvokableRun(ctx context.Context, arguments string, options ...tool.Option) (string, error) {
	if err := t.validate(arguments); err != nil {
		return "", err
	}
	return t.inner.InvokableRun(ctx, arguments, options...)
}

func NewGameRecordingTools(games GameModifier, intake *GameRecordingIntake) ([]tool.InvokableTool, error) {
	if games == nil || intake == nil {
		return nil, errors.New("game modifier and recording intake are required")
	}
	h := &gameRecordingTools{intake: intake, games: games}
	begin, err := toolutils.InferTool(TeamGameBeginToolName, "开始一次比赛录入；只提交比赛元数据和双方首发。", h.begin)
	if err != nil {
		return nil, err
	}
	appendTool, err := toolutils.InferTool(TeamGameAppendPlaysToolName, "向本次录入追加连续 Play。通常一次提交一个半局。参数协议严格：使用 batting_result=fly_out 和 ground_out，不接受 flyout、groundout；使用 pitches[].result，不接受 type 或 pitch_sequence；使用 runner_outcomes[].jersey_number/result，不接受 runner_jersey_number、safe 或 out。", h.append)
	if err != nil {
		return nil, err
	}
	finalize, err := toolutils.InferTool(TeamGameFinalizeToolName, "完成录入并一次性写入比赛。只在全部 Play 已提交后调用。", h.finalize)
	if err != nil {
		return nil, err
	}
	return []tool.InvokableTool{begin, strictInvokableTool{inner: appendTool, validate: validateAppendJSON}, finalize}, nil
}

func validateAppendJSON(arguments string) error {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	var input gameAppendPlaysInput
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("invalid team_game_append_plays arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid team_game_append_plays arguments: exactly one JSON object is required")
	}
	if input.IntakeID == "" || len(input.Plays) == 0 {
		return errors.New("intake_id and non-empty plays are required")
	}
	for index, play := range input.Plays {
		path := fmt.Sprintf("plays[%d]", index)
		if play.Inning == nil || play.BattingOrder == nil || play.BatterJerseyNumber == nil || play.PitcherJerseyNumber == nil || play.Half == "" || play.BattingResult == "" || play.ResultDescription == "" || len(play.Pitches) == 0 {
			return fmt.Errorf("%s requires inning, half, batting_order, batter_jersey_number, pitcher_jersey_number, batting_result, result_description and non-empty pitches", path)
		}
		if !oneOf(play.Half, "top", "bottom") || !oneOf(play.BattingResult, "single", "double", "triple", "home_run", "walk", "intentional_walk", "hit_by_pitch", "strikeout", "ground_out", "fly_out", "line_out", "fielders_choice", "reached_on_error", "sacrifice_bunt", "sacrifice_fly", "interference", "other") {
			return fmt.Errorf("%s has an unknown half or batting_result", path)
		}
		for pitchIndex, pitch := range play.Pitches {
			if !oneOf(pitch.Result, "ball", "called_strike", "swinging_strike", "foul", "foul_tip", "in_play", "hit_by_pitch", "intentional_ball", "pitchout", "other") {
				return fmt.Errorf("%s.pitches[%d].result has an unknown enum value", path, pitchIndex)
			}
		}
		for runnerIndex, runner := range play.RunnerOutcomes {
			if runner.JerseyNumber == nil || runner.FromBase == nil || !oneOf(runner.Result, "advance", "score", "force_out", "tag_out", "caught_stealing", "picked_off", "error_advance") {
				return fmt.Errorf("%s.runner_outcomes[%d] requires jersey_number, from_base and a valid result", path, runnerIndex)
			}
		}
		for fieldingIndex, fielding := range play.FieldingOutcomes {
			if fielding.JerseyNumber == nil || !oneOf(fielding.Position, "pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder") || !oneOf(fielding.Result, "putout", "assist", "error", "double_play", "triple_play", "passed_ball", "catcher_interference", "other") {
				return fmt.Errorf("%s.fielding_outcomes[%d] requires jersey_number and valid position/result", path, fieldingIndex)
			}
		}
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (h *gameRecordingTools) begin(_ context.Context, input gameBeginInput) (gameBeginOutput, error) {
	h.intake.mu.Lock()
	defer h.intake.mu.Unlock()
	if h.intake.begun {
		return gameBeginOutput{}, errors.New("game recording already begun")
	}
	if _, err := compactGameMetadata(input); err != nil {
		return gameBeginOutput{}, fmt.Errorf("validate game metadata: %w", err)
	}
	h.intake.record, h.intake.begun = gameCreateArguments{HomeTeamName: input.HomeTeamName, AwayTeamName: input.AwayTeamName, ScheduledAt: input.ScheduledAt, Location: input.Location, HomeLineup: input.HomeLineup, AwayLineup: input.AwayLineup}, true
	return gameBeginOutput{IntakeID: h.intake.id, Summary: "已接收比赛元数据和双方首发。"}, nil
}

func (h *gameRecordingTools) append(_ context.Context, input gameAppendPlaysInput) (gameAppendPlaysOutput, error) {
	h.intake.mu.Lock()
	defer h.intake.mu.Unlock()
	if !h.intake.begun || h.intake.finalized {
		return gameAppendPlaysOutput{}, errors.New("game recording is not accepting plays")
	}
	if input.IntakeID != h.intake.id {
		return gameAppendPlaysOutput{}, errors.New("unknown intake_id")
	}
	if len(input.Plays) == 0 {
		return gameAppendPlaysOutput{}, errors.New("plays are required")
	}
	h.intake.record.Plays = append(h.intake.record.Plays, input.Plays...)
	return gameAppendPlaysOutput{IntakeID: h.intake.id, ReceivedPlays: len(input.Plays), TotalPlays: len(h.intake.record.Plays)}, nil
}

func (h *gameRecordingTools) finalize(ctx context.Context, input gameFinalizeInput) (gameFinalizeOutput, error) {
	h.intake.mu.Lock()
	if !h.intake.begun || h.intake.finalized {
		h.intake.mu.Unlock()
		return gameFinalizeOutput{}, errors.New("game recording is not ready to finalize")
	}
	if input.IntakeID != h.intake.id {
		h.intake.mu.Unlock()
		return gameFinalizeOutput{}, errors.New("unknown intake_id")
	}
	record := h.intake.record
	h.intake.finalized = true
	h.intake.mu.Unlock()
	if len(record.Plays) == 0 {
		return gameFinalizeOutput{}, errors.New("no plays were submitted")
	}
	draft, err := convertCompactGame(record)
	if err != nil {
		return gameFinalizeOutput{}, fmt.Errorf("convert game record: %w", err)
	}
	progress, err := h.games.CreateCompactGameRecord(ctx, draft)
	if err != nil {
		return gameFinalizeOutput{}, err
	}
	h.intake.mu.Lock()
	h.intake.progress = &progress
	h.intake.record = gameCreateArguments{}
	h.intake.mu.Unlock()
	return gameFinalizeOutput{Status: progress.Status, Progress: progress}, nil
}

func compactGameMetadata(input gameBeginInput) (game.CompactGameRecordDraft, error) {
	result := game.CompactGameRecordDraft{HomeTeamName: input.HomeTeamName, AwayTeamName: input.AwayTeamName, ScheduledAt: input.ScheduledAt, Location: input.Location}
	if result.HomeTeamName == "" || result.AwayTeamName == "" || result.ScheduledAt == "" {
		return result, errors.New("home_team_name, away_team_name and scheduled_at are required")
	}
	lineup := func(value compactLineupArguments) (game.CompactLineupDraft, error) {
		out := game.CompactLineupDraft{Name: value.Name}
		for _, entry := range value.Entries {
			jersey, err := requiredNamedInt(entry.JerseyNumber, "jersey_number")
			if err != nil {
				return out, err
			}
			order, err := requiredNamedInt(entry.BattingOrder, "batting_order")
			if err != nil {
				return out, err
			}
			position, err := parsePositions([]string{entry.Position})
			if err != nil {
				return out, err
			}
			out.Entries = append(out.Entries, game.CompactLineupEntryDraft{JerseyNumber: jersey, BattingOrder: order, Position: position})
		}
		return out, nil
	}
	var err error
	if result.HomeLineup, err = lineup(input.HomeLineup); err != nil {
		return result, err
	}
	if result.AwayLineup, err = lineup(input.AwayLineup); err != nil {
		return result, err
	}
	return result, nil
}
