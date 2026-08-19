package training

import (
	"context"
	"errors"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type Repository interface {
	Create(context.Context, Record) error
	Get(context.Context, ID) (Record, error)
	Update(context.Context, Record) error
}

type PlayerRepository interface {
	Get(context.Context, player.ID) (player.Player, error)
}

type Clock interface{ Now() time.Time }

type Service struct {
	repository Repository
	players    PlayerRepository
	clock      Clock
}

func NewService(repository Repository, players PlayerRepository, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, errors.New("training repository is required")
	}
	if players == nil {
		return nil, errors.New("training player repository is required")
	}
	if clock == nil {
		return nil, errors.New("training clock is required")
	}
	return &Service{repository: repository, players: players, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, id ID, playerID player.ID, date Date, content, reflection string) (Record, error) {
	if _, err := s.players.Get(ctx, playerID); err != nil {
		return Record{}, err
	}
	value, err := New(id, playerID, date, content, reflection, s.clock.Now())
	if err != nil {
		return Record{}, err
	}
	if err := s.repository.Create(ctx, value); err != nil {
		return Record{}, err
	}
	return value, nil
}

func (s *Service) Update(ctx context.Context, id ID, content, reflection string) (Record, error) {
	value, err := s.repository.Get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if err := value.Update(content, reflection, s.clock.Now()); err != nil {
		return Record{}, err
	}
	if err := s.repository.Update(ctx, value); err != nil {
		return Record{}, err
	}
	return value, nil
}

func (s *Service) Delete(ctx context.Context, id ID) error {
	value, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := value.Delete(s.clock.Now()); err != nil {
		return err
	}
	return s.repository.Update(ctx, value)
}
