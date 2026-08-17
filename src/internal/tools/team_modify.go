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
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

const TeamModifyToolName = "team_modify"

type TeamModifier interface {
	Create(context.Context, team.ID, string) (team.Team, error)
}

type PlayerModifier interface {
	Create(context.Context, player.ID, string, player.HandFlags, player.HandFlags, player.PositionFlags) (player.Player, error)
	Update(context.Context, player.ID, string, player.HandFlags, player.HandFlags, player.PositionFlags) (player.Player, error)
	SetActive(context.Context, player.ID, bool) (player.Player, error)
}

type RosterModifier interface {
	Assign(context.Context, roster.ID, team.ID, player.ID, int, roster.Date) (roster.Membership, error)
	ChangeJersey(context.Context, team.ID, roster.ID, int) (roster.Membership, error)
	Leave(context.Context, team.ID, roster.ID, roster.Date) (roster.Membership, error)
}
type GameModifier interface {
	CreateMatchWith(context.Context, game.MatchID, team.ID, team.ID, time.Time, string, game.MatchStatus) (game.Match, error)
	UpdateMatch(context.Context, game.MatchID, team.ID, team.ID, time.Time, string) (game.Match, error)
	SetMatchStatus(context.Context, game.MatchID, game.MatchStatus) (game.Match, error)
	DeleteMatch(context.Context, game.MatchID) error
	CreateLineupWith(context.Context, game.MatchID, team.ID, game.LineupKind, int, string, []game.LineupEntry) (game.Lineup, error)
	ReplaceLineupWith(context.Context, game.MatchID, team.ID, game.LineupKind, int, string, []game.LineupEntry) (game.Lineup, error)
	DeleteLineup(context.Context, game.MatchID, team.ID, game.LineupKind, uint16) error
	CreatePlateWith(context.Context, game.PlateID, game.MatchID, int, int, game.Half, int, player.ID, player.ID, string, game.PlateType, string, [3]*player.ID, int, int) (game.Plate, error)
	UpdatePlate(context.Context, game.PlateID, int, int, game.Half, int, player.ID, player.ID, string, game.PlateType, string, [3]*player.ID, int, int) (game.Plate, error)
	DeletePlate(context.Context, game.PlateID) error
}

type IDGenerator func() string

type TeamModifyOption func(*teamModifyOptions)

type teamModifyOptions struct{ newID IDGenerator }

func WithTeamModifyIDGenerator(generator IDGenerator) TeamModifyOption {
	return func(options *teamModifyOptions) { options.newID = generator }
}

type teamModifyInput struct {
	Mode       string               `json:"mode" jsonschema:"required,description=操作模式：describe 按需加载修改操作说明，execute 执行预定义操作,enum=describe,enum=execute"`
	Topics     *[]string            `json:"topics,omitempty" jsonschema:"description=仅 describe 使用：要加载的精确操作 topic；省略时返回顶层目录"`
	Operation  *string              `json:"operation,omitempty" jsonschema:"description=仅 execute 单项调用使用：通过 describe 获得的精确操作名称"`
	Arguments  *map[string]any      `json:"arguments,omitempty" jsonschema:"description=仅 execute 单项调用使用：操作所需的结构化参数"`
	Operations *[]teamModifyRequest `json:"operations,omitempty" jsonschema:"description=仅 execute 批量调用使用：按数组顺序执行的操作；与 operation 和 arguments 互斥"`
	Confirmed  *bool                `json:"confirmed,omitempty" jsonschema:"description=仅 execute 使用：用户确认完整修改后必须为 true"`
}

type teamModifyRequest struct {
	Key       string         `json:"key" jsonschema:"required,description=批次内唯一且非空的步骤标识"`
	Operation string         `json:"operation" jsonschema:"required,description=通过 describe 获得的精确操作名称"`
	Arguments map[string]any `json:"arguments" jsonschema:"required,description=操作所需的结构化参数"`
}

type modifyFieldDescription struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"`
}

type modifyTopicSummary struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type modifyTopicDescription struct {
	Name       string                   `json:"name"`
	Kind       string                   `json:"kind"`
	Found      bool                     `json:"found"`
	Summary    string                   `json:"summary,omitempty"`
	Error      string                   `json:"error,omitempty"`
	Children   []modifyTopicSummary     `json:"children,omitempty"`
	Parameters []modifyFieldDescription `json:"parameters,omitempty"`
	ResultType string                   `json:"result_type,omitempty"`
}

type teamModifyOutput struct {
	Mode      string                   `json:"mode"`
	Topics    []modifyTopicDescription `json:"topics,omitempty"`
	Operation string                   `json:"operation,omitempty"`
	Result    any                      `json:"result,omitempty"`
	Status    string                   `json:"status,omitempty"`
	Results   []teamModifyResult       `json:"results,omitempty"`
	StoppedAt *teamModifyStoppedAt     `json:"stopped_at,omitempty"`
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

const maxTeamModifyBatchSize = 50

type modifyOperation struct {
	name       string
	group      string
	summary    string
	parameters []modifyFieldDescription
	resultType string
}

type teamModifyHandler struct {
	teams      TeamModifier
	players    PlayerModifier
	rosters    RosterModifier
	games      GameModifier
	newID      IDGenerator
	operations map[string]modifyOperation
}

func NewTeamModify(teams TeamModifier, players PlayerModifier, rosters RosterModifier, games GameModifier, options ...TeamModifyOption) (tool.InvokableTool, error) {
	if teams == nil || players == nil || rosters == nil || games == nil {
		return nil, errors.New("team modify services are required")
	}
	settings := teamModifyOptions{newID: uuid.NewString}
	for _, option := range options {
		option(&settings)
	}
	if settings.newID == nil {
		return nil, errors.New("team modify id generator is required")
	}
	handler := &teamModifyHandler{teams: teams, players: players, rosters: rosters, games: games, newID: settings.newID, operations: defaultModifyOperations()}
	return toolutils.InferTool(
		TeamModifyToolName,
		"发现并执行预定义的球队数据修改操作。先用 describe 一次加载所需操作及参数；获得用户对完整修改的明确确认后，才能用 execute 执行。多个修改通过 operations 一次提交，按数组顺序执行，遇错中止且不回滚，并返回失败位置、原因和未执行步骤。该工具不接受脚本或数据库语句。",
		handler.invoke,
	)
}

func (h *teamModifyHandler) invoke(ctx context.Context, input teamModifyInput) (teamModifyOutput, error) {
	switch strings.TrimSpace(input.Mode) {
	case "describe":
		if input.Operation != nil || input.Arguments != nil || input.Operations != nil || input.Confirmed != nil {
			return teamModifyOutput{}, errors.New("team_modify describe only accepts topics")
		}
		return teamModifyOutput{Mode: "describe", Topics: h.describe(stringSlice(input.Topics))}, nil
	case "execute":
		if input.Topics != nil {
			return teamModifyOutput{}, errors.New("team_modify execute does not accept topics")
		}
		if input.Confirmed == nil || !*input.Confirmed {
			return teamModifyOutput{}, errors.New("team_modify execute requires confirmed=true after user confirmation")
		}
		if input.Operations != nil {
			if input.Operation != nil || input.Arguments != nil {
				return teamModifyOutput{}, errors.New("team_modify execute accepts either operations or operation with arguments, not both")
			}
			return h.executeBatch(ctx, *input.Operations)
		}
		operation := strings.TrimSpace(stringValue(input.Operation))
		if _, exists := h.operations[operation]; !exists {
			return teamModifyOutput{}, fmt.Errorf("unknown team modify operation %q", operation)
		}
		if input.Arguments == nil {
			return teamModifyOutput{}, errors.New("team_modify execute requires arguments")
		}
		result, err := h.execute(ctx, operation, *input.Arguments)
		if err != nil {
			return teamModifyOutput{}, fmt.Errorf("execute team modify operation %s: %w", operation, err)
		}
		return teamModifyOutput{Mode: "execute", Operation: operation, Result: result}, nil
	default:
		return teamModifyOutput{}, fmt.Errorf("team_modify mode must be describe or execute, got %q", input.Mode)
	}
}

func (h *teamModifyHandler) executeBatch(ctx context.Context, requests []teamModifyRequest) (teamModifyOutput, error) {
	if len(requests) == 0 {
		return teamModifyOutput{}, errors.New("team_modify execute operations must not be empty")
	}
	if len(requests) > maxTeamModifyBatchSize {
		return teamModifyOutput{}, fmt.Errorf("team_modify execute accepts at most %d operations", maxTeamModifyBatchSize)
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

	output := teamModifyOutput{Mode: "execute", Status: "succeeded", Results: make([]teamModifyResult, 0, len(requests))}
	for index, request := range requests {
		result, err := h.execute(ctx, request.Operation, request.Arguments)
		if err == nil {
			output.Results = append(output.Results, teamModifyResult{Index: index, Key: request.Key, Operation: request.Operation, Status: "succeeded", Result: result})
			continue
		}
		reason := err.Error()
		output.Status = "stopped"
		output.StoppedAt = &teamModifyStoppedAt{Index: index, Key: request.Key, Operation: request.Operation, Reason: reason}
		output.Results = append(output.Results, teamModifyResult{Index: index, Key: request.Key, Operation: request.Operation, Status: "failed", Error: reason})
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
		groups := []string{"team", "player", "roster", "match", "lineup", "plate"}
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
		if name == "team" || name == "player" || name == "roster" || name == "match" || name == "lineup" || name == "plate" {
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
		result = append(result, modifyTopicDescription{Name: operation.name, Kind: "operation", Found: true, Summary: operation.summary, Parameters: operation.parameters, ResultType: operation.resultType})
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

var modifyOperationOrder = []string{"team.create", "player.create", "player.update", "player.set_active", "roster.assign", "roster.change_jersey", "roster.leave", "match.create", "match.update", "match.set_status", "match.delete", "lineup.create", "lineup.replace", "lineup.delete", "plate.create", "plate.update", "plate.delete"}

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
		"team.create":          {name: "team.create", group: "team", summary: "创建一个启用的球队。", parameters: []modifyFieldDescription{{Name: "name", Type: "string", Required: true, Description: "球队名称。"}}, resultType: "team"},
		"player.create":        {name: "player.create", group: "player", summary: "创建一个启用的球员。", parameters: profile, resultType: "player"},
		"player.update":        {name: "player.update", group: "player", summary: "更新球员的完整资料。", parameters: append([]modifyFieldDescription{{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}}, profile...), resultType: "player"},
		"player.set_active":    {name: "player.set_active", group: "player", summary: "启用或停用球员。", parameters: []modifyFieldDescription{{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}, {Name: "active", Type: "boolean", Required: true, Description: "目标启用状态。"}}, resultType: "player"},
		"roster.assign":        {name: "roster.assign", group: "roster", summary: "将球员加入球队名单。", parameters: []modifyFieldDescription{{Name: "team_id", Type: "string", Required: true, Description: "球队 ID。"}, {Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"}, {Name: "jersey_number", Type: "integer", Required: true, Description: "0 到 99 的球衣号码。"}, {Name: "joined_at", Type: "string", Required: true, Description: "加入日期，格式 YYYY-MM-DD。"}}, resultType: "membership"},
		"roster.change_jersey": {name: "roster.change_jersey", group: "roster", summary: "修改当前名单成员的球衣号码。", parameters: []modifyFieldDescription{{Name: "team_id", Type: "string", Required: true, Description: "球队 ID。"}, {Name: "membership_id", Type: "string", Required: true, Description: "名单记录 ID。"}, {Name: "jersey_number", Type: "integer", Required: true, Description: "0 到 99 的新球衣号码。"}}, resultType: "membership"},
		"roster.leave":         {name: "roster.leave", group: "roster", summary: "结束球员的当前效力关系。", parameters: []modifyFieldDescription{{Name: "team_id", Type: "string", Required: true, Description: "球队 ID。"}, {Name: "membership_id", Type: "string", Required: true, Description: "名单记录 ID。"}, {Name: "left_at", Type: "string", Required: true, Description: "离队日期，格式 YYYY-MM-DD。"}}, resultType: "membership"},
		"match.create":         {name: "match.create", group: "match", summary: "创建比赛。", parameters: matchCreateFields(), resultType: "match"},
		"match.update":         {name: "match.update", group: "match", summary: "更新比赛安排。", parameters: append([]modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}}, matchBaseFields()...), resultType: "match"},
		"match.set_status":     {name: "match.set_status", group: "match", summary: "设置比赛状态。", parameters: []modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}, {Name: "status", Type: "string", Required: true, Description: "比赛状态。", Values: []string{"scheduled", "in_progress", "final", "cancelled"}}}, resultType: "match"},
		"match.delete":         {name: "match.delete", group: "match", summary: "软删除比赛及关联阵容和打席。", parameters: []modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}}, resultType: "deleted"},
		"lineup.create":        {name: "lineup.create", group: "lineup", summary: "创建比赛阵容。", parameters: lineupFields(), resultType: "lineup"},
		"lineup.replace":       {name: "lineup.replace", group: "lineup", summary: "替换比赛阵容。", parameters: lineupFields(), resultType: "lineup"},
		"lineup.delete":        {name: "lineup.delete", group: "lineup", summary: "软删除比赛阵容。", parameters: lineupKeyFields(), resultType: "deleted"},
		"plate.create":         {name: "plate.create", group: "plate", summary: "创建打席及赛后比分快照。", parameters: plateFields(true), resultType: "plate"},
		"plate.update":         {name: "plate.update", group: "plate", summary: "更新打席及赛后比分快照。", parameters: plateFields(false), resultType: "plate"},
		"plate.delete":         {name: "plate.delete", group: "plate", summary: "软删除打席。", parameters: []modifyFieldDescription{{Name: "plate_id", Type: "string", Required: true, Description: "打席 ID。"}}, resultType: "deleted"},
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
type playerUpdateArguments struct {
	PlayerID string `json:"player_id"`
	playerProfileArguments
}
type playerSetActiveArguments struct {
	PlayerID string `json:"player_id"`
	Active   *bool  `json:"active"`
}
type rosterAssignArguments struct {
	TeamID       string `json:"team_id"`
	PlayerID     string `json:"player_id"`
	JerseyNumber *int   `json:"jersey_number"`
	JoinedAt     string `json:"joined_at"`
}
type rosterChangeJerseyArguments struct {
	TeamID       string `json:"team_id"`
	MembershipID string `json:"membership_id"`
	JerseyNumber *int   `json:"jersey_number"`
}
type rosterLeaveArguments struct {
	TeamID       string `json:"team_id"`
	MembershipID string `json:"membership_id"`
	LeftAt       string `json:"left_at"`
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
		var input playerProfileArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		batting, throwing, positions, err := parseProfile(input)
		if err != nil {
			return nil, err
		}
		value, err := h.players.Create(ctx, player.ID(h.newID()), input.Name, batting, throwing, positions)
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
	case "roster.assign":
		var input rosterAssignArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if input.JerseyNumber == nil {
			return nil, errors.New("jersey_number is required")
		}
		joined, err := roster.ParseDate(input.JoinedAt)
		if err != nil {
			return nil, fmt.Errorf("parse joined_at: %w", err)
		}
		value, err := h.rosters.Assign(ctx, roster.ID(h.newID()), team.ID(input.TeamID), player.ID(input.PlayerID), *input.JerseyNumber, joined)
		if err != nil {
			return nil, err
		}
		return membershipResult(value), nil
	case "roster.change_jersey":
		var input rosterChangeJerseyArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		if input.JerseyNumber == nil {
			return nil, errors.New("jersey_number is required")
		}
		value, err := h.rosters.ChangeJersey(ctx, team.ID(input.TeamID), roster.ID(input.MembershipID), *input.JerseyNumber)
		if err != nil {
			return nil, err
		}
		return membershipResult(value), nil
	case "roster.leave":
		var input rosterLeaveArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return nil, err
		}
		left, err := roster.ParseDate(input.LeftAt)
		if err != nil {
			return nil, fmt.Errorf("parse left_at: %w", err)
		}
		value, err := h.rosters.Leave(ctx, team.ID(input.TeamID), roster.ID(input.MembershipID), left)
		if err != nil {
			return nil, err
		}
		return membershipResult(value), nil
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
		var input playerProfileArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		_, _, _, err := parseProfile(input)
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
	case "roster.assign":
		var input rosterAssignArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if input.JerseyNumber == nil {
			return errors.New("jersey_number is required")
		}
		if _, err := roster.ParseDate(input.JoinedAt); err != nil {
			return fmt.Errorf("parse joined_at: %w", err)
		}
		return nil
	case "roster.change_jersey":
		var input rosterChangeJerseyArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if input.JerseyNumber == nil {
			return errors.New("jersey_number is required")
		}
		return nil
	case "roster.leave":
		var input rosterLeaveArguments
		if err := decodeArguments(arguments, &input); err != nil {
			return err
		}
		if _, err := roster.ParseDate(input.LeftAt); err != nil {
			return fmt.Errorf("parse left_at: %w", err)
		}
		return nil
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
	return map[string]any{"id": value.ID(), "name": value.Name(), "active": value.Active(), "version": value.Version()}
}
func playerResult(value player.Player) map[string]any {
	return map[string]any{"id": value.ID(), "name": value.Name(), "active": value.Active(), "version": value.Version()}
}
func membershipResult(value roster.Membership) map[string]any {
	result := map[string]any{"id": value.ID(), "team_id": value.TeamID(), "player_id": value.PlayerID(), "jersey_number": value.JerseyNumber(), "joined_at": value.JoinedAt().String(), "version": value.Version(), "active": value.Current()}
	if left := value.LeftAt(); left != nil {
		result["left_at"] = left.String()
	} else {
		result["left_at"] = nil
	}
	return result
}
