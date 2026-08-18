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
	plays   []game.Play
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
func (s *gameQueryStub) ListPlays(context.Context, game.MatchID) ([]game.Play, error) {
	return s.plays, nil
}

func TestGameProxyReadsWholePlay(t *testing.T) {
	now := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	match, _ := game.NewMatch("match-1", "home", "away", now, "field", game.MatchFinal, now)
	situation, _ := game.NewSituation(0, 0, 0, [3]*player.ID{})
	pitch, _ := game.NewPitch("pitch-1", 1, "pitcher", "batter", game.PitchInPlay, 0, 0, 0, 0, "", nil, nil, "in play", now)
	play, _ := game.NewPlay(game.PlayDraft{ID: "play-1", MatchID: "match-1", Sequence: 1, Inning: 1, Half: game.Top, BattingOrder: 1, BatterID: "batter", StartingPitcherID: "pitcher", Before: situation, After: situation, BattingResult: game.BattingGroundOut, ResultDescription: "out", Pitches: []game.Pitch{pitch}}, now)
	service, _ := game.NewQueryService(&gameQueryStub{match: match, plays: []game.Play{play}})
	executor, _ := NewLuaExecutor(DefaultLimits(), WithGameService(service))
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"game"}, Program: `function main(team) return {score=team.game.score({match_id="match-1"}),plays=team.game.plays({match_id="match-1"})} end`})
	if err != nil {
		t.Fatal(err)
	}
	value := result.(map[string]any)
	if len(value["plays"].([]any)) != 1 {
		t.Fatalf("result=%#v", result)
	}
}
