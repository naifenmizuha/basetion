package training

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type ID string

var (
	ErrNotFound        = errors.New("training record not found")
	ErrAlreadyExists   = errors.New("training record already exists for player and date")
	ErrVersionConflict = errors.New("training record version conflict")
	ErrCorruptedData   = errors.New("corrupted training record data")
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
	year, month, day := value.Date()
	return Date{year: year, month: month, day: day}
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

type Record struct {
	id           ID
	playerID     player.ID
	trainingDate Date
	content      string
	reflection   string
	version      uint64
	createdAt    time.Time
	updatedAt    time.Time
	deletedAt    *time.Time
}

func New(id ID, playerID player.ID, trainingDate Date, content, reflection string, now time.Time) (Record, error) {
	return restore(id, playerID, trainingDate, content, reflection, 1, now, now, nil)
}

func Restore(id ID, playerID player.ID, trainingDate Date, content, reflection string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Record, error) {
	record, err := restore(id, playerID, trainingDate, content, reflection, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrCorruptedData, err)
	}
	return record, nil
}

func restore(id ID, playerID player.ID, trainingDate Date, content, reflection string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Record, error) {
	content = strings.TrimSpace(content)
	reflection = strings.TrimSpace(reflection)
	switch {
	case id == "":
		return Record{}, errors.New("training record id is required")
	case playerID == "":
		return Record{}, errors.New("player id is required")
	case !trainingDate.Valid():
		return Record{}, errors.New("training date is required")
	case content == "":
		return Record{}, errors.New("training content is required")
	case version == 0:
		return Record{}, errors.New("training record version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Record{}, errors.New("training record timestamps are required")
	case updatedAt.Before(createdAt):
		return Record{}, errors.New("training record update time precedes creation")
	case deletedAt != nil && deletedAt.Before(createdAt):
		return Record{}, errors.New("training record deleted time precedes creation")
	}
	return Record{id: id, playerID: playerID, trainingDate: trainingDate, content: content, reflection: reflection, version: version, createdAt: createdAt, updatedAt: updatedAt, deletedAt: cloneTime(deletedAt)}, nil
}

func (r Record) ID() ID                { return r.id }
func (r Record) PlayerID() player.ID   { return r.playerID }
func (r Record) TrainingDate() Date    { return r.trainingDate }
func (r Record) Content() string       { return r.content }
func (r Record) Reflection() string    { return r.reflection }
func (r Record) Version() uint64       { return r.version }
func (r Record) CreatedAt() time.Time  { return r.createdAt }
func (r Record) UpdatedAt() time.Time  { return r.updatedAt }
func (r Record) DeletedAt() *time.Time { return cloneTime(r.deletedAt) }
func (r Record) Deleted() bool         { return r.deletedAt != nil }

func (r *Record) Update(content, reflection string, now time.Time) error {
	if r.Deleted() {
		return ErrNotFound
	}
	updated, err := restore(r.id, r.playerID, r.trainingDate, content, reflection, r.version+1, r.createdAt, now, nil)
	if err != nil {
		return err
	}
	*r = updated
	return nil
}

func (r *Record) Delete(now time.Time) error {
	if r.Deleted() {
		return ErrNotFound
	}
	if now.Before(r.createdAt) {
		return errors.New("training record deleted time precedes creation")
	}
	r.deletedAt, r.updatedAt, r.version = cloneTime(&now), now, r.version+1
	return nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
