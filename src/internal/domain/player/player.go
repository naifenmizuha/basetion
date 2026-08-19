package player

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
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
	ErrNotFound       = errors.New("player not found")
	ErrInactive       = errors.New("player is inactive")
	ErrJerseyOccupied = errors.New("player jersey number is already occupied")
	ErrCorruptedData  = errors.New("corrupted player data")
)

func (f HandFlags) Valid() bool                          { return f != 0 && f&^AllHandFlags == 0 }
func (f HandFlags) Has(wanted HandFlags) bool            { return wanted != 0 && f&wanted == wanted }
func (f PositionFlags) Valid() bool                      { return f != 0 && f&^AllPositionFlags == 0 }
func (f PositionFlags) HasAny(wanted PositionFlags) bool { return f&wanted != 0 }
func (f PositionFlags) HasAll(wanted PositionFlags) bool { return wanted != 0 && f&wanted == wanted }

type Player struct {
	id           ID
	teamID       team.ID
	jerseyNumber uint8
	name         string
	batting      HandFlags
	throwing     HandFlags
	positions    PositionFlags
	active       bool
	createdAt    time.Time
	updatedAt    time.Time
	deletedAt    *time.Time
}

func New(id ID, teamID team.ID, jersey int, name string, batting, throwing HandFlags, positions PositionFlags, now time.Time) (Player, error) {
	return restore(id, teamID, jersey, name, batting, throwing, positions, true, now, now)
}

func Restore(id ID, teamID team.ID, jersey int, name string, batting, throwing HandFlags, positions PositionFlags, active bool, createdAt, updatedAt time.Time) (Player, error) {
	return RestoreDeleted(id, teamID, jersey, name, batting, throwing, positions, active, createdAt, updatedAt, nil)
}

func RestoreDeleted(id ID, teamID team.ID, jersey int, name string, batting, throwing HandFlags, positions PositionFlags, active bool, createdAt, updatedAt time.Time, deletedAt *time.Time) (Player, error) {
	value, err := restoreDeleted(id, teamID, jersey, name, batting, throwing, positions, active, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Player{}, fmt.Errorf("%w: %v", ErrCorruptedData, err)
	}
	return value, nil
}

func restore(id ID, teamID team.ID, jersey int, name string, batting, throwing HandFlags, positions PositionFlags, active bool, createdAt, updatedAt time.Time) (Player, error) {
	return restoreDeleted(id, teamID, jersey, name, batting, throwing, positions, active, createdAt, updatedAt, nil)
}

func restoreDeleted(id ID, teamID team.ID, jersey int, name string, batting, throwing HandFlags, positions PositionFlags, active bool, createdAt, updatedAt time.Time, deletedAt *time.Time) (Player, error) {
	name = strings.TrimSpace(name)
	switch {
	case id == "":
		return Player{}, errors.New("player id is required")
	case teamID == "":
		return Player{}, errors.New("player team id is required")
	case jersey < 0 || jersey > 99:
		return Player{}, errors.New("player jersey number must be between 0 and 99")
	case name == "":
		return Player{}, errors.New("player name is required")
	case !batting.Valid():
		return Player{}, errors.New("invalid batting flags")
	case !throwing.Valid():
		return Player{}, errors.New("invalid throwing flags")
	case !positions.Valid():
		return Player{}, errors.New("invalid position flags")
	case createdAt.IsZero() || updatedAt.IsZero():
		return Player{}, errors.New("player timestamps are required")
	case updatedAt.Before(createdAt):
		return Player{}, errors.New("player updated time precedes creation")
	case deletedAt != nil && deletedAt.Before(createdAt):
		return Player{}, errors.New("player deleted time precedes creation")
	}
	return Player{id: id, teamID: teamID, jerseyNumber: uint8(jersey), name: name, batting: batting, throwing: throwing, positions: positions, active: active, createdAt: createdAt, updatedAt: updatedAt, deletedAt: cloneTime(deletedAt)}, nil
}

func (p Player) ID() ID                   { return p.id }
func (p Player) TeamID() team.ID          { return p.teamID }
func (p Player) JerseyNumber() uint8      { return p.jerseyNumber }
func (p Player) Name() string             { return p.name }
func (p Player) Batting() HandFlags       { return p.batting }
func (p Player) Throwing() HandFlags      { return p.throwing }
func (p Player) Positions() PositionFlags { return p.positions }
func (p Player) Active() bool             { return p.active }
func (p Player) CreatedAt() time.Time     { return p.createdAt }
func (p Player) UpdatedAt() time.Time     { return p.updatedAt }
func (p Player) DeletedAt() *time.Time    { return cloneTime(p.deletedAt) }
func (p Player) Deleted() bool            { return p.deletedAt != nil }

func (p *Player) UpdateProfile(name string, batting, throwing HandFlags, positions PositionFlags, now time.Time) error {
	if p.Deleted() {
		return ErrNotFound
	}
	updated, err := restore(p.id, p.teamID, int(p.jerseyNumber), name, batting, throwing, positions, p.active, p.createdAt, now)
	if err != nil {
		return err
	}
	*p = updated
	return nil
}

func (p *Player) ChangeJersey(jersey int, now time.Time) error {
	if p.Deleted() {
		return ErrNotFound
	}
	if jersey < 0 || jersey > 99 {
		return errors.New("player jersey number must be between 0 and 99")
	}
	if now.Before(p.createdAt) {
		return errors.New("player update time precedes creation")
	}
	p.jerseyNumber, p.updatedAt = uint8(jersey), now
	return nil
}

func (p *Player) SetActive(active bool, now time.Time) error {
	if p.Deleted() {
		return ErrNotFound
	}
	if now.Before(p.createdAt) {
		return errors.New("player update time precedes creation")
	}
	p.active, p.updatedAt = active, now
	return nil
}
func (p *Player) Delete(now time.Time) error {
	if p.Deleted() {
		return ErrNotFound
	}
	if now.Before(p.createdAt) {
		return errors.New("player deleted time precedes creation")
	}
	p.deletedAt, p.updatedAt = cloneTime(&now), now
	return nil
}
func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
