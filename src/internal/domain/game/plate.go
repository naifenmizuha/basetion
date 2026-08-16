package game

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type PlateID string
type Score struct {
	Home uint16
	Away uint16
}
type Half uint8

const (
	Top Half = iota + 1
	Bottom
)

type PlateType uint8

const (
	PlateOut PlateType = iota + 1
	PlateSingle
	PlateDouble
	PlateTriple
	PlateHomeRun
	PlateWalk
	PlateIntentionalWalk
	PlateStrikeout
	PlateHitByPitch
	PlateError
	PlateFieldersChoice
	PlateSacrifice
	PlateInterference
	PlateOther
)

var pitchPattern = regexp.MustCompile(`^[BSF]*$`)
var (
	ErrPlateNotFound        = errors.New("plate not found")
	ErrPlateVersionConflict = errors.New("plate version conflict")
	ErrCorruptedPlate       = errors.New("corrupted plate data")
)

type Plate struct {
	id                   PlateID
	matchID              MatchID
	sequence             uint32
	inning               uint16
	half                 Half
	battingOrder         uint8
	batterID, pitcherID  player.ID
	pitchSequence        string
	plateType            PlateType
	resultDescription    string
	runners              [3]*player.ID
	score                *Score
	version              uint64
	createdAt, updatedAt time.Time
	deletedAt            *time.Time
}

func NewPlate(id PlateID, matchID MatchID, sequence, inning int, half Half, battingOrder int, batterID, pitcherID player.ID, pitches string, plateType PlateType, description string, runners [3]*player.ID, homeScore, awayScore int, now time.Time) (Plate, error) {
	score, err := newScore(homeScore, awayScore)
	if err != nil {
		return Plate{}, err
	}
	return restorePlate(id, matchID, sequence, inning, half, battingOrder, batterID, pitcherID, pitches, plateType, description, runners, score, 1, now, now, nil)
}
func RestorePlate(id PlateID, matchID MatchID, sequence, inning int, half Half, battingOrder int, batterID, pitcherID player.ID, pitches string, plateType PlateType, description string, runners [3]*player.ID, score *Score, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Plate, error) {
	v, e := restorePlate(id, matchID, sequence, inning, half, battingOrder, batterID, pitcherID, pitches, plateType, description, runners, score, version, createdAt, updatedAt, deletedAt)
	if e != nil {
		return Plate{}, fmt.Errorf("%w: %v", ErrCorruptedPlate, e)
	}
	return v, nil
}
func restorePlate(id PlateID, matchID MatchID, sequence, inning int, half Half, battingOrder int, batterID, pitcherID player.ID, pitches string, plateType PlateType, description string, runners [3]*player.ID, score *Score, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Plate, error) {
	description = strings.TrimSpace(description)
	switch {
	case id == "" || matchID == "":
		return Plate{}, errors.New("plate and match ids are required")
	case sequence < 1:
		return Plate{}, errors.New("plate sequence must be positive")
	case inning < 1 || inning > 65535:
		return Plate{}, errors.New("inning is out of range")
	case half != Top && half != Bottom:
		return Plate{}, errors.New("invalid inning half")
	case battingOrder < 1 || battingOrder > 9:
		return Plate{}, errors.New("batting order must be between 1 and 9")
	case batterID == "" || pitcherID == "":
		return Plate{}, errors.New("batter and pitcher are required")
	case !pitchPattern.MatchString(pitches):
		return Plate{}, errors.New("pitch sequence accepts only B, S, and F")
	case plateType < PlateOut || plateType > PlateOther:
		return Plate{}, errors.New("invalid plate type")
	case description == "":
		return Plate{}, errors.New("result description is required")
	case version == 0:
		return Plate{}, errors.New("plate version must be positive")
	case createdAt.IsZero() || updatedAt.IsZero() || updatedAt.Before(createdAt):
		return Plate{}, errors.New("invalid plate timestamps")
	case deletedAt != nil && deletedAt.Before(createdAt):
		return Plate{}, errors.New("plate deleted time precedes creation")
	}
	return Plate{id: id, matchID: matchID, sequence: uint32(sequence), inning: uint16(inning), half: half, battingOrder: uint8(battingOrder), batterID: batterID, pitcherID: pitcherID, pitchSequence: pitches, plateType: plateType, resultDescription: description, runners: copyRunners(runners), score: copyScore(score), version: version, createdAt: createdAt, updatedAt: updatedAt, deletedAt: copyTime(deletedAt)}, nil
}
func newScore(home, away int) (*Score, error) {
	if home < 0 || home > 65535 || away < 0 || away > 65535 {
		return nil, errors.New("plate scores must be between 0 and 65535")
	}
	return &Score{Home: uint16(home), Away: uint16(away)}, nil
}
func copyScore(v *Score) *Score {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func copyRunners(v [3]*player.ID) [3]*player.ID {
	var r [3]*player.ID
	for i, p := range v {
		if p != nil {
			x := *p
			r[i] = &x
		}
	}
	return r
}
func (p Plate) ID() PlateID               { return p.id }
func (p Plate) MatchID() MatchID          { return p.matchID }
func (p Plate) Sequence() uint32          { return p.sequence }
func (p Plate) Inning() uint16            { return p.inning }
func (p Plate) Half() Half                { return p.half }
func (p Plate) BattingOrder() uint8       { return p.battingOrder }
func (p Plate) BatterID() player.ID       { return p.batterID }
func (p Plate) PitcherID() player.ID      { return p.pitcherID }
func (p Plate) PitchSequence() string     { return p.pitchSequence }
func (p Plate) Type() PlateType           { return p.plateType }
func (p Plate) ResultDescription() string { return p.resultDescription }
func (p Plate) Runners() [3]*player.ID    { return copyRunners(p.runners) }
func (p Plate) Score() *Score             { return copyScore(p.score) }
func (p Plate) Version() uint64           { return p.version }
func (p Plate) CreatedAt() time.Time      { return p.createdAt }
func (p Plate) UpdatedAt() time.Time      { return p.updatedAt }
func (p Plate) DeletedAt() *time.Time     { return copyTime(p.deletedAt) }
func (p *Plate) Update(sequence, inning int, half Half, battingOrder int, batterID, pitcherID player.ID, pitches string, plateType PlateType, description string, runners [3]*player.ID, homeScore, awayScore int, now time.Time) error {
	if p.deletedAt != nil {
		return ErrPlateNotFound
	}
	score, e := newScore(homeScore, awayScore)
	if e != nil {
		return e
	}
	v, e := restorePlate(p.id, p.matchID, sequence, inning, half, battingOrder, batterID, pitcherID, pitches, plateType, description, runners, score, p.version+1, p.createdAt, now, nil)
	if e == nil {
		*p = v
	}
	return e
}
func (p *Plate) Delete(now time.Time) error {
	if p.deletedAt != nil {
		return ErrPlateNotFound
	}
	if now.Before(p.createdAt) {
		return errors.New("plate deleted time precedes creation")
	}
	p.deletedAt, p.updatedAt, p.version = copyTime(&now), now, p.version+1
	return nil
}
