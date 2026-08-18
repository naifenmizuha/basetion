package game

import (
	"context"
	"errors"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type MatchFilter struct {
	TeamID   team.ID
	From, To *time.Time
	Status   *MatchStatus
}

type QueryRepository interface {
	ListMatches(context.Context) ([]Match, error)
	GetMatch(context.Context, MatchID) (Match, error)
	ListLineups(context.Context, MatchID) ([]Lineup, error)
	ListPlays(context.Context, MatchID) ([]Play, error)
}

type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("game query repository is required")
	}
	return &QueryService{repository: repository}, nil
}

func (s *QueryService) ListMatches(ctx context.Context, filter MatchFilter) ([]Match, error) {
	if filter.Status != nil && !filter.Status.Valid() {
		return nil, errors.New("invalid match status filter")
	}
	values, err := s.repository.ListMatches(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Match, 0, len(values))
	for _, v := range values {
		if filter.TeamID != "" && v.HomeTeamID() != filter.TeamID && v.AwayTeamID() != filter.TeamID {
			continue
		}
		if filter.From != nil && v.ScheduledAt().Before(*filter.From) {
			continue
		}
		if filter.To != nil && v.ScheduledAt().After(*filter.To) {
			continue
		}
		if filter.Status != nil && v.Status() != *filter.Status {
			continue
		}
		result = append(result, v)
	}
	return result, nil
}
func (s *QueryService) GetMatch(ctx context.Context, id MatchID) (Match, error) {
	return s.repository.GetMatch(ctx, id)
}
func (s *QueryService) GetDetail(ctx context.Context, id MatchID) (Detail, error) {
	record, err := s.GetGameRecord(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Match: record.Match, Lineups: record.Lineups, Plays: record.Plays}, nil
}

func (s *QueryService) GetGameRecord(ctx context.Context, id MatchID) (GameRecord, error) {
	m, err := s.repository.GetMatch(ctx, id)
	if err != nil {
		return GameRecord{}, err
	}
	ls, err := s.repository.ListLineups(ctx, id)
	if err != nil {
		return GameRecord{}, err
	}
	ps, err := s.repository.ListPlays(ctx, id)
	if err != nil {
		return GameRecord{}, err
	}
	return GameRecord{Match: m, Lineups: ls, Plays: ps}, nil
}
func (s *QueryService) ListLineups(ctx context.Context, id MatchID, teamID team.ID) ([]Lineup, error) {
	values, err := s.repository.ListLineups(ctx, id)
	if err != nil {
		return nil, err
	}
	if teamID == "" {
		return values, nil
	}
	result := make([]Lineup, 0, len(values))
	for _, v := range values {
		if v.TeamID() == teamID {
			result = append(result, v)
		}
	}
	return result, nil
}
func (s *QueryService) ListPlays(ctx context.Context, id MatchID) ([]Play, error) {
	return s.repository.ListPlays(ctx, id)
}

type ScoreSnapshot struct {
	Score *Score
	Final bool
}

func (s *QueryService) CurrentScore(ctx context.Context, id MatchID) (ScoreSnapshot, error) {
	m, err := s.repository.GetMatch(ctx, id)
	if err != nil {
		return ScoreSnapshot{}, err
	}
	plays, err := s.repository.ListPlays(ctx, id)
	if err != nil {
		return ScoreSnapshot{}, err
	}
	var score *Score
	var latest uint32
	for _, play := range plays {
		if play.Sequence() >= latest {
			after := play.After()
			latest, score = play.Sequence(), &Score{Home: after.HomeScore, Away: after.AwayScore}
		}
	}
	return ScoreSnapshot{Score: score, Final: m.Status() == MatchFinal}, nil
}
