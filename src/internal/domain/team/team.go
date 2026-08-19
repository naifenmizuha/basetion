package team

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ID string

var (
	ErrNotFound      = errors.New("team not found")
	ErrInactive      = errors.New("team is inactive")
	ErrCorruptedData = errors.New("corrupted team data")
)

type Team struct {
	id        ID
	name      string
	active    bool
	createdAt time.Time
	updatedAt time.Time
	deletedAt *time.Time
}

func New(id ID, name string, now time.Time) (Team, error) {
	return restore(id, name, true, now, now)
}

func Restore(id ID, name string, active bool, createdAt, updatedAt time.Time) (Team, error) {
	return RestoreDeleted(id, name, active, createdAt, updatedAt, nil)
}

func RestoreDeleted(id ID, name string, active bool, createdAt, updatedAt time.Time, deletedAt *time.Time) (Team, error) {
	team, err := restoreDeleted(id, name, active, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Team{}, fmt.Errorf("%w: %v", ErrCorruptedData, err)
	}
	return team, nil
}

func restore(id ID, name string, active bool, createdAt, updatedAt time.Time) (Team, error) {
	return restoreDeleted(id, name, active, createdAt, updatedAt, nil)
}

func restoreDeleted(id ID, name string, active bool, createdAt, updatedAt time.Time, deletedAt *time.Time) (Team, error) {
	name = strings.TrimSpace(name)
	switch {
	case id == "":
		return Team{}, errors.New("team id is required")
	case name == "":
		return Team{}, errors.New("team name is required")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Team{}, errors.New("team timestamps are required")
	case updatedAt.Before(createdAt):
		return Team{}, errors.New("team updated time precedes creation")
	case deletedAt != nil && deletedAt.Before(createdAt):
		return Team{}, errors.New("team deleted time precedes creation")
	}
	return Team{id: id, name: name, active: active, createdAt: createdAt, updatedAt: updatedAt, deletedAt: cloneTime(deletedAt)}, nil
}

func (t Team) ID() ID                { return t.id }
func (t Team) Name() string          { return t.name }
func (t Team) Active() bool          { return t.active }
func (t Team) CreatedAt() time.Time  { return t.createdAt }
func (t Team) UpdatedAt() time.Time  { return t.updatedAt }
func (t Team) DeletedAt() *time.Time { return cloneTime(t.deletedAt) }
func (t Team) Deleted() bool         { return t.deletedAt != nil }

func (t *Team) Delete(now time.Time) error {
	if t.Deleted() {
		return ErrNotFound
	}
	if now.Before(t.createdAt) {
		return errors.New("team deleted time precedes creation")
	}
	t.deletedAt, t.updatedAt = cloneTime(&now), now
	return nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
