package game

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type MatchID string
type MatchStatus uint8

const (
	MatchScheduled MatchStatus = iota + 1
	MatchInProgress
	MatchFinal
	MatchCancelled
)

var (
	ErrMatchNotFound        = errors.New("match not found")
	ErrMatchVersionConflict = errors.New("match version conflict")
	ErrCorruptedMatch       = errors.New("corrupted match data")
)

type Match struct {
	id                     MatchID
	homeTeamID, awayTeamID team.ID
	scheduledAt            time.Time
	location               string
	status                 MatchStatus
	version                uint64
	createdAt, updatedAt   time.Time
	deletedAt              *time.Time
}

func NewMatch(id MatchID, home, away team.ID, scheduledAt time.Time, location string, status MatchStatus, now time.Time) (Match, error) {
	return restoreMatch(id, home, away, scheduledAt, location, status, 1, now, now, nil)
}

func RestoreMatch(id MatchID, home, away team.ID, scheduledAt time.Time, location string, status MatchStatus, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Match, error) {
	value, err := restoreMatch(id, home, away, scheduledAt, location, status, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Match{}, fmt.Errorf("%w: %v", ErrCorruptedMatch, err)
	}
	return value, nil
}

func restoreMatch(id MatchID, home, away team.ID, scheduledAt time.Time, location string, status MatchStatus, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Match, error) {
	location = strings.TrimSpace(location)
	switch {
	case id == "":
		return Match{}, errors.New("match id is required")
	case home == "" || away == "":
		return Match{}, errors.New("both teams are required")
	case home == away:
		return Match{}, errors.New("home and away teams must differ")
	case scheduledAt.IsZero():
		return Match{}, errors.New("scheduled time is required")
	case !status.Valid():
		return Match{}, errors.New("invalid match status")
	case version == 0:
		return Match{}, errors.New("match version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Match{}, errors.New("match timestamps are required")
	case updatedAt.Before(createdAt):
		return Match{}, errors.New("match updated time precedes creation")
	case deletedAt != nil && deletedAt.Before(createdAt):
		return Match{}, errors.New("match deleted time precedes creation")
	}
	return Match{id: id, homeTeamID: home, awayTeamID: away, scheduledAt: scheduledAt, location: location, status: status, version: version, createdAt: createdAt, updatedAt: updatedAt, deletedAt: copyTime(deletedAt)}, nil
}

func (s MatchStatus) Valid() bool { return s >= MatchScheduled && s <= MatchCancelled }

func (m Match) ID() MatchID            { return m.id }
func (m Match) HomeTeamID() team.ID    { return m.homeTeamID }
func (m Match) AwayTeamID() team.ID    { return m.awayTeamID }
func (m Match) ScheduledAt() time.Time { return m.scheduledAt }
func (m Match) Location() string       { return m.location }
func (m Match) Status() MatchStatus    { return m.status }
func (m Match) Version() uint64        { return m.version }
func (m Match) CreatedAt() time.Time   { return m.createdAt }
func (m Match) UpdatedAt() time.Time   { return m.updatedAt }
func (m Match) DeletedAt() *time.Time  { return copyTime(m.deletedAt) }

func (m *Match) Update(home, away team.ID, scheduledAt time.Time, location string, now time.Time) error {
	if m.deletedAt != nil {
		return ErrMatchNotFound
	}
	updated, err := restoreMatch(m.id, home, away, scheduledAt, location, m.status, m.version+1, m.createdAt, now, nil)
	if err != nil {
		return err
	}
	*m = updated
	return nil
}

func (m *Match) SetStatus(status MatchStatus, now time.Time) error {
	if m.deletedAt != nil {
		return ErrMatchNotFound
	}
	updated, err := restoreMatch(m.id, m.homeTeamID, m.awayTeamID, m.scheduledAt, m.location, status, m.version+1, m.createdAt, now, nil)
	if err == nil {
		*m = updated
	}
	return err
}

func (m *Match) Delete(now time.Time) error {
	if m.deletedAt != nil {
		return ErrMatchNotFound
	}
	if now.Before(m.createdAt) {
		return errors.New("match deleted time precedes creation")
	}
	m.deletedAt, m.updatedAt, m.version = copyTime(&now), now, m.version+1
	return nil
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
