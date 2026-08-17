package team

import (
	"context"
	"errors"
)

type QueryRepository interface {
	List(context.Context, bool) ([]Team, error)
}
type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("team query repository is required")
	}
	return &QueryService{repository: repository}, nil
}
func (s *QueryService) List(ctx context.Context, activeOnly bool) ([]Team, error) {
	return s.repository.List(ctx, activeOnly)
}
