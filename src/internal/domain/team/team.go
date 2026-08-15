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
	version   uint64
	createdAt time.Time
	updatedAt time.Time
}

func New(id ID, name string, now time.Time) (Team, error) {
	return restore(id, name, true, 1, now, now)
}

func Restore(id ID, name string, active bool, version uint64, createdAt, updatedAt time.Time) (Team, error) {
	team, err := restore(id, name, active, version, createdAt, updatedAt)
	if err != nil {
		return Team{}, fmt.Errorf("%w: %v", ErrCorruptedData, err)
	}
	return team, nil
}

func restore(id ID, name string, active bool, version uint64, createdAt, updatedAt time.Time) (Team, error) {
	name = strings.TrimSpace(name)
	switch {
	case id == "":
		return Team{}, errors.New("team id is required")
	case name == "":
		return Team{}, errors.New("team name is required")
	case version == 0:
		return Team{}, errors.New("team version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Team{}, errors.New("team timestamps are required")
	case updatedAt.Before(createdAt):
		return Team{}, errors.New("team updated time precedes creation")
	}
	return Team{id: id, name: name, active: active, version: version, createdAt: createdAt, updatedAt: updatedAt}, nil
}

func (t Team) ID() ID               { return t.id }
func (t Team) Name() string         { return t.name }
func (t Team) Active() bool         { return t.active }
func (t Team) Version() uint64      { return t.version }
func (t Team) CreatedAt() time.Time { return t.createdAt }
func (t Team) UpdatedAt() time.Time { return t.updatedAt }
