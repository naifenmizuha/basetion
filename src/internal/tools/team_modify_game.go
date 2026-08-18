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
