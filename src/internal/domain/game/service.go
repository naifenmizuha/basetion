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
type PlayRepository interface {
	Create(context.Context, Play) error
	Get(context.Context, PlayID) (Play, error)
	GetForUpdate(context.Context, PlayID) (Play, error)
	Replace(context.Context, Play, uint64, time.Time) error
	SoftDelete(context.Context, PlayID, uint64, time.Time) error
	SoftDeleteByMatch(context.Context, MatchID, time.Time) error
	ListByMatch(context.Context, MatchID) ([]Play, error)
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
	Plays   PlayRepository
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
		if e = r.Plays.SoftDeleteByMatch(ctx, id, now); e != nil {
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

func (s *Service) CreatePlay(ctx context.Context, value Play) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		if e := validatePlayReferences(ctx, r, value); e != nil {
			return e
		}
		plays, e := r.Plays.ListByMatch(ctx, value.MatchID())
		if e != nil {
			return e
		}
		if e = validatePlayNeighbors(value, plays, ""); e != nil {
			return e
		}
		return r.Plays.Create(ctx, value)
	})
}
func (s *Service) ReplacePlay(ctx context.Context, id PlayID, expectedVersion uint64, draft PlayDraft) (Play, error) {
	var result Play
	e := s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		current, e := r.Plays.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		if current.Version() != expectedVersion {
			return ErrPlayVersionConflict
		}
		draft.ID, draft.MatchID = id, current.MatchID()
		value, e := RestorePlay(draft, current.Version()+1, current.CreatedAt(), s.clock.Now(), nil)
		if e != nil {
			return e
		}
		if e = validatePlayReferences(ctx, r, value); e != nil {
			return e
		}
		plays, e := r.Plays.ListByMatch(ctx, value.MatchID())
		if e != nil {
			return e
		}
		if e = validatePlayNeighbors(value, plays, id); e != nil {
			return e
		}
		if e = r.Plays.Replace(ctx, value, expectedVersion, s.clock.Now()); e != nil {
			return e
		}
		result = value
		return nil
	})
	return result, e
}
func (s *Service) DeletePlay(ctx context.Context, id PlayID, expectedVersion uint64) error {
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		v, e := r.Plays.GetForUpdate(ctx, id)
		if e != nil {
			return e
		}
		if v.Version() != expectedVersion {
			return ErrPlayVersionConflict
		}
		return r.Plays.SoftDelete(ctx, id, expectedVersion, s.clock.Now())
	})
}

type GameRecord struct {
	Match   Match
	Lineups []Lineup
	Plays   []Play
}

func (s *Service) CreateGameRecord(ctx context.Context, value GameRecord) error {
	if err := validateGameRecord(value); err != nil {
		return err
	}
	return s.uow.WithinGameTransaction(ctx, func(r Repositories) error {
		if _, err := r.Teams.GetForUpdate(ctx, value.Match.HomeTeamID()); err != nil {
			return err
		}
		if _, err := r.Teams.GetForUpdate(ctx, value.Match.AwayTeamID()); err != nil {
			return err
		}
		if err := r.Matches.Create(ctx, value.Match); err != nil {
			return err
		}
		for _, lineup := range value.Lineups {
			if err := validateLineupReferences(ctx, r, lineup); err != nil {
				return err
			}
			if err := r.Lineups.Create(ctx, lineup); err != nil {
				return err
			}
		}
		for _, play := range value.Plays {
			if err := validatePlayReferences(ctx, r, play); err != nil {
				return err
			}
			if err := r.Plays.Create(ctx, play); err != nil {
				return err
			}
		}
		return nil
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
func validatePlayReferences(ctx context.Context, r Repositories, v Play) error {
	if _, e := r.Matches.GetForUpdate(ctx, v.MatchID()); e != nil {
		return e
	}
	ids := []player.ID{v.BatterID(), v.StartingPitcherID()}
	for _, pitch := range v.Pitches() {
		ids = append(ids, pitch.PitcherID, pitch.BatterID)
	}
	for _, runner := range v.RunnerOutcomes() {
		ids = append(ids, runner.RunnerID)
		if runner.ChargedPitcherID != nil {
			ids = append(ids, *runner.ChargedPitcherID)
		}
		if runner.RBIBatterID != nil {
			ids = append(ids, *runner.RBIBatterID)
		}
	}
	for _, fielding := range v.FieldingOutcomes() {
		ids = append(ids, fielding.FielderID)
	}
	for _, situation := range []Situation{v.Before(), v.After()} {
		for _, runner := range situation.Runners {
			if runner != nil {
				ids = append(ids, *runner)
			}
		}
	}
	seen := map[player.ID]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, e := r.Players.GetForUpdate(ctx, id); e != nil {
			return e
		}
	}
	return nil
}

func validateGameRecord(v GameRecord) error {
	for i, play := range v.Plays {
		if play.MatchID() != v.Match.ID() || play.Sequence() != uint32(i+1) {
			return errors.New("game record plays must be contiguous and belong to match")
		}
		if i > 0 && !situationsConnect(v.Plays[i-1], play) {
			return errors.New("game record play situations are discontinuous")
		}
	}
	return nil
}
func validatePlayNeighbors(value Play, plays []Play, except PlayID) error {
	filtered := make([]Play, 0, len(plays)+1)
	for _, play := range plays {
		if play.ID() != except {
			filtered = append(filtered, play)
		}
	}
	filtered = append(filtered, value)
	for i := 1; i < len(filtered); i++ {
		for j := i; j > 0 && filtered[j].Sequence() < filtered[j-1].Sequence(); j-- {
			filtered[j], filtered[j-1] = filtered[j-1], filtered[j]
		}
	}
	for i, play := range filtered {
		if play.Sequence() != uint32(i+1) {
			return errors.New("play sequence must be contiguous")
		}
		if i > 0 && !situationsConnect(filtered[i-1], play) {
			return errors.New("play situations are discontinuous")
		}
	}
	return nil
}
func situationsConnect(previous, next Play) bool {
	a, b := previous.After(), next.Before()
	if previous.Inning() == next.Inning() && previous.Half() == next.Half() {
		return a.Outs == b.Outs && a.HomeScore == b.HomeScore && a.AwayScore == b.AwayScore && sameRunners(a.Runners, b.Runners)
	}
	return a.Outs == 3 && b.Outs == 0 && b.HomeScore == a.HomeScore && b.AwayScore == a.AwayScore && sameRunners(b.Runners, [3]*player.ID{})
}
func sameRunners(a, b [3]*player.ID) bool {
	for i := range a {
		if a[i] == nil && b[i] == nil {
			continue
		}
		if a[i] == nil || b[i] == nil || *a[i] != *b[i] {
			return false
		}
	}
	return true
}

type Detail struct {
	Match   Match
	Lineups []Lineup
	Plays   []Play
}
