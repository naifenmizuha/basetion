package teamquery

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	lua "github.com/yuin/gopher-lua"
)

func newTeamModule(ctx context.Context, state *lua.LState, service *team.QueryService, arrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("list", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() > 1 {
			state.RaiseError("team.list accepts at most one filter table")
			return 0
		}
		activeOnly := false
		if state.GetTop() == 1 {
			value := state.CheckTable(1).RawGetString("active")
			if value != lua.LNil {
				flag, ok := value.(lua.LBool)
				if !ok {
					state.RaiseError("team.list active must be boolean")
					return 0
				}
				activeOnly = bool(flag)
			}
		}
		values, err := service.List(ctx, activeOnly)
		if err != nil {
			state.RaiseError("list teams: %v", err)
			return 0
		}
		result := state.NewTable()
		arrays[result] = struct{}{}
		for _, value := range values {
			entry := state.NewTable()
			entry.RawSetString("name", lua.LString(value.Name()))
			entry.RawSetString("active", lua.LBool(value.Active()))
			result.Append(entry)
		}
		state.Push(result)
		return 1
	}))
	return readOnlyProxy(state, "team", backing)
}

func newPlayerModule(ctx context.Context, state *lua.LState, service *player.QueryService, arrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("list", state.NewFunction(func(state *lua.LState) int {
		filter, err := playerFilterFromLua(state, "player.list")
		if err != nil {
			state.RaiseError("%v", err)
			return 0
		}
		values, err := service.List(ctx, filter)
		if err != nil {
			state.RaiseError("list players: %v", err)
			return 0
		}
		result := state.NewTable()
		arrays[result] = struct{}{}
		for _, value := range values {
			result.Append(playerViewToLua(state, value, arrays))
		}
		state.Push(result)
		return 1
	}))
	return readOnlyProxy(state, "player", backing)
}

func playerFilterFromLua(state *lua.LState, call string) (player.PlayerFilter, error) {
	if state.GetTop() > 1 {
		return player.PlayerFilter{}, fmt.Errorf("%s accepts at most one filter table", call)
	}
	if state.GetTop() == 0 {
		return player.PlayerFilter{}, nil
	}
	table := state.CheckTable(1)
	filter := player.PlayerFilter{}
	if value := table.RawGetString("team_name"); value != lua.LNil {
		text, ok := value.(lua.LString)
		if !ok {
			return filter, fmt.Errorf("%s team_name must be string", call)
		}
		filter.TeamName = string(text)
	}
	if value := table.RawGetString("jersey_number"); value != lua.LNil {
		number, ok := value.(lua.LNumber)
		if !ok || number != lua.LNumber(math.Trunc(float64(number))) || number < 0 || number > 99 {
			return filter, fmt.Errorf("%s jersey_number must be an integer from 0 to 99", call)
		}
		jersey := uint8(number)
		filter.JerseyNumber = &jersey
	}
	if value := table.RawGetString("position_any"); value != lua.LNil {
		positions, ok := value.(*lua.LTable)
		if !ok {
			return filter, fmt.Errorf("%s position_any must be an array", call)
		}
		var parseErr error
		positions.ForEach(func(_ lua.LValue, value lua.LValue) {
			if parseErr != nil {
				return
			}
			flag, exists := positionNames[string(lua.LVAsString(value))]
			if !exists {
				parseErr = fmt.Errorf("%s position_any contains unknown position %q", call, value.String())
				return
			}
			filter.PositionAny |= flag
		})
		if parseErr != nil {
			return filter, parseErr
		}
	}
	return filter, nil
}

func playerViewToLua(state *lua.LState, value player.PlayerView, arrays map[*lua.LTable]struct{}) *lua.LTable {
	entry := state.NewTable()
	entry.RawSetString("name", lua.LString(value.Name))
	entry.RawSetString("team_name", lua.LString(value.TeamName))
	entry.RawSetString("jersey_number", lua.LNumber(value.JerseyNumber))
	entry.RawSetString("positions", stringsToLua(state, positionFlagNames(value.Positions), arrays))
	entry.RawSetString("active", lua.LBool(value.Active))
	return entry
}

func newGameModule(ctx context.Context, state *lua.LState, service *game.QueryService, arrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("list", state.NewFunction(func(state *lua.LState) int {
		filter, err := matchFilterFromLua(state, "game.list")
		if err != nil {
			state.RaiseError("%v", err)
			return 0
		}
		values, err := service.ListMatches(ctx, filter)
		if err != nil {
			state.RaiseError("list matches: %v", err)
			return 0
		}
		result := state.NewTable()
		arrays[result] = struct{}{}
		for _, value := range values {
			result.Append(matchViewProjectionToLua(state, value))
		}
		state.Push(result)
		return 1
	}))
	backing.RawSetString("summaries", state.NewFunction(func(state *lua.LState) int {
		filter, err := matchFilterFromLua(state, "game.summaries")
		if err != nil {
			state.RaiseError("%v", err)
			return 0
		}
		values, err := service.SummarizeMatches(ctx, filter)
		if err != nil {
			state.RaiseError("summarize matches: %v", err)
			return 0
		}
		result := state.NewTable()
		arrays[result] = struct{}{}
		for _, value := range values {
			result.Append(matchSummaryToLua(state, value))
		}
		state.Push(result)
		return 1
	}))
	backing.RawSetString("records", state.NewFunction(gameRecordsFunction(ctx, state, service, arrays)))
	backing.RawSetString("lineups", state.NewFunction(gameLineupsFunction(ctx, state, service, arrays)))
	backing.RawSetString("performances", state.NewFunction(gamePerformancesFunction(ctx, state, service, arrays)))
	return readOnlyProxy(state, "game", backing)
}

func gameRecordsFunction(ctx context.Context, state *lua.LState, service *game.QueryService, arrays map[*lua.LTable]struct{}) lua.LGFunction {
	return func(state *lua.LState) int {
		filter, err := matchFilterFromLua(state, "game.records")
		if err != nil {
			state.RaiseError("%v", err)
			return 0
		}
		values, err := service.GetMatchRecords(ctx, filter)
		if err != nil {
			state.RaiseError("get match records: %v", err)
			return 0
		}
		out := state.NewTable()
		arrays[out] = struct{}{}
		for _, v := range values {
			entry := state.NewTable()
			entry.RawSetString("summary", matchSummaryToLua(state, v.Summary))
			events := state.NewTable()
			arrays[events] = struct{}{}
			for _, e := range v.Events {
				event := state.NewTable()
				event.RawSetString("sequence", lua.LNumber(e.Sequence))
				event.RawSetString("inning", lua.LNumber(e.Inning))
				event.RawSetString("half", lua.LString(halfName(e.Half)))
				event.RawSetString("batter", identityToLua(state, e.Batter))
				event.RawSetString("starting_pitcher", identityToLua(state, e.StartingPitcher))
				event.RawSetString("batting_result", lua.LNumber(e.BattingResult))
				event.RawSetString("result_description", lua.LString(e.ResultDescription))
				sit := state.NewTable()
				sit.RawSetString("outs", lua.LNumber(e.Situation.Outs))
				sit.RawSetString("home_score", lua.LNumber(e.Situation.HomeScore))
				sit.RawSetString("away_score", lua.LNumber(e.Situation.AwayScore))
				event.RawSetString("situation", sit)
				events.Append(event)
			}
			entry.RawSetString("events", events)
			out.Append(entry)
		}
		state.Push(out)
		return 1
	}
}
func gameLineupsFunction(ctx context.Context, state *lua.LState, service *game.QueryService, arrays map[*lua.LTable]struct{}) lua.LGFunction {
	return func(state *lua.LState) int {
		filter, err := matchFilterFromLua(state, "game.lineups")
		if err != nil {
			state.RaiseError("%v", err)
			return 0
		}
		values, err := service.ListMatchLineups(ctx, filter)
		if err != nil {
			state.RaiseError("list match lineups: %v", err)
			return 0
		}
		out := state.NewTable()
		arrays[out] = struct{}{}
		for _, v := range values {
			entry := state.NewTable()
			entry.RawSetString("match", matchViewProjectionToLua(state, v.Match))
			lineups := state.NewTable()
			arrays[lineups] = struct{}{}
			for _, l := range v.Lineups {
				x := state.NewTable()
				x.RawSetString("team_name", lua.LString(l.TeamName))
				x.RawSetString("kind", lua.LString(lineupKindName(l.Kind)))
				x.RawSetString("variant_number", lua.LNumber(l.VariantNumber))
				x.RawSetString("variant_name", lua.LString(l.VariantName))
				entries := state.NewTable()
				arrays[entries] = struct{}{}
				for _, e := range l.Entries {
					z := state.NewTable()
					z.RawSetString("player", identityToLua(state, e.Player))
					z.RawSetString("batting_order", lua.LNumber(e.BattingOrder))
					z.RawSetString("position", lua.LString(positionFlagNames(e.Position)[0]))
					entries.Append(z)
				}
				x.RawSetString("entries", entries)
				lineups.Append(x)
			}
			entry.RawSetString("lineups", lineups)
			out.Append(entry)
		}
		state.Push(out)
		return 1
	}
}
func gamePerformancesFunction(ctx context.Context, state *lua.LState, service *game.QueryService, arrays map[*lua.LTable]struct{}) lua.LGFunction {
	return func(state *lua.LState) int {
		filter, err := matchFilterFromLua(state, "game.performances")
		if err != nil {
			state.RaiseError("%v", err)
			return 0
		}
		values, err := service.AnalyzeMatchPlayers(ctx, filter)
		if err != nil {
			state.RaiseError("analyze match players: %v", err)
			return 0
		}
		out := state.NewTable()
		arrays[out] = struct{}{}
		for _, v := range values {
			x := state.NewTable()
			x.RawSetString("match", matchViewProjectionToLua(state, v.Match))
			x.RawSetString("offense", offenseToLua(state, v.Offense, arrays))
			x.RawSetString("pitching", pitchingToLua(state, v.Pitching, arrays))
			x.RawSetString("fielding", fieldingToLua(state, v.Fielding, arrays))
			limits := state.NewTable()
			limits.RawSetString("fielding_opportunities_unavailable", lua.LBool(v.Limits.FieldingOpportunitiesUnavailable))
			limits.RawSetString("earned_runs_require_explicit_mark", lua.LBool(v.Limits.EarnedRunsRequireExplicitMark))
			limits.RawSetString("unrecorded_pitch_facts_excluded", lua.LBool(v.Limits.UnrecordedPitchFactsExcluded))
			x.RawSetString("limits", limits)
			out.Append(x)
		}
		state.Push(out)
		return 1
	}
}
func identityToLua(s *lua.LState, v game.PlayerIdentityView) *lua.LTable {
	x := s.NewTable()
	x.RawSetString("name", lua.LString(v.Name))
	x.RawSetString("team_name", lua.LString(v.TeamName))
	x.RawSetString("jersey_number", lua.LNumber(v.JerseyNumber))
	return x
}
func offenseToLua(s *lua.LState, values []game.OffenseLineView, arrays map[*lua.LTable]struct{}) *lua.LTable {
	out := s.NewTable()
	arrays[out] = struct{}{}
	for _, v := range values {
		x := s.NewTable()
		x.RawSetString("player", identityToLua(s, v.Player))
		for _, p := range []struct {
			n string
			v uint
		}{{"pa", v.PA}, {"ab", v.AB}, {"h", v.H}, {"singles", v.Singles}, {"doubles", v.Doubles}, {"triples", v.Triples}, {"home_runs", v.HomeRuns}, {"walks", v.Walks}, {"hit_by_pitch", v.HitByPitch}, {"strikeouts", v.Strikeouts}, {"rbi", v.RBI}, {"runs", v.Runs}, {"total_bases", v.TotalBases}} {
			x.RawSetString(p.n, lua.LNumber(p.v))
		}
		setFloat(s, x, "avg", v.AVG)
		setFloat(s, x, "obp", v.OBP)
		setFloat(s, x, "slg", v.SLG)
		setFloat(s, x, "ops", v.OPS)
		out.Append(x)
	}
	return out
}
func pitchingToLua(s *lua.LState, values []game.PitchingLineView, arrays map[*lua.LTable]struct{}) *lua.LTable {
	out := s.NewTable()
	arrays[out] = struct{}{}
	for _, v := range values {
		x := s.NewTable()
		x.RawSetString("player", identityToLua(s, v.Player))
		for _, p := range []struct {
			n string
			v uint
		}{{"batters_faced", v.BattersFaced}, {"pitches", v.Pitches}, {"called_strikes", v.CalledStrikes}, {"swinging_strikes", v.SwingingStrikes}, {"hits", v.Hits}, {"home_runs", v.HomeRuns}, {"walks", v.Walks}, {"hit_by_pitch", v.HitByPitch}, {"strikeouts", v.Strikeouts}, {"runs", v.Runs}, {"earned_runs", v.EarnedRuns}, {"outs", v.Outs}} {
			x.RawSetString(p.n, lua.LNumber(p.v))
		}
		out.Append(x)
	}
	return out
}
func fieldingToLua(s *lua.LState, values []game.FieldingLineView, arrays map[*lua.LTable]struct{}) *lua.LTable {
	out := s.NewTable()
	arrays[out] = struct{}{}
	for _, v := range values {
		x := s.NewTable()
		x.RawSetString("player", identityToLua(s, v.Player))
		x.RawSetString("putouts", lua.LNumber(v.Putouts))
		x.RawSetString("assists", lua.LNumber(v.Assists))
		x.RawSetString("errors", lua.LNumber(v.Errors))
		out.Append(x)
	}
	return out
}
func setFloat(s *lua.LState, t *lua.LTable, n string, v *float64) {
	if v == nil {
		setNull(s, t, n)
	} else {
		t.RawSetString(n, lua.LNumber(*v))
	}
}

func halfName(value game.Half) string {
	if value == game.Top {
		return "top"
	}
	return "bottom"
}
func lineupKindName(value game.LineupKind) string {
	if value == game.LineupStarter {
		return "starter"
	}
	return "backup"
}

func matchFilterFromLua(state *lua.LState, call string) (game.MatchFilter, error) {
	if state.GetTop() > 1 {
		return game.MatchFilter{}, fmt.Errorf("%s accepts at most one filter table", call)
	}
	if state.GetTop() == 0 {
		return game.MatchFilter{}, nil
	}
	table := state.CheckTable(1)
	filter := game.MatchFilter{}
	if value := table.RawGetString("participant_names"); value != lua.LNil {
		values, ok := value.(*lua.LTable)
		if !ok {
			return filter, fmt.Errorf("%s participant_names must be an array", call)
		}
		var parseErr error
		values.ForEach(func(_ lua.LValue, value lua.LValue) {
			if parseErr != nil {
				return
			}
			text, ok := value.(lua.LString)
			if !ok {
				parseErr = fmt.Errorf("%s participant_names must contain strings", call)
				return
			}
			filter.ParticipantNames = append(filter.ParticipantNames, string(text))
		})
		if parseErr != nil {
			return filter, parseErr
		}
		if len(filter.ParticipantNames) > 2 {
			return filter, fmt.Errorf("%s participant_names must contain at most two strings", call)
		}
	}
	for _, definition := range []struct {
		name   string
		end    bool
		target **time.Time
	}{{"date_from", false, &filter.ScheduledFrom}, {"date_to", true, &filter.ScheduledTo}} {
		value := table.RawGetString(definition.name)
		if value == lua.LNil {
			continue
		}
		text, ok := value.(lua.LString)
		if !ok {
			return filter, fmt.Errorf("%s %s must be string", call, definition.name)
		}
		parsed, err := parseQueryTime(string(text), definition.end)
		if err != nil {
			return filter, fmt.Errorf("%s %s: %w", call, definition.name, err)
		}
		*definition.target = &parsed
	}
	if value := table.RawGetString("limit"); value != lua.LNil {
		number, ok := value.(lua.LNumber)
		if !ok || number != lua.LNumber(math.Trunc(float64(number))) || number < 0 {
			return filter, fmt.Errorf("%s limit must be a non-negative integer", call)
		}
		filter.Limit = int(number)
	}
	return filter, nil
}

func matchViewProjectionToLua(state *lua.LState, value game.MatchView) *lua.LTable {
	result := state.NewTable()
	result.RawSetString("scheduled_at", lua.LString(value.ScheduledAt.Format(time.RFC3339)))
	result.RawSetString("home_team_name", lua.LString(value.HomeTeamName))
	result.RawSetString("away_team_name", lua.LString(value.AwayTeamName))
	result.RawSetString("location", lua.LString(value.Location))
	result.RawSetString("status", lua.LString(matchStatusName(value.Status)))
	return result
}

func matchSummaryToLua(state *lua.LState, value game.MatchSummaryView) *lua.LTable {
	result := matchViewProjectionToLua(state, value.MatchView)
	if value.HomeScore == nil {
		setNull(state, result, "home_score")
	} else {
		result.RawSetString("home_score", lua.LNumber(*value.HomeScore))
	}
	if value.AwayScore == nil {
		setNull(state, result, "away_score")
	} else {
		result.RawSetString("away_score", lua.LNumber(*value.AwayScore))
	}
	results := map[game.Result]string{game.ResultPending: "pending", game.ResultHomeWin: "home_win", game.ResultAwayWin: "away_win", game.ResultDraw: "draw", game.ResultCancelled: "cancelled"}
	result.RawSetString("result", lua.LString(results[value.Result]))
	return result
}
