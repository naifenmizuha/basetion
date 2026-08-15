package teamops

import (
	"context"
	"testing"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamops"
)

type stubRosterReader struct{ filter domain.RosterPlayerFilter }

type emptyRosterReader struct{}

func (*emptyRosterReader) ListTeams(context.Context, bool) ([]domain.TeamView, error) {
	return nil, nil
}
func (*emptyRosterReader) ListPlayers(context.Context, domain.RosterPlayerFilter) ([]domain.RosterPlayerView, error) {
	return nil, nil
}

func (r *stubRosterReader) ListTeams(context.Context, bool) ([]domain.TeamView, error) {
	return []domain.TeamView{{ID: "team-1", Name: "Team", Active: true}}, nil
}
func (r *stubRosterReader) ListPlayers(_ context.Context, filter domain.RosterPlayerFilter) ([]domain.RosterPlayerView, error) {
	r.filter = filter
	return []domain.RosterPlayerView{{MembershipID: "membership-1", TeamID: "team-1", PlayerID: "player-1", Name: "Player", JerseyNumber: 18, BattingHands: []string{"left", "right"}, ThrowingHands: []string{"right"}, Positions: []string{"pitcher", "outfielder"}, JoinedAt: "2026-08-01", Active: true}}, nil
}

func TestLuaRosterModule(t *testing.T) {
	t.Parallel()
	reader := &stubRosterReader{}
	executor, err := NewLuaExecutor(DefaultLimits(), WithRosterReader(reader))
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"roster"}, Program: `function main(teamops) return teamops.roster.players({team_id="team-1",on_date="2026-08-15",position_any={"pitcher"}}) end`})
	if err != nil {
		t.Fatal(err)
	}
	players := result.([]any)
	if len(players) != 1 {
		t.Fatalf("players=%#v", players)
	}
	value := players[0].(map[string]any)
	if value["jersey_number"] != float64(18) || value["joined_at"] != "2026-08-01" || value["left_at"] != nil {
		t.Fatalf("player=%#v", value)
	}
	if reader.filter.TeamID != "team-1" || reader.filter.OnDate == nil || reader.filter.PositionAny != player.PositionPitcher {
		t.Fatalf("filter=%#v", reader.filter)
	}
}

func TestLuaRosterPreservesEmptyArrays(t *testing.T) {
	t.Parallel()
	executor, _ := NewLuaExecutor(DefaultLimits(), WithRosterReader(&emptyRosterReader{}))
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"roster"}, Program: `function main(teamops) return teamops.roster.teams({active=true}) end`})
	if err != nil {
		t.Fatal(err)
	}
	if values, ok := result.([]any); !ok || len(values) != 0 {
		t.Fatalf("teams result is not an empty array: %#v", result)
	}
}

func TestLuaRosterRequiresTeamScope(t *testing.T) {
	t.Parallel()
	executor, _ := NewLuaExecutor(DefaultLimits(), WithRosterReader(&stubRosterReader{}))
	_, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"roster"}, Program: `function main(teamops) return teamops.roster.players({}) end`})
	if err == nil {
		t.Fatal("missing team id accepted")
	}
}
