package roster

import (
	"context"
	"testing"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type queryRepositoryStub struct{ filter PlayerFilter }

func (s *queryRepositoryStub) ListPlayers(_ context.Context, filter PlayerFilter) ([]Member, error) {
	s.filter = filter
	return nil, nil
}

func TestQueryServiceAcceptsNameFilters(t *testing.T) {
	t.Parallel()
	repository := &queryRepositoryStub{}
	service, err := NewQueryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListPlayers(context.Background(), PlayerFilter{TeamName: " 蜀汉队 ", PlayerName: " 刘备 "}); err != nil {
		t.Fatal(err)
	}
	if repository.filter.TeamName != "蜀汉队" || repository.filter.PlayerName != "刘备" {
		t.Fatalf("filter=%#v", repository.filter)
	}
}

func TestQueryServiceRejectsUnscopedOrConflictingFilters(t *testing.T) {
	t.Parallel()
	service, _ := NewQueryService(&queryRepositoryStub{})
	if _, err := service.ListPlayers(context.Background(), PlayerFilter{}); err == nil {
		t.Fatal("unscoped filter accepted")
	}
	if _, err := service.ListPlayers(context.Background(), PlayerFilter{TeamID: team.ID("team-1"), TeamName: "Team"}); err == nil {
		t.Fatal("team ID and name accepted together")
	}
}
