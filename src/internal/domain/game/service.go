package game

import (
	"context"
	"errors"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type MatchRepository interface {
	Create(context.Context, Match) error
	Get(context.Context, MatchID) (Match, error)
	GetForUpdate(context.Context, MatchID) (Match, error)
	Update(context.Context, Match, uint64) error
	List(context.Context) ([]Match, error)
}
type LineupRepository interface {
	Create(context.Context, Lineup) error
	Get(context.Context, MatchID, team.ID, LineupKind, uint16) (Lineup, error)
	GetForUpdate(context.Context, MatchID, team.ID, LineupKind, uint16) (Lineup, error)
	Replace(context.Context, Lineup, uint64, time.Time) error
	SoftDelete(context.Context, MatchID, team.ID, LineupKind, uint16, uint64, time.Time) error
	SoftDeleteByMatch(context.Context, MatchID, time.Time) error
	ListByMatch(context.Context, MatchID) ([]Lineup, error)
	NameOccupied(context.Context, MatchID, team.ID, string, LineupKind, uint16) (bool, error)
}
type PlateRepository interface {
	Create(context.Context, Plate) error
	Get(context.Context, PlateID) (Plate, error)
	GetForUpdate(context.Context, PlateID) (Plate, error)
	Update(context.Context, Plate, uint64) error
	SoftDeleteByMatch(context.Context, MatchID, time.Time) error
	ListByMatch(context.Context, MatchID) ([]Plate, error)
}
type TeamRepository interface {
	GetForUpdate(context.Context, team.ID) (team.Team, error)
}
type PlayerRepository interface {
	GetForUpdate(context.Context, player.ID) (player.Player, error)
}
type Repositories struct {
	Matches MatchRepository
	Lineups LineupRepository
	Plates  PlateRepository
	Teams   TeamRepository
	Players PlayerRepository
}
type UnitOfWork interface {
	WithinGameTransaction(context.Context, func(Repositories) error) error
}
type Clock interface{ Now() time.Time }
type Service struct {
	uow   UnitOfWork
	clock Clock
}

func NewService(uow UnitOfWork, clock Clock) (*Service, error) {
	if uow == nil {
		return nil, errors.New("game unit of work is required")
	}
	if clock == nil {
		return nil, errors.New("game clock is required")
	}
	return &Service{uow: uow, clock: clock}, nil
}

func (s *Service) CreateMatch(ctx context.Context, value Match) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		if _, e := r.Teams.GetForUpdate(ctx, value.HomeTeamID()); e != nil {
			return e
		}
		if _, e := r.Teams.GetForUpdate(ctx, value.AwayTeamID()); e != nil {
			return e
		}
		return r.Matches.Create(ctx, value)
	})
}
func (s *Service) CreateMatchWith(ctx context.Context, id MatchID, home, away team.ID, scheduled time.Time, location string, status MatchStatus) (Match, error) {
	value, err := NewMatch(id, home, away, scheduled, location, status, s.clock.Now())
	if err != nil {
		return Match{}, err
	}
	if err := s.CreateMatch(ctx, value); err != nil {
		return Match{}, err
	}
	return value, nil
}
func (s *Service) UpdateMatch(ctx context.Context, id MatchID, home, away team.ID, scheduled time.Time, location string) (Match, error) {
	var result Match
	e := s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Matches.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		if _, e = r.Teams.GetForUpdate(ctx, home); e != nil {
			return e
		}
		if _, e = r.Teams.GetForUpdate(ctx, away); e != nil {
			return e
		}
		expected := v.Version()
		if e = v.Update(home, away, scheduled, location, s.clock.Now()); e != nil {
			return e
		}
		if e = r.Matches.Update(ctx, v, expected); e != nil {
			return e
		}
		result = v
		return nil
	})
	return result, e
}
func (s *Service) SetMatchStatus(ctx context.Context, id MatchID, status MatchStatus) (Match, error) {
	var result Match
	e := s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Matches.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		expected := v.Version()
		if e = v.SetStatus(status, s.clock.Now()); e != nil {
			return e
		}
		if e = r.Matches.Update(ctx, v, expected); e != nil {
			return e
		}
		result = v
		return nil
	})
	return result, e
}
func (s *Service) DeleteMatch(ctx context.Context, id MatchID) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Matches.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		now, expected := s.clock.Now(), v.Version()
		if e = r.Plates.SoftDeleteByMatch(ctx, id, now); e != nil {
			return e
		}
		if e = r.Lineups.SoftDeleteByMatch(ctx, id, now); e != nil {
			return e
		}
		if e = v.Delete(now); e != nil {
			return e
		}
		return r.Matches.Update(ctx, v, expected)
	})
}

func (s *Service) CreateLineup(ctx context.Context, value Lineup) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		if e := validateLineupReferences(ctx, r, value); e != nil {
			return e
		}
		occupied, e := r.Lineups.NameOccupied(ctx, value.MatchID(), value.TeamID(), value.VariantName(), 0, 0)
		if e != nil {
			return e
		}
		if occupied {
			return errors.New("lineup name is already used")
		}
		return r.Lineups.Create(ctx, value)
	})
}
func (s *Service) CreateLineupWith(ctx context.Context, matchID MatchID, teamID team.ID, kind LineupKind, number int, name string, entries []LineupEntry) (Lineup, error) {
	value, err := NewLineup(matchID, teamID, kind, number, name, entries, s.clock.Now())
	if err != nil {
		return Lineup{}, err
	}
	if err := s.CreateLineup(ctx, value); err != nil {
		return Lineup{}, err
	}
	return value, nil
}
func (s *Service) ReplaceLineup(ctx context.Context, value Lineup) (Lineup, error) {
	var result Lineup
	e := s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		current, e := r.Lineups.GetForUpdate(ctx, value.MatchID(), value.TeamID(), value.Kind(), value.VariantNumber())
		if e != nil {
			return e
		}
		if e = validateLineupReferences(ctx, r, value); e != nil {
			return e
		}
		occupied, e := r.Lineups.NameOccupied(ctx, value.MatchID(), value.TeamID(), value.VariantName(), value.Kind(), value.VariantNumber())
		if e != nil {
			return e
		}
		if occupied {
			return errors.New("lineup name is already used")
		}
		replacement, e := RestoreLineup(value.MatchID(), value.TeamID(), value.Kind(), int(value.VariantNumber()), value.VariantName(), value.Entries(), current.Version()+1, current.CreatedAt(), s.clock.Now(), nil)
		if e != nil {
			return e
		}
		if e = r.Lineups.Replace(ctx, replacement, current.Version(), s.clock.Now()); e != nil {
			return e
		}
		result = replacement
		return nil
	})
	return result, e
}
func (s *Service) ReplaceLineupWith(ctx context.Context, matchID MatchID, teamID team.ID, kind LineupKind, number int, name string, entries []LineupEntry) (Lineup, error) {
	value, err := NewLineup(matchID, teamID, kind, number, name, entries, s.clock.Now())
	if err != nil {
		return Lineup{}, err
	}
	return s.ReplaceLineup(ctx, value)
}
func (s *Service) DeleteLineup(ctx context.Context, matchID MatchID, teamID team.ID, kind LineupKind, number uint16) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Lineups.GetForUpdate(ctx, matchID, teamID, kind, number)
		if e != nil {
			return e
		}
		return r.Lineups.SoftDelete(ctx, matchID, teamID, kind, number, v.Version(), s.clock.Now())
	})
}

func (s *Service) CreatePlate(ctx context.Context, value Plate) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		if e := validatePlateReferences(ctx, r, value); e != nil {
			return e
		}
		return r.Plates.Create(ctx, value)
	})
}
func (s *Service) CreatePlateWith(ctx context.Context, id PlateID, matchID MatchID, sequence, inning int, half Half, order int, batter, pitcher player.ID, pitches string, kind PlateType, description string, runners [3]*player.ID, homeScore, awayScore int) (Plate, error) {
	value, err := NewPlate(id, matchID, sequence, inning, half, order, batter, pitcher, pitches, kind, description, runners, homeScore, awayScore, s.clock.Now())
	if err != nil {
		return Plate{}, err
	}
	if err := s.CreatePlate(ctx, value); err != nil {
		return Plate{}, err
	}
	return value, nil
}
func (s *Service) UpdatePlate(ctx context.Context, id PlateID, sequence, inning int, half Half, order int, batter, pitcher player.ID, pitches string, kind PlateType, description string, runners [3]*player.ID, homeScore, awayScore int) (Plate, error) {
	var result Plate
	e := s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Plates.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		expected := v.Version()
		if e = v.Update(sequence, inning, half, order, batter, pitcher, pitches, kind, description, runners, homeScore, awayScore, s.clock.Now()); e != nil {
			return e
		}
		if e = validatePlateReferences(ctx, r, v); e != nil {
			return e
		}
		if e = r.Plates.Update(ctx, v, expected); e != nil {
			return e
		}
		result = v
		return nil
	})
	return result, e
}
func (s *Service) DeletePlate(ctx context.Context, id PlateID) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Plates.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		expected := v.Version()
		if e = v.Delete(s.clock.Now()); e != nil {
			return e
		}
		return r.Plates.Update(ctx, v, expected)
	})
}

func validateLineupReferences(ctx context.Context, r Repositories, v Lineup) error {
	m, e := r.Matches.GetForUpdate(ctx, v.MatchID())
	if e != nil {
		return e
	}
	if v.TeamID() != m.HomeTeamID() && v.TeamID() != m.AwayTeamID() {
		return errors.New("lineup team does not participate in match")
	}
	for _, entry := range v.Entries() {
		if _, e = r.Players.GetForUpdate(ctx, entry.PlayerID()); e != nil {
			return e
		}
	}
	return nil
}
func validatePlateReferences(ctx context.Context, r Repositories, v Plate) error {
	if _, e := r.Matches.GetForUpdate(ctx, v.MatchID()); e != nil {
		return e
	}
	ids := []player.ID{v.BatterID(), v.PitcherID()}
	for _, runner := range v.Runners() {
		if runner != nil {
			ids = append(ids, *runner)
		}
	}
	for _, id := range ids {
		if _, e := r.Players.GetForUpdate(ctx, id); e != nil {
			return e
		}
	}
	return nil
}

type Detail struct {
	Match   Match
	Lineups []Lineup
	Plates  []Plate
}
