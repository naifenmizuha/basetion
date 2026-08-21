package tools

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

const TeamFetchToolName = "team_fetch"

type teamFetchInput struct {
	Operation string         `json:"operation" jsonschema:"required,description=读取操作的叶子名称，不带任何前缀。可用值：game.summaries、game.records、game.lineups、game.performances、training.records。先通过 team_describe 查看参数说明，再把叶子名传给本字段。"`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"description=读取操作各自的筛选参数，通过 team_describe 获取"`
}

type teamFetchTopic struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Found      bool     `json:"found"`
	Summary    string   `json:"summary,omitempty"`
	Error      string   `json:"error,omitempty"`
	Children   []string `json:"children,omitempty"`
	Parameters []string `json:"parameters,omitempty"`
}

type teamFetchOutput struct {
	Operation string `json:"operation,omitempty"`
	Result    any    `json:"result,omitempty"`
}

// NewTeamFetch exposes server-owned composite read models. Its discovery
// protocol is provided by team_describe so this tool only executes reads.
func NewTeamFetch(games *game.QueryService, trainings *training.QueryService) (tool.InvokableTool, error) {
	if games == nil {
		return nil, errors.New("game query service is required")
	}
	if trainings == nil {
		return nil, errors.New("training query service is required")
	}
	return toolutils.InferTool(
		TeamFetchToolName,
		"执行预定义的组合读取。先通过 team_describe 获取比赛、自训记录等读取的参数说明；再用本工具取得结果。该工具不接受 Lua、SQL 或修改操作。需要自由组合球队、球员、比赛和原子 Play 时使用 team_query。",
		func(ctx context.Context, input teamFetchInput) (teamFetchOutput, error) {
			op := strings.TrimSpace(input.Operation)
			if !isFetchOperation(op) {
				return teamFetchOutput{}, fmt.Errorf("unknown team fetch operation %q; available: %s", op, strings.Join(fetchOperationNames(), ", "))
			}
			if op == "training.records" {
				filter, err := fetchTrainingFilter(input.Arguments)
				if err != nil {
					return teamFetchOutput{}, fmt.Errorf("validate team fetch arguments: %w", err)
				}
				values, err := trainings.ListViews(ctx, filter)
				if err != nil {
					return teamFetchOutput{}, fmt.Errorf("fetch %s: %w", op, err)
				}
				return teamFetchOutput{Operation: op, Result: fetchTrainingRecords(values)}, nil
			}
			filter, err := fetchMatchFilter(input.Arguments)
			if err != nil {
				return teamFetchOutput{}, fmt.Errorf("validate team fetch arguments: %w", err)
			}
			result, err := fetchGame(ctx, games, op, filter)
			if err != nil {
				return teamFetchOutput{}, fmt.Errorf("fetch %s: %w", op, err)
			}
			return teamFetchOutput{Operation: op, Result: result}, nil
		},
	)
}

func isFetchOperation(value string) bool {
	switch value {
	case "game.summaries", "game.records", "game.lineups", "game.performances", "training.records":
		return true
	default:
		return false
	}
}

func fetchOperationNames() []string {
	return []string{"game.summaries", "game.records", "game.lineups", "game.performances", "training.records"}
}

func describeFetchTopics(requested []string) []teamFetchTopic {
	all := map[string]teamFetchTopic{
		"game":              {Name: "game", Kind: "module", Found: true, Summary: "服务端计算的比赛组合读取。", Children: []string{"game.summaries", "game.records", "game.lineups", "game.performances"}},
		"game.summaries":    {Name: "game.summaries", Kind: "operation", Found: true, Summary: "读取比赛比分和赛果摘要。", Parameters: []string{"participant_names?: string[]", "date_from?: YYYY-MM-DD|RFC3339", "date_to?: YYYY-MM-DD|RFC3339", "limit?: 1..500"}},
		"game.records":      {Name: "game.records", Kind: "operation", Found: true, Summary: "读取比赛摘要及完整 Play 记录。", Parameters: []string{"participant_names?: string[]", "date_from?: YYYY-MM-DD|RFC3339", "date_to?: YYYY-MM-DD|RFC3339", "limit?: 1..500"}},
		"game.lineups":      {Name: "game.lineups", Kind: "operation", Found: true, Summary: "读取比赛阵容。", Parameters: []string{"participant_names?: string[]", "date_from?: YYYY-MM-DD|RFC3339", "date_to?: YYYY-MM-DD|RFC3339", "limit?: 1..500"}},
		"game.performances": {Name: "game.performances", Kind: "operation", Found: true, Summary: "按既有领域规则计算逐场球员表现。", Parameters: []string{"participant_names?: string[]", "date_from?: YYYY-MM-DD|RFC3339", "date_to?: YYYY-MM-DD|RFC3339", "limit?: 1..500"}},
		"training":          {Name: "training", Kind: "module", Found: true, Summary: "服务端组合的自训记录读取。", Children: []string{"training.records"}},
		"training.records":  {Name: "training.records", Kind: "operation", Found: true, Summary: "按球员姓名或日期范围读取自训记录；同名球员的记录全部返回。", Parameters: []string{"player_name?: string（精确匹配，同名全返回）", "date_from?: YYYY-MM-DD", "date_to?: YYYY-MM-DD", "limit?: 1..500"}},
	}
	if len(requested) == 0 {
		return []teamFetchTopic{all["game"], all["training"]}
	}
	result := make([]teamFetchTopic, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		if topic, exists := all[name]; exists {
			result = append(result, topic)
		} else {
			result = append(result, teamFetchTopic{Name: name, Found: false, Error: "unknown team fetch topic"})
		}
	}
	return result
}

func fetchMatchFilter(arguments map[string]any) (game.MatchFilter, error) {
	filter := game.MatchFilter{Limit: 100}
	if arguments == nil {
		return filter, nil
	}
	for key := range arguments {
		switch key {
		case "participant_names", "date_from", "date_to", "limit":
		default:
			return filter, fmt.Errorf("unknown argument %q", key)
		}
	}
	if raw, exists := arguments["participant_names"]; exists {
		values, ok := raw.([]any)
		if !ok {
			return filter, errors.New("participant_names must be an array")
		}
		if len(values) > 2 {
			return filter, errors.New("participant_names accepts at most two names")
		}
		for _, raw := range values {
			value, ok := raw.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return filter, errors.New("participant_names must contain non-empty strings")
			}
			filter.ParticipantNames = append(filter.ParticipantNames, value)
		}
	}
	for _, definition := range []struct {
		key string
		end bool
		set func(time.Time)
	}{
		{key: "date_from", set: func(value time.Time) { filter.ScheduledFrom = &value }},
		{key: "date_to", end: true, set: func(value time.Time) { filter.ScheduledTo = &value }},
	} {
		raw, exists := arguments[definition.key]
		if !exists {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return filter, fmt.Errorf("%s must be string", definition.key)
		}
		value, err := parseFetchTime(text, definition.end)
		if err != nil {
			return filter, fmt.Errorf("invalid %s: %w", definition.key, err)
		}
		definition.set(value)
	}
	if raw, exists := arguments["limit"]; exists {
		value, ok := raw.(float64)
		if !ok || value != math.Trunc(value) || value < 1 || value > 500 {
			return filter, errors.New("limit must be an integer from 1 to 500")
		}
		filter.Limit = int(value)
	}
	if filter.ScheduledFrom != nil && filter.ScheduledTo != nil && filter.ScheduledFrom.After(*filter.ScheduledTo) {
		return filter, errors.New("date range is invalid")
	}
	return filter, nil
}

func parseFetchTime(value string, end bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, err
	}
	if end {
		return parsed.Add(24*time.Hour - time.Nanosecond), nil
	}
	return parsed, nil
}

func fetchTrainingFilter(arguments map[string]any) (training.Filter, error) {
	filter := training.Filter{Limit: 100}
	if arguments == nil {
		return filter, nil
	}
	for key := range arguments {
		switch key {
		case "player_name", "date_from", "date_to", "limit":
		default:
			return filter, fmt.Errorf("unknown argument %q", key)
		}
	}
	if raw, exists := arguments["player_name"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return filter, errors.New("player_name must be a non-empty string")
		}
		filter.PlayerName = strings.TrimSpace(value)
	}
	for _, definition := range []struct {
		key string
		set func(training.Date)
	}{
		{key: "date_from", set: func(value training.Date) { filter.From = &value }},
		{key: "date_to", set: func(value training.Date) { filter.To = &value }},
	} {
		raw, exists := arguments[definition.key]
		if !exists {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return filter, fmt.Errorf("%s must be string", definition.key)
		}
		value, err := training.ParseDate(text)
		if err != nil {
			return filter, fmt.Errorf("invalid %s: %w", definition.key, err)
		}
		definition.set(value)
	}
	if raw, exists := arguments["limit"]; exists {
		value, ok := raw.(float64)
		if !ok || value != math.Trunc(value) || value < 1 || value > 500 {
			return filter, errors.New("limit must be an integer from 1 to 500")
		}
		filter.Limit = int(value)
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return filter, errors.New("date range is invalid")
	}
	return filter, nil
}

func fetchGame(ctx context.Context, service *game.QueryService, operation string, filter game.MatchFilter) (any, error) {
	switch operation {
	case "game.summaries":
		values, err := service.SummarizeMatches(ctx, filter)
		return fetchSummaries(values), err
	case "game.records":
		values, err := service.GetMatchRecords(ctx, filter)
		return fetchRecords(values), err
	case "game.lineups":
		values, err := service.ListMatchLineups(ctx, filter)
		return fetchLineups(values), err
	case "game.performances":
		values, err := service.AnalyzeMatchPlayers(ctx, filter)
		return fetchPerformances(values), err
	default:
		return nil, fmt.Errorf("unknown operation %q", operation)
	}
}

func fetchMatch(value game.MatchView) map[string]any {
	return map[string]any{"scheduled_at": value.ScheduledAt.Format(time.RFC3339), "home_team_name": value.HomeTeamName, "away_team_name": value.AwayTeamName, "location": value.Location, "status": fetchMatchStatus(value.Status)}
}
func fetchMatchStatus(value game.MatchStatus) string {
	return map[game.MatchStatus]string{game.MatchScheduled: "scheduled", game.MatchInProgress: "in_progress", game.MatchFinal: "final", game.MatchCancelled: "cancelled"}[value]
}
func fetchIdentity(value game.PlayerIdentityView) map[string]any {
	return map[string]any{"name": value.Name, "team_name": value.TeamName, "jersey_number": value.JerseyNumber}
}
func fetchSummaries(values []game.MatchSummaryView) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		item := fetchMatch(value.MatchView)
		item["home_score"], item["away_score"] = value.HomeScore, value.AwayScore
		item["result"] = map[game.Result]string{game.ResultPending: "pending", game.ResultHomeWin: "home_win", game.ResultAwayWin: "away_win", game.ResultDraw: "draw", game.ResultCancelled: "cancelled"}[value.Result]
		result = append(result, item)
	}
	return result
}
func fetchRecords(values []game.MatchRecordView) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		events := make([]any, 0, len(value.Events))
		for _, event := range value.Events {
			events = append(events, fetchPlay(event))
		}
		result = append(result, map[string]any{"summary": fetchSummaries([]game.MatchSummaryView{value.Summary})[0], "events": events})
	}
	return result
}
func fetchPlay(value game.PlayEventView) map[string]any {
	return map[string]any{"sequence": value.Sequence, "inning": value.Inning, "half": map[game.Half]string{game.Top: "top", game.Bottom: "bottom"}[value.Half], "batting_order": value.BattingOrder, "batter": fetchIdentity(value.Batter), "starting_pitcher": fetchIdentity(value.StartingPitcher), "situation": map[string]any{"outs": value.Situation.Outs, "home_score": value.Situation.HomeScore, "away_score": value.Situation.AwayScore}, "batting_result": value.BattingResult, "result_description": value.ResultDescription}
}
func fetchLineups(values []game.MatchLineupsView) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		lineups := make([]any, 0, len(value.Lineups))
		for _, lineup := range value.Lineups {
			entries := make([]any, 0, len(lineup.Entries))
			for _, entry := range lineup.Entries {
				entries = append(entries, map[string]any{"player": fetchIdentity(entry.Player), "batting_order": entry.BattingOrder, "position": positionName(entry.Position)})
			}
			kind := "starter"
			if lineup.Kind == game.LineupBackup {
				kind = "backup"
			}
			lineups = append(lineups, map[string]any{"team_name": lineup.TeamName, "kind": kind, "variant_number": lineup.VariantNumber, "variant_name": lineup.VariantName, "entries": entries})
		}
		result = append(result, map[string]any{"match": fetchMatch(value.Match), "lineups": lineups})
	}
	return result
}
func fetchPerformances(values []game.MatchPlayerPerformanceView) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		offense, pitching, fielding := make([]any, 0, len(value.Offense)), make([]any, 0, len(value.Pitching)), make([]any, 0, len(value.Fielding))
		for _, line := range value.Offense {
			offense = append(offense, map[string]any{"player": fetchIdentity(line.Player), "pa": line.PA, "ab": line.AB, "h": line.H, "home_runs": line.HomeRuns, "rbi": line.RBI, "runs": line.Runs, "avg": line.AVG, "obp": line.OBP, "slg": line.SLG, "ops": line.OPS})
		}
		for _, line := range value.Pitching {
			pitching = append(pitching, map[string]any{"player": fetchIdentity(line.Player), "batters_faced": line.BattersFaced, "pitches": line.Pitches, "called_strikes": line.CalledStrikes, "swinging_strikes": line.SwingingStrikes, "hits": line.Hits, "home_runs": line.HomeRuns, "walks": line.Walks, "hit_by_pitch": line.HitByPitch, "strikeouts": line.Strikeouts, "runs": line.Runs, "earned_runs": line.EarnedRuns, "outs": line.Outs})
		}
		for _, line := range value.Fielding {
			fielding = append(fielding, map[string]any{"player": fetchIdentity(line.Player), "putouts": line.Putouts, "assists": line.Assists, "errors": line.Errors})
		}
		result = append(result, map[string]any{"match": fetchMatch(value.Match), "offense": offense, "pitching": pitching, "fielding": fielding, "limits": map[string]any{"fielding_opportunities_unavailable": value.Limits.FieldingOpportunitiesUnavailable, "earned_runs_require_explicit_mark": value.Limits.EarnedRunsRequireExplicitMark, "unrecorded_pitch_facts_excluded": value.Limits.UnrecordedPitchFactsExcluded}})
	}
	return result
}
func fetchTrainingRecords(values []training.RecordView) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"player_name": value.PlayerName, "team_name": value.TeamName, "training_date": value.TrainingDate.String(), "content": value.Content, "reflection": value.Reflection})
	}
	return result
}
func positionName(value player.PositionFlags) string {
	for _, candidate := range []struct {
		name string
		flag player.PositionFlags
	}{{"pitcher", player.PositionPitcher}, {"catcher", player.PositionCatcher}, {"first_base", player.PositionFirstBase}, {"second_base", player.PositionSecondBase}, {"shortstop", player.PositionShortstop}, {"third_base", player.PositionThirdBase}, {"outfielder", player.PositionOutfielder}} {
		if value.HasAny(candidate.flag) {
			return candidate.name
		}
	}
	return ""
}
