package tools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type matchBaseArguments struct {
	HomeTeamID  string `json:"home_team_id"`
	AwayTeamID  string `json:"away_team_id"`
	ScheduledAt string `json:"scheduled_at"`
	Location    string `json:"location"`
}
type matchCreateArguments struct {
	matchBaseArguments
	Status string `json:"status"`
}
type matchUpdateArguments struct {
	MatchID string `json:"match_id"`
	matchBaseArguments
}
type matchStatusArguments struct {
	MatchID string `json:"match_id"`
	Status  string `json:"status"`
}
type matchDeleteArguments struct {
	MatchID string `json:"match_id"`
}
type lineupEntryArguments struct {
	PlayerID     string `json:"player_id"`
	BattingOrder *int   `json:"batting_order"`
	Position     string `json:"position"`
}
type lineupArguments struct {
	MatchID       string                 `json:"match_id"`
	TeamID        string                 `json:"team_id"`
	Kind          string                 `json:"kind"`
	VariantNumber *int                   `json:"variant_number"`
	VariantName   string                 `json:"variant_name"`
	Entries       []lineupEntryArguments `json:"entries"`
}
type lineupDeleteArguments struct {
	MatchID       string `json:"match_id"`
	TeamID        string `json:"team_id"`
	Kind          string `json:"kind"`
	VariantNumber *int   `json:"variant_number"`
}
type plateArguments struct {
	PlateID           string  `json:"plate_id"`
	MatchID           string  `json:"match_id"`
	Sequence          *int    `json:"sequence"`
	Inning            *int    `json:"inning"`
	Half              string  `json:"half"`
	BattingOrder      *int    `json:"batting_order"`
	BatterID          string  `json:"batter_id"`
	PitcherID         string  `json:"pitcher_id"`
	PitchSequence     string  `json:"pitch_sequence"`
	PlateType         string  `json:"plate_type"`
	ResultDescription string  `json:"result_description"`
	RunnerOnFirstID   *string `json:"runner_on_first_id"`
	RunnerOnSecondID  *string `json:"runner_on_second_id"`
	RunnerOnThirdID   *string `json:"runner_on_third_id"`
	HomeScore         *int    `json:"home_score"`
	AwayScore         *int    `json:"away_score"`
}
type plateDeleteArguments struct {
	PlateID string `json:"plate_id"`
}

func matchBaseFields() []modifyFieldDescription {
	return []modifyFieldDescription{{Name: "home_team_id", Type: "string", Required: true, Description: "主队 ID。"}, {Name: "away_team_id", Type: "string", Required: true, Description: "客队 ID。"}, {Name: "scheduled_at", Type: "string", Required: true, Description: "RFC3339 比赛时间。"}, {Name: "location", Type: "string", Required: true, Description: "比赛地点。"}}
}
func matchCreateFields() []modifyFieldDescription {
	return append(matchBaseFields(), modifyFieldDescription{Name: "status", Type: "string", Required: true, Description: "比赛状态。", Values: []string{"scheduled", "in_progress", "final", "cancelled"}})
}
func lineupKeyFields() []modifyFieldDescription {
	return []modifyFieldDescription{{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"}, {Name: "team_id", Type: "string", Required: true, Description: "球队 ID。"}, {Name: "kind", Type: "string", Required: true, Description: "阵容种类。", Values: []string{"starter", "backup"}}, {Name: "variant_number", Type: "integer", Required: true, Description: "首发为 0，候选阵容为正数。"}}
}
func lineupFields() []modifyFieldDescription {
	return append(lineupKeyFields(), modifyFieldDescription{Name: "variant_name", Type: "string", Required: true, Description: "阵容名称。"}, modifyFieldDescription{Name: "entries", Type: "array<lineup_entry>", Required: true, Description: "球员、棒次和单一守备位置列表。"})
}
func plateFields(create bool) []modifyFieldDescription {
	fields := []modifyFieldDescription{}
	if !create {
		fields = append(fields, modifyFieldDescription{Name: "plate_id", Type: "string", Required: true, Description: "打席 ID。"})
	} else {
		fields = append(fields, modifyFieldDescription{Name: "match_id", Type: "string", Required: true, Description: "比赛 ID。"})
	}
	return append(fields, []modifyFieldDescription{{Name: "sequence", Type: "integer", Required: true, Description: "比赛内事件顺序。"}, {Name: "inning", Type: "integer", Required: true, Description: "局数。"}, {Name: "half", Type: "string", Required: true, Description: "上下半局。", Values: []string{"top", "bottom"}}, {Name: "batting_order", Type: "integer", Required: true, Description: "棒次。"}, {Name: "batter_id", Type: "string", Required: true, Description: "打者 ID。"}, {Name: "pitcher_id", Type: "string", Required: true, Description: "投手 ID。"}, {Name: "pitch_sequence", Type: "string", Required: true, Description: "B/S/F 投球序列，可为空。"}, {Name: "plate_type", Type: "string", Required: true, Description: "打席类型。", Values: plateTypeValues()}, {Name: "result_description", Type: "string", Required: true, Description: "结果说明。"}, {Name: "runner_on_first_id", Type: "string|null", Required: false, Description: "打席后的1垒跑者。"}, {Name: "runner_on_second_id", Type: "string|null", Required: false, Description: "打席后的2垒跑者。"}, {Name: "runner_on_third_id", Type: "string|null", Required: false, Description: "打席后的3垒跑者。"}, {Name: "home_score", Type: "integer", Required: true, Description: "打席后的主队累计得分。"}, {Name: "away_score", Type: "integer", Required: true, Description: "打席后的客队累计得分。"}}...)
}
func plateTypeValues() []string {
	return []string{"out", "single", "double", "triple", "home_run", "walk", "intentional_walk", "strikeout", "hit_by_pitch", "error", "fielders_choice", "sacrifice", "interference", "other"}
}

func parseStatus(v string) (game.MatchStatus, error) {
	values := map[string]game.MatchStatus{"scheduled": game.MatchScheduled, "in_progress": game.MatchInProgress, "final": game.MatchFinal, "cancelled": game.MatchCancelled}
	x, ok := values[v]
	if !ok {
		return 0, fmt.Errorf("unknown match status %q", v)
	}
	return x, nil
}
func parseLineupKind(v string) (game.LineupKind, error) {
	if v == "starter" {
		return game.LineupStarter, nil
	}
	if v == "backup" {
		return game.LineupBackup, nil
	}
	return 0, fmt.Errorf("unknown lineup kind %q", v)
}
func parseHalf(v string) (game.Half, error) {
	if v == "top" {
		return game.Top, nil
	}
	if v == "bottom" {
		return game.Bottom, nil
	}
	return 0, fmt.Errorf("unknown inning half %q", v)
}
func parsePlateType(v string) (game.PlateType, error) {
	values := plateTypeValues()
	for i, x := range values {
		if x == v {
			return game.PlateType(i + 1), nil
		}
	}
	return 0, fmt.Errorf("unknown plate type %q", v)
}
func requireInt(v *int, name string) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	return *v, nil
}
func parseScheduled(v string) (time.Time, error) {
	x, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse scheduled_at: %w", err)
	}
	return x, nil
}
func parseLineupEntries(values []lineupEntryArguments, newID IDGenerator) ([]game.LineupEntry, error) {
	if len(values) == 0 {
		return nil, errors.New("entries are required")
	}
	result := make([]game.LineupEntry, 0, len(values))
	for _, v := range values {
		order, err := requireInt(v.BattingOrder, "batting_order")
		if err != nil {
			return nil, err
		}
		position, err := parsePositions([]string{v.Position})
		if err != nil {
			return nil, err
		}
		entry, err := game.NewLineupEntry(game.LineupID(newID()), player.ID(v.PlayerID), order, position)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, nil
}
func parseRunners(v plateArguments) [3]*player.ID {
	convert := func(raw *string) *player.ID {
		if raw == nil || *raw == "" {
			return nil
		}
		id := player.ID(*raw)
		return &id
	}
	return [3]*player.ID{convert(v.RunnerOnFirstID), convert(v.RunnerOnSecondID), convert(v.RunnerOnThirdID)}
}

func validateGameArguments(operation string, arguments map[string]any) error {
	switch operation {
	case "match.create":
		var v matchCreateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		_, err := parseStatus(v.Status)
		if err != nil {
			return err
		}
		_, err = parseScheduled(v.ScheduledAt)
		return err
	case "match.update":
		var v matchUpdateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		_, err := parseScheduled(v.ScheduledAt)
		return err
	case "match.set_status":
		var v matchStatusArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		_, err := parseStatus(v.Status)
		return err
	case "match.delete":
		return decodeArguments(arguments, &matchDeleteArguments{})
	case "lineup.create", "lineup.replace":
		var v lineupArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		if _, err := parseLineupKind(v.Kind); err != nil {
			return err
		}
		_, err := requireInt(v.VariantNumber, "variant_number")
		if err != nil {
			return err
		}
		index := 0
		_, err = parseLineupEntries(v.Entries, func() string { index++; return fmt.Sprintf("validation-id-%d", index) })
		return err
	case "lineup.delete":
		var v lineupDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		if _, err := parseLineupKind(v.Kind); err != nil {
			return err
		}
		_, err := requireInt(v.VariantNumber, "variant_number")
		return err
	case "plate.create", "plate.update":
		var v plateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return err
		}
		if _, err := requireInt(v.Sequence, "sequence"); err != nil {
			return err
		}
		if _, err := requireInt(v.Inning, "inning"); err != nil {
			return err
		}
		if _, err := requireInt(v.BattingOrder, "batting_order"); err != nil {
			return err
		}
		if _, err := requireInt(v.HomeScore, "home_score"); err != nil {
			return err
		}
		if _, err := requireInt(v.AwayScore, "away_score"); err != nil {
			return err
		}
		if _, err := parseHalf(v.Half); err != nil {
			return err
		}
		_, err := parsePlateType(v.PlateType)
		return err
	case "plate.delete":
		return decodeArguments(arguments, &plateDeleteArguments{})
	default:
		return fmt.Errorf("unknown team modify operation %q", operation)
	}
}

func (h *teamModifyHandler) executeGame(ctx context.Context, operation string, arguments map[string]any) (any, error) {
	switch operation {
	case "match.create":
		var v matchCreateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		scheduled, err := parseScheduled(v.ScheduledAt)
		if err != nil {
			return nil, err
		}
		status, err := parseStatus(v.Status)
		if err != nil {
			return nil, err
		}
		value, err := h.games.CreateMatchWith(ctx, game.MatchID(h.newID()), team.ID(v.HomeTeamID), team.ID(v.AwayTeamID), scheduled, v.Location, status)
		if err != nil {
			return nil, err
		}
		return matchResult(value), nil
	case "match.update":
		var v matchUpdateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		scheduled, err := parseScheduled(v.ScheduledAt)
		if err != nil {
			return nil, err
		}
		value, err := h.games.UpdateMatch(ctx, game.MatchID(v.MatchID), team.ID(v.HomeTeamID), team.ID(v.AwayTeamID), scheduled, v.Location)
		if err != nil {
			return nil, err
		}
		return matchResult(value), nil
	case "match.set_status":
		var v matchStatusArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		status, err := parseStatus(v.Status)
		if err != nil {
			return nil, err
		}
		value, err := h.games.SetMatchStatus(ctx, game.MatchID(v.MatchID), status)
		if err != nil {
			return nil, err
		}
		return matchResult(value), nil
	case "match.delete":
		var v matchDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		if err := h.games.DeleteMatch(ctx, game.MatchID(v.MatchID)); err != nil {
			return nil, err
		}
		return map[string]any{"match_id": v.MatchID, "deleted": true}, nil
	case "lineup.create", "lineup.replace":
		var v lineupArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		kind, err := parseLineupKind(v.Kind)
		if err != nil {
			return nil, err
		}
		number, err := requireInt(v.VariantNumber, "variant_number")
		if err != nil {
			return nil, err
		}
		entries, err := parseLineupEntries(v.Entries, h.newID)
		if err != nil {
			return nil, err
		}
		var value game.Lineup
		if operation == "lineup.create" {
			value, err = h.games.CreateLineupWith(ctx, game.MatchID(v.MatchID), team.ID(v.TeamID), kind, number, v.VariantName, entries)
		} else {
			value, err = h.games.ReplaceLineupWith(ctx, game.MatchID(v.MatchID), team.ID(v.TeamID), kind, number, v.VariantName, entries)
		}
		if err != nil {
			return nil, err
		}
		return lineupResult(value), nil
	case "lineup.delete":
		var v lineupDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		kind, err := parseLineupKind(v.Kind)
		if err != nil {
			return nil, err
		}
		number, err := requireInt(v.VariantNumber, "variant_number")
		if err != nil {
			return nil, err
		}
		if err := h.games.DeleteLineup(ctx, game.MatchID(v.MatchID), team.ID(v.TeamID), kind, uint16(number)); err != nil {
			return nil, err
		}
		return map[string]any{"match_id": v.MatchID, "team_id": v.TeamID, "deleted": true}, nil
	case "plate.create", "plate.update":
		var v plateArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		sequence, _ := requireInt(v.Sequence, "sequence")
		inning, _ := requireInt(v.Inning, "inning")
		order, _ := requireInt(v.BattingOrder, "batting_order")
		home, _ := requireInt(v.HomeScore, "home_score")
		away, _ := requireInt(v.AwayScore, "away_score")
		half, err := parseHalf(v.Half)
		if err != nil {
			return nil, err
		}
		kind, err := parsePlateType(v.PlateType)
		if err != nil {
			return nil, err
		}
		var value game.Plate
		if operation == "plate.create" {
			value, err = h.games.CreatePlateWith(ctx, game.PlateID(h.newID()), game.MatchID(v.MatchID), sequence, inning, half, order, player.ID(v.BatterID), player.ID(v.PitcherID), v.PitchSequence, kind, v.ResultDescription, parseRunners(v), home, away)
		} else {
			value, err = h.games.UpdatePlate(ctx, game.PlateID(v.PlateID), sequence, inning, half, order, player.ID(v.BatterID), player.ID(v.PitcherID), v.PitchSequence, kind, v.ResultDescription, parseRunners(v), home, away)
		}
		if err != nil {
			return nil, err
		}
		return plateResult(value), nil
	case "plate.delete":
		var v plateDeleteArguments
		if err := decodeArguments(arguments, &v); err != nil {
			return nil, err
		}
		if err := h.games.DeletePlate(ctx, game.PlateID(v.PlateID)); err != nil {
			return nil, err
		}
		return map[string]any{"plate_id": v.PlateID, "deleted": true}, nil
	default:
		return nil, fmt.Errorf("unknown team modify operation %q", operation)
	}
}
func matchResult(v game.Match) map[string]any {
	return map[string]any{"id": v.ID(), "home_team_id": v.HomeTeamID(), "away_team_id": v.AwayTeamID(), "scheduled_at": v.ScheduledAt().Format(time.RFC3339), "location": v.Location(), "status": matchStatusResult(v.Status()), "version": v.Version()}
}
func matchStatusResult(v game.MatchStatus) string {
	values := map[game.MatchStatus]string{game.MatchScheduled: "scheduled", game.MatchInProgress: "in_progress", game.MatchFinal: "final", game.MatchCancelled: "cancelled"}
	return values[v]
}
func lineupResult(v game.Lineup) map[string]any {
	return map[string]any{"match_id": v.MatchID(), "team_id": v.TeamID(), "kind": v.Kind(), "variant_number": v.VariantNumber(), "variant_name": v.VariantName(), "version": v.Version()}
}
func plateResult(v game.Plate) map[string]any {
	result := map[string]any{"id": v.ID(), "match_id": v.MatchID(), "sequence": v.Sequence(), "version": v.Version()}
	if score := v.Score(); score != nil {
		result["home_score"] = score.Home
		result["away_score"] = score.Away
	} else {
		result["home_score"] = nil
		result["away_score"] = nil
	}
	return result
}
