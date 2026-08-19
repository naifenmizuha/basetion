package teamquery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
)

type dynamicGameRepository struct{ matchCalls, playCalls int }

func (r *dynamicGameRepository) ListMatches(_ context.Context, _ game.MatchFilter) ([]game.MatchView, error) {
	r.matchCalls++
	return []game.MatchView{{ID: "match-a", ScheduledAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), HomeTeamName: "蜀汉队", AwayTeamName: "魏国队", Location: "主场", Status: game.MatchFinal}}, nil
}
func (r *dynamicGameRepository) ListPlays(_ context.Context, ids []game.MatchID) ([]game.PlayEventView, error) {
	r.playCalls++
	if len(ids) != 1 || ids[0] != "match-a" {
		return nil, context.Canceled
	}
	return []game.PlayEventView{{Sequence: 1, Inning: 1, Half: game.Top, Batter: game.PlayerIdentityView{Name: "甲", TeamName: "魏国队", JerseyNumber: 1}, StartingPitcher: game.PlayerIdentityView{Name: "乙", TeamName: "蜀汉队", JerseyNumber: 18}, Situation: game.SituationView{Outs: 1, HomeScore: 2, AwayScore: 1}, BattingResult: game.BattingSingle, ResultDescription: "安打"}}, nil
}
func (*dynamicGameRepository) SummarizeMatches(context.Context, game.MatchFilter) ([]game.MatchSummaryView, error) {
	return nil, nil
}
func (*dynamicGameRepository) GetMatchRecords(context.Context, game.MatchFilter) ([]game.MatchRecordView, error) {
	return nil, nil
}
func (*dynamicGameRepository) ListMatchLineups(context.Context, game.MatchFilter) ([]game.MatchLineupsView, error) {
	return nil, nil
}
func (*dynamicGameRepository) AnalyzeMatchPlayers(context.Context, game.MatchFilter) ([]game.MatchPlayerPerformanceView, error) {
	return nil, nil
}

func testExecutor(t *testing.T) *LuaExecutor {
	t.Helper()
	limits := DefaultLimits()
	limits.Timeout = 250 * time.Millisecond
	executor, err := NewLuaExecutor(limits)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func execute(executor *LuaExecutor, ctx context.Context, program string) (any, error) {
	return executor.Execute(ctx, domain.Query{Program: program})
}

func TestLuaExecutorAggregatesAndConvertsResult(t *testing.T) {
	t.Parallel()
	executor := testExecutor(t)
	result, err := execute(executor, context.Background(), `
function main(team)
    local values = {4, 1, 3, 2}
    table.sort(values)
    local sum = 0
    for _, value in ipairs(values) do
        sum = sum + value
    end
    return {
        sum = sum,
        maximum = math.max(unpack(values)),
        label = string.upper("hits"),
        values = values,
        empty_array = team.array(),
        null_value = team.null,
    }
end`)
	if err != nil {
		t.Fatal(err)
	}
	object := result.(map[string]any)
	if object["sum"] != float64(10) || object["maximum"] != float64(4) || object["label"] != "HITS" {
		t.Fatalf("unexpected aggregation: %#v", object)
	}
	values := object["values"].([]any)
	if len(values) != 4 || values[0] != float64(1) || values[3] != float64(4) {
		t.Fatalf("unexpected values: %#v", values)
	}
	if len(object["empty_array"].([]any)) != 0 || object["null_value"] != nil {
		t.Fatalf("unexpected special values: %#v", object)
	}
}

func TestLuaExecutorSandbox(t *testing.T) {
	t.Parallel()
	executor := testExecutor(t)
	result, err := execute(executor, context.Background(), `
function main(team)
    return {
        os = type(os), io = type(io), require = type(require),
        load = type(load), loadfile = type(loadfile), debug = type(debug),
        coroutine = type(coroutine), random = type(math.random),
        setmetatable = type(setmetatable),
    }
end`)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range result.(map[string]any) {
		if value != "nil" {
			t.Fatalf("sandbox global %s has type %v", name, value)
		}
	}
	if _, err := execute(executor, context.Background(), `function main(team) team.array = 1 end`); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("team mutation error = %v", err)
	}
	if _, err := execute(executor, context.Background(), `function main(team) table.insert(team, 1) end`); err == nil || !strings.Contains(err.Error(), "table expected") {
		t.Fatalf("team raw table mutation error = %v", err)
	}
	if _, err := execute(executor, context.Background(), `function main(team) return team.array({}) end`); err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("team.array argument error = %v", err)
	}
}

func TestLuaExecutorHonorsTimeout(t *testing.T) {
	t.Parallel()
	limits := DefaultLimits()
	limits.Timeout = 20 * time.Millisecond
	executor, _ := NewLuaExecutor(limits)
	started := time.Now()
	_, err := execute(executor, context.Background(), `function main(team) while true do end end`)
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("timeout error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(started))
	}
}

func TestLuaExecutorRejectsInvalidResults(t *testing.T) {
	t.Parallel()
	executor := testExecutor(t)
	tests := []struct {
		name    string
		program string
		want    string
	}{
		{name: "missing main", program: `return {}`, want: "must define main"},
		{name: "cycle", program: `function main() local t = {}; t.self = t; return t end`, want: "cycle"},
		{name: "sparse", program: `function main() return {[1] = "a", [3] = "c"} end`, want: "sparse"},
		{name: "mixed", program: `function main() return {[1] = "a", name = "x"} end`, want: "mixes"},
		{name: "function", program: `function main() return {value = function() end} end`, want: "unsupported function"},
		{name: "nan", program: `function main() return 0 / 0 end`, want: "non-finite"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := execute(executor, context.Background(), test.program)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLuaExecutorLimits(t *testing.T) {
	t.Parallel()
	limits := DefaultLimits()
	limits.Timeout = 250 * time.Millisecond
	limits.MaxElements = 2
	limits.MaxResultBytes = 12
	executor, _ := NewLuaExecutor(limits)
	if _, err := execute(executor, context.Background(), `function main() return {1, 2, 3} end`); err == nil || !strings.Contains(err.Error(), "elements") {
		t.Fatalf("element limit error = %v", err)
	}
	if _, err := execute(executor, context.Background(), `function main() return "a result larger than twelve bytes" end`); err == nil || !strings.Contains(err.Error(), "result exceeds") {
		t.Fatalf("result limit error = %v", err)
	}
}

func TestLuaExecutorReadsPlaysFromRuntimeMatchReferences(t *testing.T) {
	t.Parallel()
	games, err := game.NewQueryService(&dynamicGameRepository{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewLuaExecutor(DefaultLimits(), WithGameService(games))
	if err != nil {
		t.Fatal(err)
	}
	result, err := execute(executor, context.Background(), `
function main(data)
  local matches = data.game.list({limit = 5})
  local selected = data.array()
  table.insert(selected, matches[1])
  local plays = data.game.plays({matches = selected})
  return {match = matches[1], plays = plays}
end`)
	if err != nil {
		t.Fatal(err)
	}
	encoded := result.(map[string]any)
	if _, exists := encoded["match"].(map[string]any)["id"]; exists {
		t.Fatalf("internal match id leaked: %#v", encoded)
	}
	plays := encoded["plays"].([]any)
	if len(plays) != 1 || plays[0].(map[string]any)["result_description"] != "安打" {
		t.Fatalf("unexpected plays: %#v", plays)
	}
	if _, err := execute(executor, context.Background(), `function main(data) return data.game.plays({matches = {{}}}) end`); err == nil || !strings.Contains(err.Error(), "unrecognized match reference") {
		t.Fatalf("forged reference error = %v", err)
	}
}

func TestLuaExecutorLimitsRuntimeReads(t *testing.T) {
	t.Parallel()
	limits := DefaultLimits()
	limits.MaxDataCalls = 1
	games, _ := game.NewQueryService(&dynamicGameRepository{})
	executor, _ := NewLuaExecutor(limits, WithGameService(games))
	_, err := execute(executor, context.Background(), `function main(data) local m=data.game.list({limit=1}); return data.game.plays({matches=m}) end`)
	if err == nil || !strings.Contains(err.Error(), "source reads exceed") {
		t.Fatalf("runtime read limit error = %v", err)
	}
}

func TestLuaExecutorMemoizesIdenticalRuntimeReads(t *testing.T) {
	t.Parallel()
	limits := DefaultLimits()
	limits.MaxDataCalls = 2
	repository := &dynamicGameRepository{}
	games, _ := game.NewQueryService(repository)
	executor, _ := NewLuaExecutor(limits, WithGameService(games))
	_, err := execute(executor, context.Background(), `
function main(data)
  local first = data.game.list({limit=1})
  local second = data.game.list({limit=1})
  local a = data.game.plays({matches=first})
  local b = data.game.plays({matches=second})
  return {count = #a + #b}
end`)
	if err != nil {
		t.Fatal(err)
	}
	if repository.matchCalls != 1 || repository.playCalls != 1 {
		t.Fatalf("runtime reads were not memoized: matches=%d plays=%d", repository.matchCalls, repository.playCalls)
	}
}
