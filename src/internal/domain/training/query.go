package training

import (
	"context"
	"errors"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type Filter struct {
	PlayerID player.ID
	From     *Date
	To       *Date
}

type QueryRepository interface {
	List(context.Context, Filter) ([]Record, error)
}

type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("training query repository is required")
	}
	return &QueryService{repository: repository}, nil
}

func (s *QueryService) List(ctx context.Context, filter Filter) ([]Record, error) {
	if filter.PlayerID == "" {
		return nil, errors.New("player id is required")
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return nil, errors.New("training date range is reversed")
	}
	return s.repository.List(ctx, filter)
}
