package game

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type LineupID string
type LineupKind uint8

const (
	LineupStarter LineupKind = iota + 1
	LineupBackup
)

var (
	ErrLineupNotFound        = errors.New("lineup not found")
	ErrLineupVersionConflict = errors.New("lineup version conflict")
	ErrCorruptedLineup       = errors.New("corrupted lineup data")
)

type LineupEntry struct {
	id           LineupID
	playerID     player.ID
	battingOrder uint8
	position     player.PositionFlags
}

func NewLineupEntry(id LineupID, playerID player.ID, order int, position player.PositionFlags) (LineupEntry, error) {
	if id == "" {
		return LineupEntry{}, errors.New("lineup entry id is required")
	}
	if playerID == "" {
		return LineupEntry{}, errors.New("lineup player id is required")
	}
	if order < 1 || order > 9 {
		return LineupEntry{}, errors.New("batting order must be between 1 and 9")
	}
	if !singlePosition(position) {
		return LineupEntry{}, errors.New("lineup position must contain exactly one position")
	}
	return LineupEntry{id: id, playerID: playerID, battingOrder: uint8(order), position: position}, nil
}
func (e LineupEntry) ID() LineupID                   { return e.id }
func (e LineupEntry) PlayerID() player.ID            { return e.playerID }
func (e LineupEntry) BattingOrder() uint8            { return e.battingOrder }
func (e LineupEntry) Position() player.PositionFlags { return e.position }

type Lineup struct {
	matchID              MatchID
	teamID               team.ID
	kind                 LineupKind
	variantNumber        uint16
	variantName          string
	entries              []LineupEntry
	version              uint64
	createdAt, updatedAt time.Time
	deletedAt            *time.Time
}

func NewLineup(matchID MatchID, teamID team.ID, kind LineupKind, number int, name string, entries []LineupEntry, now time.Time) (Lineup, error) {
	return restoreLineup(matchID, teamID, kind, number, name, entries, 1, now, now, nil)
}
func RestoreLineup(matchID MatchID, teamID team.ID, kind LineupKind, number int, name string, entries []LineupEntry, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Lineup, error) {
	v, err := restoreLineup(matchID, teamID, kind, number, name, entries, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Lineup{}, fmt.Errorf("%w: %v", ErrCorruptedLineup, err)
	}
	return v, nil
}
func restoreLineup(matchID MatchID, teamID team.ID, kind LineupKind, number int, name string, entries []LineupEntry, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Lineup, error) {
	name = strings.TrimSpace(name)
	switch {
	case matchID == "" || teamID == "":
		return Lineup{}, errors.New("lineup match and team are required")
	case kind != LineupStarter && kind != LineupBackup:
		return Lineup{}, errors.New("invalid lineup kind")
	case kind == LineupStarter && number != 0:
		return Lineup{}, errors.New("starter lineup number must be zero")
	case kind == LineupBackup && number < 1:
		return Lineup{}, errors.New("backup lineup number must be positive")
	case number > 65535:
		return Lineup{}, errors.New("lineup number is too large")
	case name == "":
		return Lineup{}, errors.New("lineup name is required")
	case len(entries) == 0:
		return Lineup{}, errors.New("lineup entries are required")
	case version == 0:
		return Lineup{}, errors.New("lineup version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(createdAt):
		return Lineup{}, errors.New("invalid lineup timestamps")
	case deletedAt != nil && deletedAt.Before(createdAt):
		return Lineup{}, errors.New("lineup deleted time precedes creation")
	}
	players, orders, ids := map[player.ID]bool{}, map[uint8]bool{}, map[LineupID]bool{}
	for _, entry := range entries {
		if entry.id == "" || entry.playerID == "" || entry.battingOrder < 1 || entry.battingOrder > 9 || !singlePosition(entry.position) {
			return Lineup{}, errors.New("invalid lineup entry")
		}
		if players[entry.playerID] {
			return Lineup{}, errors.New("lineup player is duplicated")
		}
		if orders[entry.battingOrder] {
			return Lineup{}, errors.New("lineup batting order is duplicated")
		}
		if ids[entry.id] {
			return Lineup{}, errors.New("lineup entry id is duplicated")
		}
		players[entry.playerID], orders[entry.battingOrder], ids[entry.id] = true, true, true
	}
	return Lineup{matchID: matchID, teamID: teamID, kind: kind, variantNumber: uint16(number), variantName: name, entries: append([]LineupEntry(nil), entries...), version: version, createdAt: createdAt, updatedAt: updatedAt, deletedAt: copyTime(deletedAt)}, nil
}
func singlePosition(position player.PositionFlags) bool {
	return position.Valid() && position&(position-1) == 0
}
func (l Lineup) MatchID() MatchID       { return l.matchID }
func (l Lineup) TeamID() team.ID        { return l.teamID }
func (l Lineup) Kind() LineupKind       { return l.kind }
func (l Lineup) VariantNumber() uint16  { return l.variantNumber }
func (l Lineup) VariantName() string    { return l.variantName }
func (l Lineup) Entries() []LineupEntry { return append([]LineupEntry(nil), l.entries...) }
func (l Lineup) Version() uint64        { return l.version }
func (l Lineup) CreatedAt() time.Time   { return l.createdAt }
func (l Lineup) UpdatedAt() time.Time   { return l.updatedAt }
func (l Lineup) DeletedAt() *time.Time  { return copyTime(l.deletedAt) }
