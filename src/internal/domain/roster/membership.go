package roster

import (
	"errors"
	"fmt"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type ID string

var (
	ErrNotFound              = errors.New("membership not found")
	ErrVersionConflict       = errors.New("membership version conflict")
	ErrJerseyOccupied        = errors.New("jersey number is occupied")
	ErrAlreadyMember         = errors.New("player already has a current membership")
	ErrOverlappingMembership = errors.New("membership period overlaps existing membership")
	ErrCorruptedData         = errors.New("corrupted membership data")
)

// Date is a calendar date without a time zone or time-of-day.
type Date struct {
	year  int
	month time.Month
	day   int
}

func NewDate(year int, month time.Month, day int) (Date, error) {
	value := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if value.Year() != year || value.Month() != month || value.Day() != day {
		return Date{}, errors.New("invalid date")
	}
	return Date{year: year, month: month, day: day}, nil
}

func DateFromTime(value time.Time) Date {
	y, m, d := value.Date()
	return Date{year: y, month: m, day: d}
}
func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return Date{}, err
	}
	return DateFromTime(parsed), nil
}
func (d Date) Valid() bool { return d.year != 0 }
func (d Date) String() string {
	if !d.Valid() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day)
}
func (d Date) Time() time.Time        { return time.Date(d.year, d.month, d.day, 0, 0, 0, 0, time.UTC) }
func (d Date) Before(other Date) bool { return d.Time().Before(other.Time()) }
func (d Date) After(other Date) bool  { return d.Time().After(other.Time()) }

type Membership struct {
	id           ID
	teamID       team.ID
	playerID     player.ID
	jerseyNumber uint8
	joinedAt     Date
	leftAt       *Date
	version      uint64
	createdAt    time.Time
	updatedAt    time.Time
}

func New(id ID, teamID team.ID, playerID player.ID, jersey int, joinedAt Date, now time.Time) (Membership, error) {
	return restore(id, teamID, playerID, jersey, joinedAt, nil, 1, now, now)
}

func Restore(id ID, teamID team.ID, playerID player.ID, jersey int, joinedAt Date, leftAt *Date, version uint64, createdAt, updatedAt time.Time) (Membership, error) {
	membership, err := restore(id, teamID, playerID, jersey, joinedAt, leftAt, version, createdAt, updatedAt)
	if err != nil {
		return Membership{}, fmt.Errorf("%w: %v", ErrCorruptedData, err)
	}
	return membership, nil
}

func restore(id ID, teamID team.ID, playerID player.ID, jersey int, joinedAt Date, leftAt *Date, version uint64, createdAt, updatedAt time.Time) (Membership, error) {
	switch {
	case id == "":
		return Membership{}, errors.New("membership id is required")
	case teamID == "":
		return Membership{}, errors.New("team id is required")
	case playerID == "":
		return Membership{}, errors.New("player id is required")
	case jersey < 0 || jersey > 99:
		return Membership{}, errors.New("jersey number must be between 0 and 99")
	case !joinedAt.Valid():
		return Membership{}, errors.New("joined date is required")
	case leftAt != nil && !joinedAt.Before(*leftAt):
		return Membership{}, errors.New("left date must be after joined date")
	case version == 0:
		return Membership{}, errors.New("membership version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Membership{}, errors.New("membership timestamps are required")
	case updatedAt.Before(createdAt):
		return Membership{}, errors.New("membership updated time precedes creation")
	}
	return Membership{id: id, teamID: teamID, playerID: playerID, jerseyNumber: uint8(jersey), joinedAt: joinedAt, leftAt: leftAt, version: version, createdAt: createdAt, updatedAt: updatedAt}, nil
}

func (m Membership) ID() ID              { return m.id }
func (m Membership) TeamID() team.ID     { return m.teamID }
func (m Membership) PlayerID() player.ID { return m.playerID }
func (m Membership) JerseyNumber() uint8 { return m.jerseyNumber }
func (m Membership) JoinedAt() Date      { return m.joinedAt }
func (m Membership) LeftAt() *Date {
	if m.leftAt == nil {
		return nil
	}
	value := *m.leftAt
	return &value
}
func (m Membership) Version() uint64      { return m.version }
func (m Membership) CreatedAt() time.Time { return m.createdAt }
func (m Membership) UpdatedAt() time.Time { return m.updatedAt }
func (m Membership) Current() bool        { return m.leftAt == nil }
func (m Membership) ActiveOn(date Date) bool {
	return !date.Before(m.joinedAt) && (m.leftAt == nil || date.Before(*m.leftAt))
}

func (m *Membership) ChangeJersey(jersey int, now time.Time) error {
	if jersey < 0 || jersey > 99 {
		return errors.New("jersey number must be between 0 and 99")
	}
	if !m.Current() {
		return errors.New("cannot change jersey of a past membership")
	}
	m.jerseyNumber, m.updatedAt, m.version = uint8(jersey), now, m.version+1
	return nil
}

func (m *Membership) Leave(leftAt Date, today Date, now time.Time) error {
	if !m.Current() {
		return errors.New("membership already ended")
	}
	if !m.joinedAt.Before(leftAt) {
		return errors.New("left date must be after joined date")
	}
	if leftAt.After(today) {
		return errors.New("future left date is not supported")
	}
	m.leftAt, m.updatedAt, m.version = &leftAt, now, m.version+1
	return nil
}
