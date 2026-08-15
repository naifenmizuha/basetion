package team

import (
	"context"
	"errors"
	"time"
)

type Repository interface {
	Create(context.Context, Team) error
}

type Clock interface{ Now() time.Time }

type Service struct {
	repository Repository
	clock      Clock
}

func NewService(repository Repository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, errors.New("team repository is required")
	}
	if clock == nil {
		return nil, errors.New("team clock is required")
	}
	return &Service{repository: repository, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, id ID, name string) (Team, error) {
	value, err := New(id, name, s.clock.Now())
	if err != nil {
		return Team{}, err
	}
	if err := s.repository.Create(ctx, value); err != nil {
		return Team{}, err
	}
	return value, nil
}
