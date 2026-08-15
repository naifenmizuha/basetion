package roster

import (
	"context"
	"errors"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type TeamRepository interface {
	GetForUpdate(context.Context, team.ID) (team.Team, error)
}

type PlayerRepository interface {
	GetForUpdate(context.Context, player.ID) (player.Player, error)
}

type MembershipRepository interface {
	Create(context.Context, Membership) error
	Get(context.Context, ID) (Membership, error)
	GetForUpdate(context.Context, ID) (Membership, error)
	Update(context.Context, Membership, uint64) error
	CurrentByTeamAndPlayer(context.Context, team.ID, player.ID) (*Membership, error)
	JerseyOccupied(context.Context, team.ID, int, *ID) (bool, error)
	Overlaps(context.Context, team.ID, player.ID, Date, *Date, *ID) (bool, error)
}

type Repositories struct {
	Teams       TeamRepository
	Players     PlayerRepository
	Memberships MembershipRepository
}

type UnitOfWork interface {
	WithinTransaction(context.Context, func(Repositories) error) error
}

type Clock interface{ Now() time.Time }

type Service struct {
	uow   UnitOfWork
	clock Clock
}

func NewService(uow UnitOfWork, clock Clock) (*Service, error) {
	if uow == nil {
		return nil, errors.New("roster unit of work is required")
	}
	if clock == nil {
		return nil, errors.New("roster clock is required")
	}
	return &Service{uow: uow, clock: clock}, nil
}

func (s *Service) Assign(ctx context.Context, id ID, teamID team.ID, playerID player.ID, jersey int, joinedAt Date) (Membership, error) {
	now := s.clock.Now()
	if joinedAt.After(DateFromTime(now)) {
		return Membership{}, errors.New("future joined date is not supported")
	}
	membership, err := New(id, teamID, playerID, jersey, joinedAt, now)
	if err != nil {
		return Membership{}, err
	}
	err = s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		if err := validateParticipants(ctx, repos, teamID, playerID); err != nil {
			return err
		}
		current, err := repos.Memberships.CurrentByTeamAndPlayer(ctx, teamID, playerID)
		if err != nil {
			return err
		}
		if current != nil {
			return ErrAlreadyMember
		}
		occupied, err := repos.Memberships.JerseyOccupied(ctx, teamID, jersey, nil)
		if err != nil {
			return err
		}
		if occupied {
			return ErrJerseyOccupied
		}
		overlaps, err := repos.Memberships.Overlaps(ctx, teamID, playerID, joinedAt, nil, nil)
		if err != nil {
			return err
		}
		if overlaps {
			return ErrOverlappingMembership
		}
		return repos.Memberships.Create(ctx, membership)
	})
	if err != nil {
		return Membership{}, err
	}
	return membership, nil
}

func (s *Service) ChangeJersey(ctx context.Context, teamID team.ID, id ID, jersey int) (Membership, error) {
	var changed Membership
	err := s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		if _, err := repos.Teams.GetForUpdate(ctx, teamID); err != nil {
			return err
		}
		membership, err := repos.Memberships.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if membership.TeamID() != teamID {
			return ErrNotFound
		}
		occupied, err := repos.Memberships.JerseyOccupied(ctx, membership.TeamID(), jersey, &id)
		if err != nil {
			return err
		}
		if occupied {
			return ErrJerseyOccupied
		}
		expected := membership.Version()
		if err := membership.ChangeJersey(jersey, s.clock.Now()); err != nil {
			return err
		}
		if err := repos.Memberships.Update(ctx, membership, expected); err != nil {
			return err
		}
		changed = membership
		return nil
	})
	return changed, err
}

func (s *Service) Leave(ctx context.Context, teamID team.ID, id ID, leftAt Date) (Membership, error) {
	var ended Membership
	err := s.uow.WithinTransaction(ctx, func(repos Repositories) error {
		if _, err := repos.Teams.GetForUpdate(ctx, teamID); err != nil {
			return err
		}
		membership, err := repos.Memberships.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if membership.TeamID() != teamID {
			return ErrNotFound
		}
		expected := membership.Version()
		now := s.clock.Now()
		if err := membership.Leave(leftAt, DateFromTime(now), now); err != nil {
			return err
		}
		if err := repos.Memberships.Update(ctx, membership, expected); err != nil {
			return err
		}
		ended = membership
		return nil
	})
	return ended, err
}

func validateParticipants(ctx context.Context, repos Repositories, teamID team.ID, playerID player.ID) error {
	currentTeam, err := repos.Teams.GetForUpdate(ctx, teamID)
	if err != nil {
		return err
	}
	if !currentTeam.Active() {
		return team.ErrInactive
	}
	currentPlayer, err := repos.Players.GetForUpdate(ctx, playerID)
	if err != nil {
		return err
	}
	if !currentPlayer.Active() {
		return player.ErrInactive
	}
	return nil
}
