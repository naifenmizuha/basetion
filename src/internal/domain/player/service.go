package player

import (
	"context"
	"errors"
	"time"
)

type Repository interface {
	Create(context.Context, Player) error
	Get(context.Context, ID) (Player, error)
	Update(context.Context, Player, uint64) error
}

type Clock interface{ Now() time.Time }

type Service struct {
	repository Repository
	clock      Clock
}

func NewService(repository Repository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, errors.New("player repository is required")
	}
	if clock == nil {
		return nil, errors.New("player clock is required")
	}
	return &Service{repository: repository, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, id ID, name string, batting, throwing HandFlags, positions PositionFlags) (Player, error) {
	created, err := New(id, name, batting, throwing, positions, s.clock.Now())
	if err != nil {
		return Player{}, err
	}
	if err := s.repository.Create(ctx, created); err != nil {
		return Player{}, err
	}
	return created, nil
}

func (s *Service) Update(ctx context.Context, id ID, name string, batting, throwing HandFlags, positions PositionFlags) (Player, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Player{}, err
	}
	expected := current.Version()
	if err := current.UpdateProfile(name, batting, throwing, positions, s.clock.Now()); err != nil {
		return Player{}, err
	}
	if err := s.repository.Update(ctx, current, expected); err != nil {
		return Player{}, err
	}
	return current, nil
}

func (s *Service) SetActive(ctx context.Context, id ID, active bool) (Player, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Player{}, err
	}
	expected := current.Version()
	if err := current.SetActive(active, s.clock.Now()); err != nil {
		return Player{}, err
	}
	if err := s.repository.Update(ctx, current, expected); err != nil {
		return Player{}, err
	}
	return current, nil
}
