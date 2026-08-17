package teamquery

import (
	"context"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
)

type gameQueryStub struct {
	match   game.Match
	plates  []game.Plate
	lineups []game.Lineup
}

func (s *gameQueryStub) ListMatches(context.Context) ([]game.Match, error) {
	return []game.Match{s.match}, nil
}
func (s *gameQueryStub) GetMatch(context.Context, game.MatchID) (game.Match, error) {
	return s.match, nil
}
func (s *gameQueryStub) ListLineups(context.Context, game.MatchID) ([]game.Lineup, error) {
	return s.lineups, nil
}
func (s *gameQueryStub) ListPlates(context.Context, game.MatchID) ([]game.Plate, error) {
	return s.plates, nil
}

func TestLuaGameAndLineupModules(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 18, 19, 0, 0, 0, time.UTC)
	match, _ := game.NewMatch("match-1", "home", "away", now, "Field", game.MatchFinal, now)
	plate, _ := game.NewPlate("plate-1", "match-1", 1, 1, game.Top, 1, "batter", "pitcher", "S", game.PlateHomeRun, "home run", [3]*player.ID{}, 0, 1, now)
	entry, _ := game.NewLineupEntry("entry-1", "batter", 1, player.PositionPitcher)
	lineup, _ := game.NewLineup("match-1", "away", game.LineupStarter, 0, "Starter", []game.LineupEntry{entry}, now)
	service, _ := game.NewQueryService(&gameQueryStub{match: match, plates: []game.Plate{plate}, lineups: []game.Lineup{lineup}})
	executor, err := NewLuaExecutor(DefaultLimits(), WithGameService(service))
	if err != nil {
		t.Fatal(err)
	}
	if got := executor.AvailableModules(); len(got) != 2 || got[0] != "game" || got[1] != "lineup" {
		t.Fatalf("modules=%v", got)
	}
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"game", "lineup"}, Program: `function main(team) return {score=team.game.score({match_id="match-1"}), plates=team.game.plates({match_id="match-1"}), lineups=team.lineup.list({match_id="match-1"})} end`})
	if err != nil {
		t.Fatal(err)
	}
	value := result.(map[string]any)
	score := value["score"].(map[string]any)
	if score["known"] != true || score["final"] != true || score["away_score"] != float64(1) {
		t.Fatalf("score=%#v", score)
	}
	if len(value["plates"].([]any)) != 1 || len(value["lineups"].([]any)) != 1 {
		t.Fatalf("result=%#v", value)
	}
}

func TestLuaGameUnknownScore(t *testing.T) {
	t.Parallel()
	now := time.Now()
	match, _ := game.NewMatch("match-1", "home", "away", now, "Field", game.MatchInProgress, now)
	legacy, _ := game.RestorePlate("plate-1", "match-1", 1, 1, game.Top, 1, "batter", "pitcher", "", game.PlateOut, "legacy", [3]*player.ID{}, nil, 1, now, now, nil)
	service, _ := game.NewQueryService(&gameQueryStub{match: match, plates: []game.Plate{legacy}})
	executor, _ := NewLuaExecutor(DefaultLimits(), WithGameService(service))
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"game"}, Program: `function main(team) return team.game.score({match_id="match-1"}) end`})
	if err != nil {
		t.Fatal(err)
	}
	score := result.(map[string]any)
	if score["known"] != false || score["home_score"] != nil || score["final"] != false {
		t.Fatalf("score=%#v", score)
	}
}
