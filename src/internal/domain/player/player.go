package player

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type ID string

type HandFlags uint8

const (
	HandLeft HandFlags = 1 << iota
	HandRight
	AllHandFlags = HandLeft | HandRight
)

type PositionFlags uint8

const (
	PositionPitcher PositionFlags = 1 << iota
	PositionCatcher
	PositionFirstBase
	PositionSecondBase
	PositionShortstop
	PositionThirdBase
	PositionOutfielder
	AllPositionFlags = PositionPitcher | PositionCatcher | PositionFirstBase | PositionSecondBase | PositionShortstop | PositionThirdBase | PositionOutfielder
)

var (
	ErrNotFound        = errors.New("player not found")
	ErrInactive        = errors.New("player is inactive")
	ErrVersionConflict = errors.New("player version conflict")
	ErrCorruptedData   = errors.New("corrupted player data")
)

func (f HandFlags) Valid() bool                          { return f != 0 && f&^AllHandFlags == 0 }
func (f HandFlags) Has(wanted HandFlags) bool            { return wanted != 0 && f&wanted == wanted }
func (f PositionFlags) Valid() bool                      { return f != 0 && f&^AllPositionFlags == 0 }
func (f PositionFlags) HasAny(wanted PositionFlags) bool { return f&wanted != 0 }
func (f PositionFlags) HasAll(wanted PositionFlags) bool { return wanted != 0 && f&wanted == wanted }

type Player struct {
	id        ID
	name      string
	batting   HandFlags
	throwing  HandFlags
	positions PositionFlags
	active    bool
	version   uint64
	createdAt time.Time
	updatedAt time.Time
}

func New(id ID, name string, batting, throwing HandFlags, positions PositionFlags, now time.Time) (Player, error) {
	return restore(id, name, batting, throwing, positions, true, 1, now, now)
}

func Restore(id ID, name string, batting, throwing HandFlags, positions PositionFlags, active bool, version uint64, createdAt, updatedAt time.Time) (Player, error) {
	player, err := restore(id, name, batting, throwing, positions, active, version, createdAt, updatedAt)
	if err != nil {
		return Player{}, fmt.Errorf("%w: %v", ErrCorruptedData, err)
	}
	return player, nil
}

func restore(id ID, name string, batting, throwing HandFlags, positions PositionFlags, active bool, version uint64, createdAt, updatedAt time.Time) (Player, error) {
	name = strings.TrimSpace(name)
	switch {
	case id == "":
		return Player{}, errors.New("player id is required")
	case name == "":
		return Player{}, errors.New("player name is required")
	case !batting.Valid():
		return Player{}, errors.New("invalid batting flags")
	case !throwing.Valid():
		return Player{}, errors.New("invalid throwing flags")
	case !positions.Valid():
		return Player{}, errors.New("invalid position flags")
	case version == 0:
		return Player{}, errors.New("player version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Player{}, errors.New("player timestamps are required")
	case updatedAt.Before(createdAt):
		return Player{}, errors.New("player updated time precedes creation")
	}
	return Player{id: id, name: name, batting: batting, throwing: throwing, positions: positions, active: active, version: version, createdAt: createdAt, updatedAt: updatedAt}, nil
}

func (p Player) ID() ID                   { return p.id }
func (p Player) Name() string             { return p.name }
func (p Player) Batting() HandFlags       { return p.batting }
func (p Player) Throwing() HandFlags      { return p.throwing }
func (p Player) Positions() PositionFlags { return p.positions }
func (p Player) Active() bool             { return p.active }
func (p Player) Version() uint64          { return p.version }
func (p Player) CreatedAt() time.Time     { return p.createdAt }
func (p Player) UpdatedAt() time.Time     { return p.updatedAt }

func (p *Player) UpdateProfile(name string, batting, throwing HandFlags, positions PositionFlags, now time.Time) error {
	updated, err := restore(p.id, name, batting, throwing, positions, p.active, p.version, p.createdAt, now)
	if err != nil {
		return err
	}
	updated.version++
	*p = updated
	return nil
}

func (p *Player) SetActive(active bool, now time.Time) error {
	if now.Before(p.createdAt) {
		return errors.New("player update time precedes creation")
	}
	p.active, p.updatedAt, p.version = active, now, p.version+1
	return nil
}
