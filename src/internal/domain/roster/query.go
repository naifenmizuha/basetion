package roster

import (
	"context"
	"errors"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type PlayerFilter struct {
	TeamID      team.ID
	OnDate      *Date
	PositionAny player.PositionFlags
}
type Member struct {
	Membership Membership
	Player     player.Player
	Team       team.Team
}

func (m Member) ActiveOn(date Date) bool {
	return m.Team.Active() && m.Player.Active() && m.Membership.ActiveOn(date)
}

type QueryRepository interface {
	ListPlayers(context.Context, PlayerFilter) ([]Member, error)
}
type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("roster query repository is required")
	}
	return &QueryService{repository: repository}, nil
}
func (s *QueryService) ListPlayers(ctx context.Context, filter PlayerFilter) ([]Member, error) {
	if filter.TeamID == "" {
		return nil, errors.New("team id is required")
	}
	return s.repository.ListPlayers(ctx, filter)
}
