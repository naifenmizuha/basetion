package player

import (
	"context"
	"errors"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type Repository interface {
	Create(context.Context, Player) error
	Get(context.Context, ID) (Player, error)
	Update(context.Context, Player) error
	JerseyOccupied(context.Context, team.ID, int, *ID) (bool, error)
	SoftDeleteByTeam(context.Context, team.ID, time.Time) error
}
type TeamRepository interface {
	Get(context.Context, team.ID) (team.Team, error)
	Update(context.Context, team.Team) error
}
type TrainingRepository interface {
	SoftDeleteByPlayer(context.Context, ID, time.Time) error
}
type Repositories struct {
	Teams    TeamRepository
	Players  Repository
	Training TrainingRepository
}
type UnitOfWork interface {
	WithinTransaction(context.Context, func(Repositories) error) error
}
type Clock interface{ Now() time.Time }

type Service struct {
	repository Repository
	uow        UnitOfWork
	clock      Clock
}

func NewService(repository Repository, uow UnitOfWork, clock Clock) (*Service, error) {
	if repository == nil {
		return nil, errors.New("player repository is required")
	}
	if uow == nil {
		return nil, errors.New("player unit of work is required")
	}
	if clock == nil {
		return nil, errors.New("player clock is required")
	}
	return &Service{repository: repository, uow: uow, clock: clock}, nil
}

func (s *Service) Create(ctx context.Context, id ID, teamID team.ID, jersey int, name string, batting, throwing HandFlags, positions PositionFlags) (Player, error) {
	created, err := New(id, teamID, jersey, name, batting, throwing, positions, s.clock.Now())
	if err != nil {
		return Player{}, err
	}
	err = s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		currentTeam, err := repos.Teams.Get(ctx, teamID)
		if err != nil {
			return err
		}
		if !currentTeam.Active() {
			return team.ErrInactive
		}
		occupied, err := repos.Players.JerseyOccupied(ctx, teamID, jersey, nil)
		if err != nil {
			return err
		}
		if occupied {
			return ErrJerseyOccupied
		}
		return repos.Players.Create(ctx, created)
	})
	if err != nil {
		return Player{}, err
	}
	return created, nil
}

func (s *Service) Update(ctx context.Context, id ID, name string, batting, throwing HandFlags, positions PositionFlags) (Player, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Player{}, err
	}
	if err := current.UpdateProfile(name, batting, throwing, positions, s.clock.Now()); err != nil {
		return Player{}, err
	}
	if err := s.repository.Update(ctx, current); err != nil {
		return Player{}, err
	}
	return current, nil
}
func (s *Service) SetActive(ctx context.Context, id ID, active bool) (Player, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return Player{}, err
	}
	if err := current.SetActive(active, s.clock.Now()); err != nil {
		return Player{}, err
	}
	if err := s.repository.Update(ctx, current); err != nil {
		return Player{}, err
	}
	return current, nil
}
func (s *Service) ChangeJersey(ctx context.Context, id ID, jersey int) (Player, error) {
	var changed Player
	err := s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		current, err := repos.Players.Get(ctx, id)
		if err != nil {
			return err
		}
		occupied, err := repos.Players.JerseyOccupied(ctx, current.TeamID(), jersey, &id)
		if err != nil {
			return err
		}
		if occupied {
			return ErrJerseyOccupied
		}
		if err := current.ChangeJersey(jersey, s.clock.Now()); err != nil {
			return err
		}
		if err := repos.Players.Update(ctx, current); err != nil {
			return err
		}
		changed = current
		return nil
	})
	return changed, err
}
func (s *Service) DeleteTeam(ctx context.Context, id team.ID) error {
	return s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		value, err := repos.Teams.Get(ctx, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := repos.Players.SoftDeleteByTeam(ctx, id, now); err != nil {
			return err
		}
		if err := value.Delete(now); err != nil {
			return err
		}
		return repos.Teams.Update(ctx, value)
	})
}
func (s *Service) Delete(ctx context.Context, id ID) error {
	return s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		value, err := repos.Players.Get(ctx, id)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		if err := repos.Training.SoftDeleteByPlayer(ctx, id, now); err != nil {
			return err
		}
		if err := value.Delete(now); err != nil {
			return err
		}
		return repos.Players.Update(ctx, value)
	})
}
