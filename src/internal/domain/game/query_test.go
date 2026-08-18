package game

import (
	"context"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type queryRepositoryStub struct {
	filter    MatchFilter
	matches   []Match
	summaries []MatchSummary
}

func (s *queryRepositoryStub) ListMatches(_ context.Context, filter MatchFilter) ([]Match, error) {
	s.filter = filter
	return s.matches, nil
}
func (s *queryRepositoryStub) ListMatchSummaries(_ context.Context, filter MatchFilter) ([]MatchSummary, error) {
	s.filter = filter
	return s.summaries, nil
}
func (s *queryRepositoryStub) FindMatchSummary(_ context.Context, filter MatchFilter, _ MatchSelection) (MatchSummary, error) {
	s.filter = filter
	if len(s.summaries) > 0 {
		return s.summaries[0], nil
	}
	return MatchSummary{}, ErrMatchNotFound
}
func (*queryRepositoryStub) GetMatch(context.Context, MatchID) (Match, error)       { return Match{}, nil }
func (*queryRepositoryStub) ListLineups(context.Context, MatchID) ([]Lineup, error) { return nil, nil }
func (*queryRepositoryStub) ListPlays(context.Context, MatchID) ([]Play, error)     { return nil, nil }

func TestQueryServicePassesMatchFilterToRepository(t *testing.T) {
	t.Parallel()
	repository := &queryRepositoryStub{}
	service, err := NewQueryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	status := MatchFinal
	if _, err := service.ListMatches(context.Background(), MatchFilter{TeamName: " 蜀汉队 ", From: &from, Status: &status}); err != nil {
		t.Fatal(err)
	}
	if repository.filter.TeamName != "蜀汉队" || repository.filter.From != &from || repository.filter.Status != &status {
		t.Fatalf("filter=%#v", repository.filter)
	}
}

func TestQueryServiceRejectsInvalidMatchFilter(t *testing.T) {
	t.Parallel()
	service, _ := NewQueryService(&queryRepositoryStub{})
	if _, err := service.ListMatches(context.Background(), MatchFilter{TeamID: team.ID("team-1"), TeamName: "Team"}); err == nil {
		t.Fatal("team ID and name accepted together")
	}
	from := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	if _, err := service.ListMatchSummaries(context.Background(), MatchFilter{From: &from, To: &to}); err == nil {
		t.Fatal("reversed range accepted")
	}
}
