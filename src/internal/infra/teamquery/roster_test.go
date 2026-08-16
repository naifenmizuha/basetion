package teamquery

import (
	"context"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
)

type stubTeamQueryRepository struct{ empty bool }

func (r *stubTeamQueryRepository) List(context.Context, bool) ([]team.Team, error) {
	if r.empty {
		return nil, nil
	}
	now := time.Now()
	value, _ := team.New("team-1", "Team", now)
	return []team.Team{value}, nil
}

type stubRosterQueryRepository struct {
	filter roster.PlayerFilter
	empty  bool
}

func (r *stubRosterQueryRepository) ListPlayers(_ context.Context, filter roster.PlayerFilter) ([]roster.Member, error) {
	r.filter = filter
	if r.empty {
		return nil, nil
	}
	now := time.Now()
	joined, _ := roster.ParseDate("2026-08-01")
	membership, _ := roster.New("membership-1", "team-1", "player-1", 18, joined, now)
	currentTeam, _ := team.New("team-1", "Team", now)
	currentPlayer, _ := player.New("player-1", "Player", player.HandLeft|player.HandRight, player.HandRight, player.PositionPitcher|player.PositionOutfielder, now)
	return []roster.Member{{Membership: membership, Team: currentTeam, Player: currentPlayer}}, nil
}
func rosterExecutor(t *testing.T, empty bool) (*LuaExecutor, *stubRosterQueryRepository) {
	t.Helper()
	teams, _ := team.NewQueryService(&stubTeamQueryRepository{empty: empty})
	repo := &stubRosterQueryRepository{empty: empty}
	rosters, _ := roster.NewQueryService(repo)
	executor, err := NewLuaExecutor(DefaultLimits(), WithRosterServices(teams, rosters))
	if err != nil {
		t.Fatal(err)
	}
	return executor, repo
}

func TestLuaRosterModule(t *testing.T) {
	t.Parallel()
	executor, reader := rosterExecutor(t, false)
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"roster"}, Program: `function main(team) return team.roster.players({team_id="team-1",on_date="2026-08-15",position_any={"pitcher"}}) end`})
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
	executor, _ := rosterExecutor(t, true)
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"roster"}, Program: `function main(team) return team.roster.teams({active=true}) end`})
	if err != nil {
		t.Fatal(err)
	}
	if values, ok := result.([]any); !ok || len(values) != 0 {
		t.Fatalf("teams result is not an empty array: %#v", result)
	}
}
func TestLuaRosterRequiresTeamScope(t *testing.T) {
	t.Parallel()
	executor, _ := rosterExecutor(t, false)
	_, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"roster"}, Program: `function main(team) return team.roster.players({}) end`})
	if err == nil {
		t.Fatal("missing team id accepted")
	}
}
