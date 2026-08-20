package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/google/uuid"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

const TeamModifyToolName = "team_modify"

type TeamModifier interface {
	Create(context.Context, team.ID, string) (team.Team, error)
}

type PlayerModifier interface {
	Create(context.Context, player.ID, team.ID, int, string, player.HandFlags, player.HandFlags, player.PositionFlags) (player.Player, error)
	Update(context.Context, player.ID, string, player.HandFlags, player.HandFlags, player.PositionFlags) (player.Player, error)
	SetActive(context.Context, player.ID, bool) (player.Player, error)
	ChangeJersey(context.Context, player.ID, int) (player.Player, error)
}

type GameModifier interface {
	CreateCompactGameRecord(context.Context, game.CompactGameRecordDraft) (game.GameCreateProgress, error)
	CreateNamedGameRecord(context.Context, game.NamedGameRecordDraft) (game.GameCreateProgress, error)
	CreateMatchWith(context.Context, game.MatchID, team.ID, team.ID, time.Time, string, game.MatchStatus) (game.Match, error)
	UpdateMatch(context.Context, game.MatchID, team.ID, team.ID, time.Time, string) (game.Match, error)
	SetMatchStatus(context.Context, game.MatchID, game.MatchStatus) (game.Match, error)
	DeleteMatch(context.Context, game.MatchID) error
	CreateLineupWith(context.Context, game.MatchID, team.ID, game.LineupKind, int, string, []game.LineupEntry) (game.Lineup, error)
	ReplaceLineupWith(context.Context, game.MatchID, team.ID, game.LineupKind, int, string, []game.LineupEntry) (game.Lineup, error)
	DeleteLineup(context.Context, game.MatchID, team.ID, game.LineupKind, uint16) error
}
type TrainingModifier interface {
	Create(context.Context, training.ID, player.ID, training.Date, string, string) (training.Record, error)
	Update(context.Context, training.ID, string, string) (training.Record, error)
	Delete(context.Context, training.ID) error
}

type IDGenerator func() string

type TeamModifyOption func(*teamModifyOptions)

type teamModifyOptions struct{ newID IDGenerator }

func WithTeamModifyIDGenerator(generator IDGenerator) TeamModifyOption {
	return func(options *teamModifyOptions) { options.newID = generator }
}

type teamModifyInput struct {
	Confirmed  bool                `json:"confirmed" jsonschema:"required,description=用户确认完整修改后必须为 true"`
	Operations []teamModifyRequest `json:"operations" jsonschema:"required,description=按数组顺序执行的预定义修改；单项修改也传一个元素；遇错中止且不回滚"`
}

type teamModifyRequest struct {
	Key       string         `json:"key" jsonschema:"required,description=批次内唯一且非空的步骤标识"`
	Operation string         `json:"operation" jsonschema:"required,description=通过 describe 获得的精确操作名称"`
	Arguments map[string]any `json:"arguments" jsonschema:"required,description=操作所需的结构化参数"`
}

type modifyFieldDescription struct {
	Name        string                   `json:"name"`
	Type        string                   `json:"type"`
	Required    bool                     `json:"required"`
	Description string                   `json:"description"`
	Values      []string                 `json:"values,omitempty"`
	Fields      []modifyFieldDescription `json:"fields,omitempty"`
	Item        *modifyFieldDescription  `json:"item,omitempty"`
}

type modifyTopicSummary struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type modifyTopicDescription struct {
	Name        string                   `json:"name"`
	Kind        string                   `json:"kind"`
	Found       bool                     `json:"found"`
	Summary     string                   `json:"summary,omitempty"`
	Error       string                   `json:"error,omitempty"`
	Children    []modifyTopicSummary     `json:"children,omitempty"`
	Parameters  []modifyFieldDescription `json:"parameters,omitempty"`
	Conventions []string                 `json:"conventions,omitempty"`
	Invariants  []string                 `json:"invariants,omitempty"`
	ResultType  string                   `json:"result_type,omitempty"`
}

type teamModifyOutput struct {
	Status         string               `json:"status,omitempty"`
	Results        []teamModifyResult   `json:"results,omitempty"`
	StoppedAt      *teamModifyStoppedAt `json:"stopped_at,omitempty"`
	ContextReceipt *toolContextReceipt  `json:"context_receipt,omitempty"`
}

// toolContextReceipt is a compact, model-visible record that permits the
// runtime to replace a completed, potentially large tool-call pair in a later
// model request. The full pair remains in the session and trace.
type toolContextReceipt struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Summary   string `json:"summary"`
}

type teamModifyResult struct {
	Index     int    `json:"index"`
	Key       string `json:"key"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Result    any    `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

type teamModifyStoppedAt struct {
	Index     int    `json:"index"`
	Key       string `json:"key"`
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
}

type partialModifyError struct {
	result any
	reason string
}

func (e *partialModifyError) Error() string { return e.reason }

const maxTeamModifyBatchSize = 50

type modifyOperation struct {
	name        string
	group       string
	summary     string
	parameters  []modifyFieldDescription
	conventions []string
	invariants  []string
	resultType  string
}

type teamModifyHandler struct {
	teams      TeamModifier
	players    PlayerModifier
	games      GameModifier
	training   TrainingModifier
	newID      IDGenerator
	operations map[string]modifyOperation
}

func NewTeamModify(teams TeamModifier, players PlayerModifier, games GameModifier, trainingService TrainingModifier, options ...TeamModifyOption) (tool.InvokableTool, error) {
	handler, err := newTeamModifyHandler(teams, players, games, trainingService, options...)
	if err != nil {
		return nil, err
	}
	return newTeamModifyTool(handler)
}

func newTeamModifyTool(handler *teamModifyHandler) (tool.InvokableTool, error) {
	return toolutils.InferTool(
		TeamModifyToolName,
		"执行已通过 team_describe 读取说明的预定义球队数据修改。用户确认后传入非空 operations 数组；按顺序执行，遇错中止且不回滚，并返回逐项结果。该工具不接受脚本或数据库语句。",
		handler.invoke,
	)
}

func newTeamModifyHandler(teams TeamModifier, players PlayerModifier, games GameModifier, trainingService TrainingModifier, options ...TeamModifyOption) (*teamModifyHandler, error) {
	if teams == nil || players == nil || games == nil || trainingService == nil {
		return nil, errors.New("team modify services are required")
	}
	settings := teamModifyOptions{newID: uuid.NewString}
	for _, option := range options {
		option(&settings)
	}
	if settings.newID == nil {
		return nil, errors.New("team modify id generator is required")
	}
	return &teamModifyHandler{teams: teams, players: players, games: games, training: trainingService, newID: settings.newID, operations: defaultModifyOperations()}, nil
}

func (h *teamModifyHandler) invoke(ctx context.Context, input teamModifyInput) (teamModifyOutput, error) {
	if !input.Confirmed {
		return teamModifyOutput{}, errors.New("team_modify requires confirmed=true after user confirmation")
	}
	return h.executeBatch(ctx, input.Operations)
}

func (h *teamModifyHandler) executeBatch(ctx context.Context, requests []teamModifyRequest) (teamModifyOutput, error) {
	if len(requests) == 0 {
		return teamModifyOutput{}, errors.New("team_modify operations must not be empty")
	}
	if len(requests) > maxTeamModifyBatchSize {
		return teamModifyOutput{}, fmt.Errorf("team_modify accepts at most %d operations", maxTeamModifyBatchSize)
	}
	seen := make(map[string]struct{}, len(requests))
	for index := range requests {
		request := &requests[index]
		request.Key = strings.TrimSpace(request.Key)
		request.Operation = strings.TrimSpace(request.Operation)
		if request.Key == "" {
			return teamModifyOutput{}, fmt.Errorf("team_modify operation at index %d requires a non-empty key", index)
		}
		if _, exists := seen[request.Key]; exists {
			return teamModifyOutput{}, fmt.Errorf("team_modify operation at index %d has duplicate key %q", index, request.Key)
		}
		seen[request.Key] = struct{}{}
		if _, exists := h.operations[request.Operation]; !exists {
			return teamModifyOutput{}, fmt.Errorf("team_modify operation at index %d (%s) is unknown: %q", index, request.Key, request.Operation)
		}
		if request.Arguments == nil {
			return teamModifyOutput{}, fmt.Errorf("team_modify operation at index %d (%s) requires arguments", index, request.Key)
		}
		if err := validateModifyArguments(request.Operation, request.Arguments); err != nil {
			return teamModifyOutput{}, fmt.Errorf("validate team_modify operation at index %d (%s, %s): %w", index, request.Key, request.Operation, err)
		}
	}

	output := teamModifyOutput{Status: "succeeded", Results: make([]teamModifyResult, 0, len(requests))}
	for index, request := range requests {
		result, err := h.execute(ctx, request.Operation, request.Arguments)
		if err == nil {
			output.Results = append(output.Results, teamModifyResult{Index: index, Key: request.Key, Operation: request.Operation, Status: "succeeded", Result: result})
			if len(requests) == 1 && request.Operation == "game.create" {
				if progress, ok := result.(game.GameCreateProgress); ok && progress.Status == "succeeded" {
					output.ContextReceipt = &toolContextReceipt{Kind: "completed_write", Operation: request.Operation, Summary: fmt.Sprintf("已记录 %s（主）对 %s（客）的比赛；已创建 %d 套阵容和 %d 个 Play。", progress.HomeTeamName, progress.AwayTeamName, progress.CompletedLineups, progress.CompletedPlays)}
				}
			}
			continue
		}
		reason := err.Error()
		var partial *partialModifyError
		if errors.As(err, &partial) {
			result = partial.result
		}
		output.Status = "stopped"
		output.StoppedAt = &teamModifyStoppedAt{Index: index, Key: request.Key, Operation: request.Operation, Reason: reason}
		output.Results = append(output.Results, teamModifyResult{Index: index, Key: request.Key, Operation: request.Operation, Status: "failed", Result: result, Error: reason})
		for skippedIndex := index + 1; skippedIndex < len(requests); skippedIndex++ {
			skipped := requests[skippedIndex]
			output.Results = append(output.Results, teamModifyResult{Index: skippedIndex, Key: skipped.Key, Operation: skipped.Operation, Status: "skipped", Error: fmt.Sprintf("not executed because operation at index %d (%s) failed", index, request.Key)})
		}
		break
	}
	return output, nil
}

func (h *teamModifyHandler) describe(requested []string) []modifyTopicDescription {
	if len(requested) == 0 {
		groups := []string{"team", "player", "game", "match", "lineup", "training"}
		result := make([]modifyTopicDescription, 0, len(groups))
		for _, group := range groups {
			result = append(result, h.describeGroup(group))
		}
		return result
	}
	result := make([]modifyTopicDescription, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		if name == "team" || name == "player" || name == "game" || name == "match" || name == "lineup" || name == "training" {
			result = append(result, h.describeGroup(name))
			continue
		}
		operation, exists := h.operations[name]
		if !exists {
			message := "unknown team modify topic"
			if name == "" {
				message = "team modify topic is required"
			}
			result = append(result, modifyTopicDescription{Name: name, Found: false, Error: message})
			continue
		}
		result = append(result, modifyTopicDescription{Name: operation.name, Kind: "operation", Found: true, Summary: operation.summary, Parameters: operation.parameters, Conventions: operation.conventions, Invariants: operation.invariants, ResultType: operation.resultType})
	}
	return result
}

func (h *teamModifyHandler) describeGroup(group string) modifyTopicDescription {
	children := make([]modifyTopicSummary, 0)
	for _, name := range modifyOperationOrder {
		operation := h.operations[name]
		if operation.group == group {
			children = append(children, modifyTopicSummary{Name: operation.name, Summary: operation.summary})
		}
	}
	return modifyTopicDescription{Name: group, Kind: "group", Found: true, Summary: group + " 数据修改操作。", Children: children}
}

var modifyOperationOrder = []string{"team.create", "player.create", "player.update", "player.set_active", "player.change_jersey", "game.create", "match.create", "match.update", "match.set_status", "match.delete", "lineup.create", "lineup.replace", "lineup.delete", "training.create", "training.update", "training.delete"}

func defaultModifyOperations() map[string]modifyOperation {
	hands := []string{"left", "right"}
	positions := []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}
	profile := []modifyFieldDescription{
		{Name: "name", Type: "string", Required: true, Description: "球员姓名。"},
		{Name: "batting_hands", Type: "string[]", Required: true, Description: "打击手别。", Values: hands},
		{Name: "throwing_hands", Type: "string[]", Required: true, Description: "投球手别。", Values: hands},
		{Name: "positions", Type: "string[]", Required: true, Description: "守备位置。", Values: positions},
	}
	return map[string]modifyOperation{
		"game.create":          {name: "game.create", group: "game", summary: "按球队名和背号的紧凑事件流创建一场包含双方首发及全部比赛过程的已结束比赛；服务端推导局面，不使用数据库事务。", parameters: compactGameCreateFields(), conventions: compactGameCreateConventions(), invariants: compactGameCreateInvariants(), resultType: "game_create_progress"},
		"team.create":          {name: "team.create", group: "team", summary: "创建一个启用的球队。", parameters: []modifyFieldDescription{{Name: "name", Type: "string", Required: true, Description: "球队名称。"}}, resultType: "team"},
		"player.create":        {name: "player.create", group: "player", summary: "创建一个归属球队的启用球员。", parameters: append([]modifyFieldDescription{{Name: "team_id", Type: "string", Required: true, Description: "所属球队 ID。"}, {Name: "jersey_number", Type: "integer", Required: true, Description: "0 到 99 的球衣号码。"}}, profile...), resultType: "player"},
		"player.update":        {name: "player.update", group: "player", summary: "更新球员的完整资料。", parameters: append([]modifyFieldDescription{{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}}, profile...), resultType: "player"},
		"player.set_active":    {name: "player.set_active", group: "player", summary: "启用或停用球员。", parameters: []modifyFieldDescription{{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}, {Name: "active", Type: "boolean", Required: true, Description: "目标启用状态。"}}, resultType: "player"},
		"player.change_jersey": {name: "player.change_jersey", group: "player", summary: "修改球员当前背号。", parameters: []modifyFieldDescription{{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}, {Name: "jersey_number", Type: "integer", Required: true, Description: "0 到 99 的新球衣号码。"}}, resultType: "player"},
		"match.create":         {name: "match.create", group: "match", summary: "创建比赛。", parameters: matchCreateFields(), resultType: "match"},
		"match.update":         {name: "match.update", group: "match", summary: "更新比赛安排。", parameters: append([]modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}}, matchBaseFields()...), resultType: "match"},
		"match.set_status":     {name: "match.set_status", group: "match", summary: "设置比赛状态。", parameters: []modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}, {Name: "status", Type: "string", Required: true, Description: "比赛状态。", Values: []string{"scheduled", "in_progress", "final", "cancelled"}}}, resultType: "match"},
		"match.delete":         {name: "match.delete", group: "match", summary: "软删除比赛及关联阵容和比赛过程。", parameters: []modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}}, resultType: "deleted"},
		"lineup.create":        {name: "lineup.create", group: "lineup", summary: "创建比赛阵容。", parameters: lineupFields(), resultType: "lineup"},
		"lineup.replace":       {name: "lineup.replace", group: "lineup", summary: "替换比赛阵容。", parameters: lineupFields(), resultType: "lineup"},
		"lineup.delete":        {name: "lineup.delete", group: "lineup", summary: "软删除比赛阵容。", parameters: lineupKeyFields(), resultType: "deleted"},
		"training.create":      {name: "training.create", group: "training", summary: "创建球员每日自训记录。", parameters: []modifyFieldDescription{{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}, {Name: "training_date", Type: "string", Required: true, Description: "训练日期，格式 YYYY-MM-DD。"}, {Name: "content", Type: "string", Required: true, Description: "训练内容。"}, {Name: "reflection", Type: "string", Required: false, Description: "训练感想。"}}, resultType: "training_record"},
		"training.update":      {name: "training.update", group: "training", summary: "更新自训记录的内容与感想。", parameters: []modifyFieldDescription{{Name: "training_id", Type: "string", Required: true, Description: "自训记录 ID。"}, {Name: "content", Type: "string", Required: true, Description: "训练内容。"}, {Name: "reflection", Type: "string", Required: false, Description: "训练感想。"}}, resultType: "training_record"},
		"training.delete":      {name: "training.delete", group: "training", summary: "软删除自训记录。", parameters: []modifyFieldDescription{{Name: "training_id", Type: "string", Required: true, Description: "自训记录 ID。"}}, resultType: "deleted"},
	}
}

type teamCreateArguments struct {
	Name string `json:"name"`
}
type playerProfileArguments struct {
	Name          string   `json:"name"`
	BattingHands  []string `json:"batting_hands"`
	ThrowingHands []string `json:"throwing_hands"`
	Positions     []string `json:"positions"`
}
type playerCreateArguments struct {
	TeamID       string `json:"team_id"`
	JerseyNumber *int   `json:"jersey_number"`
	playerProfileArguments
}
type playerChangeJerseyArguments struct {
	PlayerID     string `json:"player_id"`
	JerseyNumber *int   `json:"jersey_number"`
}
type playerUpdateArguments struct {
	PlayerID string `json:"player_id"`
	playerProfileArguments
}
type playerSetActiveArguments struct {
	PlayerID string `json:"player_id"`
	Active   *bool  `json:"active"`
}
type trainingCreateArguments struct {
	PlayerID     string `json:"player_id"`
	TrainingDate string `json:"training_date"`
	Content      string `json:"content"`
	Reflection   string `json:"reflection"`
}
type trainingUpdateArguments struct {
	TrainingID string `json:"training_id"`
	Content    string `json:"content"`
	Reflection string `json:"reflection"`
}
type trainingDeleteArguments struct {
	TrainingID string `json:"training_id"`
}

func (h *teamModifyHandler) execute(ctx context.Context, operation string, arguments map[string]any) (any, error) {
	switch operation {
	case "team.create":
		var input teamCreateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		value, err := h.teams.Create(ctx, team.ID(h.newID()), input.Name)
		if err != nil {
			return nil, err
		}
		return teamResult(value), nil
	case "player.create":
		var input playerCreateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if input.JerseyNumber == nil {
			return nil, errors.New("jersey_number is required")
		}
		batting, throwing, positions, err := parseProfile(input.playerProfileArguments)
		if err != nil {
			return nil, err
		}
		value, err := h.players.Create(ctx, player.ID(h.newID()), team.ID(input.TeamID), *input.JerseyNumber, input.Name, batting, throwing, positions)
		if err != nil {
			return nil, err
		}
		return playerResult(value), nil
	case "player.update":
		var input playerUpdateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		batting, throwing, positions, err := parseProfile(input.playerProfileArguments)
		if err != nil {
			return nil, err
		}
		value, err := h.players.Update(ctx, player.ID(input.PlayerID), input.Name, batting, throwing, positions)
		if err != nil {
			return nil, err
		}
		return playerResult(value), nil
	case "player.set_active":
		var input playerSetActiveArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if input.Active == nil {
			return nil, errors.New("active is required")
		}
		value, err := h.players.SetActive(ctx, player.ID(input.PlayerID), *input.Active)
		if err != nil {
			return nil, err
		}
		return playerResult(value), nil
	case "player.change_jersey":
		var input playerChangeJerseyArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if input.JerseyNumber == nil {
			return nil, errors.New("jersey_number is required")
		}
		value, err := h.players.ChangeJersey(ctx, player.ID(input.PlayerID), *input.JerseyNumber)
		if err != nil {
			return nil, err
		}
		return playerResult(value), nil
	case "training.create":
		var input trainingCreateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		date, err := training.ParseDate(input.TrainingDate)
		if err != nil {
			return nil, fmt.Errorf("parse training_date: %w", err)
		}
		value, err := h.training.Create(ctx, training.ID(h.newID()), player.ID(input.PlayerID), date, input.Content, input.Reflection)
		if err != nil {
			return nil, err
		}
		return trainingResult(value), nil
	case "training.update":
		var input trainingUpdateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		value, err := h.training.Update(ctx, training.ID(input.TrainingID), input.Content, input.Reflection)
		if err != nil {
			return nil, err
		}
		return trainingResult(value), nil
	case "training.delete":
		var input trainingDeleteArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if err := h.training.Delete(ctx, training.ID(input.TrainingID)); err != nil {
			return nil, err
		}
		return map[string]any{"training_id": input.TrainingID, "deleted": true}, nil
	default:
		return h.executeGame(ctx, operation, arguments)
	}
}

func decodeArguments(arguments map[string]any, destination any) error {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return fmt.Errorf("encode arguments: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("arguments must contain one object")
	}
	return nil
}

func validateModifyArguments(operation string, arguments map[string]any) error {
	switch operation {
	case "team.create":
		return decodeArguments(arguments, &teamCreateArguments{})
	case "player.create":
		var input playerCreateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if input.JerseyNumber == nil {
			return errors.New("jersey_number is required")
		}
		_, _, _, err := parseProfile(input.playerProfileArguments)
		return err
	case "player.update":
		var input playerUpdateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		_, _, _, err := parseProfile(input.playerProfileArguments)
		return err
	case "player.set_active":
		var input playerSetActiveArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if input.Active == nil {
			return errors.New("active is required")
		}
		return nil
	case "player.change_jersey":
		var input playerChangeJerseyArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if input.JerseyNumber == nil {
			return errors.New("jersey_number is required")
		}
		return nil
	case "training.create":
		var input trainingCreateArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if _, err := training.ParseDate(input.TrainingDate); err != nil {
			return fmt.Errorf("parse training_date: %w", err)
		}
		return nil
	case "training.update":
		return decodeArguments(arguments, &trainingUpdateArguments{})
	case "training.delete":
		return decodeArguments(arguments, &trainingDeleteArguments{})
	default:
		return validateGameArguments(operation, arguments)
	}
}

func parseProfile(input playerProfileArguments) (player.HandFlags, player.HandFlags, player.PositionFlags, error) {
	batting, err := parseHands(input.BattingHands)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("batting_hands: %w", err)
	}
	throwing, err := parseHands(input.ThrowingHands)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("throwing_hands: %w", err)
	}
	positions, err := parsePositions(input.Positions)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("positions: %w", err)
	}
	return batting, throwing, positions, nil
}

func parseHands(values []string) (player.HandFlags, error) {
	var result player.HandFlags
	for _, value := range values {
		switch value {
		case "left":
			result |= player.HandLeft
		case "right":
			result |= player.HandRight
		default:
			return 0, fmt.Errorf("unknown hand %q", value)
		}
	}
	if !result.Valid() {
		return 0, errors.New("at least one hand is required")
	}
	return result, nil
}

func parsePositions(values []string) (player.PositionFlags, error) {
	known := map[string]player.PositionFlags{"pitcher": player.PositionPitcher, "catcher": player.PositionCatcher, "first_base": player.PositionFirstBase, "second_base": player.PositionSecondBase, "shortstop": player.PositionShortstop, "third_base": player.PositionThirdBase, "outfielder": player.PositionOutfielder}
	var result player.PositionFlags
	for _, value := range values {
		flag, exists := known[value]
		if !exists {
			return 0, fmt.Errorf("unknown position %q", value)
		}
		result |= flag
	}
	if !result.Valid() {
		return 0, errors.New("at least one position is required")
	}
	return result, nil
}

func teamResult(value team.Team) map[string]any {
	return map[string]any{"id": value.ID(), "name": value.Name(), "active": value.Active()}
}
func playerResult(value player.Player) map[string]any {
	return map[string]any{"id": value.ID(), "team_id": value.TeamID(), "jersey_number": value.JerseyNumber(), "name": value.Name(), "active": value.Active()}
}

func trainingResult(value training.Record) map[string]any {
	return map[string]any{"id": value.ID(), "player_id": value.PlayerID(), "training_date": value.TrainingDate().String(), "content": value.Content(), "reflection": value.Reflection(), "created_at": value.CreatedAt().Format(time.RFC3339Nano), "updated_at": value.UpdatedAt().Format(time.RFC3339Nano)}
}
