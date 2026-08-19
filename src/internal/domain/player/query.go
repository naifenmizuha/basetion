package player

import (
	"context"
	"errors"
)

type PlayerFilter struct {
	TeamName     string
	PositionAny  PositionFlags
	JerseyNumber *uint8
}

type PlayerView struct {
	Name         string
	TeamName     string
	JerseyNumber uint8
	Positions    PositionFlags
	Active       bool
}

type QueryRepository interface {
	List(context.Context, PlayerFilter) ([]PlayerView, error)
}
type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("player query repository is required")
	}
	return &QueryService{repository: repository}, nil
}
func (s *QueryService) List(ctx context.Context, filter PlayerFilter) ([]PlayerView, error) {
	if !filter.PositionAny.Valid() && filter.PositionAny != 0 {
		return nil, errors.New("invalid player position filter")
	}
	return s.repository.List(ctx, filter)
}
