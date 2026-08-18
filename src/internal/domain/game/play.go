package game

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type PlayID string
type PitchID string
type RunnerResultID string
type FieldingResultID string
type Score struct{ Home, Away uint16 }
type Half uint8

const (
	Top Half = iota + 1
	Bottom
)

var (
	ErrPlayNotFound        = errors.New("play not found")
	ErrPlayVersionConflict = errors.New("play version conflict")
	ErrCorruptedPlay       = errors.New("corrupted play data")
)

type Situation struct {
	Outs      uint8
	HomeScore uint16
	AwayScore uint16
	Runners   [3]*player.ID
}

func NewSituation(outs, homeScore, awayScore int, runners [3]*player.ID) (Situation, error) {
	if outs < 0 || outs > 3 {
		return Situation{}, errors.New("situation outs must be between 0 and 3")
	}
	if homeScore < 0 || homeScore > 65535 || awayScore < 0 || awayScore > 65535 {
		return Situation{}, errors.New("situation scores must be between 0 and 65535")
	}
	seen := map[player.ID]bool{}
	for _, runner := range runners {
		if runner == nil {
			continue
		}
		if *runner == "" || seen[*runner] {
			return Situation{}, errors.New("situation runners must be non-empty and unique")
		}
		seen[*runner] = true
	}
	return Situation{Outs: uint8(outs), HomeScore: uint16(homeScore), AwayScore: uint16(awayScore), Runners: copyRunners(runners)}, nil
}

type PitchResult uint8

const (
	PitchBall PitchResult = iota + 1
	PitchCalledStrike
	PitchSwingingStrike
	PitchFoul
	PitchFoulTip
	PitchInPlay
	PitchHitByPitch
	PitchIntentionalBall
	PitchPitchout
	PitchOther
)

func (v PitchResult) Valid() bool { return v >= PitchBall && v <= PitchOther }

type Pitch struct {
	ID            PitchID
	Sequence      uint16
	PitcherID     player.ID
	BatterID      player.ID
	Result        PitchResult
	BallsBefore   uint8
	StrikesBefore uint8
	BallsAfter    uint8
	StrikesAfter  uint8
	PitchType     string
	Velocity      *float64
	Zone          *uint8
	Description   string
	Version       uint64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func NewPitch(id PitchID, sequence int, pitcherID, batterID player.ID, result PitchResult, ballsBefore, strikesBefore, ballsAfter, strikesAfter int, pitchType string, velocity *float64, zone *int, description string, now time.Time) (Pitch, error) {
	var convertedZone *uint8
	if zone != nil {
		if *zone < 0 || *zone > 255 {
			return Pitch{}, errors.New("pitch zone is out of range")
		}
		value := uint8(*zone)
		convertedZone = &value
	}
	return restorePitch(id, sequence, pitcherID, batterID, result, ballsBefore, strikesBefore, ballsAfter, strikesAfter, pitchType, velocity, convertedZone, description, 1, now, now, nil)
}

func RestorePitch(id PitchID, sequence int, pitcherID, batterID player.ID, result PitchResult, ballsBefore, strikesBefore, ballsAfter, strikesAfter int, pitchType string, velocity *float64, zone *uint8, description string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Pitch, error) {
	v, err := restorePitch(id, sequence, pitcherID, batterID, result, ballsBefore, strikesBefore, ballsAfter, strikesAfter, pitchType, velocity, zone, description, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Pitch{}, fmt.Errorf("%w: %v", ErrCorruptedPlay, err)
	}
	return v, nil
}

func restorePitch(id PitchID, sequence int, pitcherID, batterID player.ID, result PitchResult, ballsBefore, strikesBefore, ballsAfter, strikesAfter int, pitchType string, velocity *float64, zone *uint8, description string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Pitch, error) {
	description = strings.TrimSpace(description)
	if id == "" || pitcherID == "" || batterID == "" {
		return Pitch{}, errors.New("pitch id, pitcher and batter are required")
	}
	if sequence < 1 || sequence > 65535 || !result.Valid() {
		return Pitch{}, errors.New("invalid pitch sequence or result")
	}
	if ballsBefore < 0 || ballsBefore > 3 || strikesBefore < 0 || strikesBefore > 2 || ballsAfter < 0 || ballsAfter > 4 || strikesAfter < 0 || strikesAfter > 3 {
		return Pitch{}, errors.New("invalid pitch count")
	}
	if velocity != nil && *velocity < 0 {
		return Pitch{}, errors.New("pitch velocity must not be negative")
	}
	if version == 0 || createdAt.IsZero() || updatedAt.Before(createdAt) || deletedAt != nil && deletedAt.Before(createdAt) {
		return Pitch{}, errors.New("invalid pitch lifecycle")
	}
	return Pitch{ID: id, Sequence: uint16(sequence), PitcherID: pitcherID, BatterID: batterID, Result: result, BallsBefore: uint8(ballsBefore), StrikesBefore: uint8(strikesBefore), BallsAfter: uint8(ballsAfter), StrikesAfter: uint8(strikesAfter), PitchType: strings.TrimSpace(pitchType), Velocity: copyFloat(velocity), Zone: copyUint8(zone), Description: description, Version: version, CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: copyTime(deletedAt)}, nil
}

type BattingResult uint8

const (
	BattingSingle BattingResult = iota + 1
	BattingDouble
	BattingTriple
	BattingHomeRun
	BattingWalk
	BattingIntentionalWalk
	BattingHitByPitch
	BattingStrikeout
	BattingGroundOut
	BattingFlyOut
	BattingLineOut
	BattingFieldersChoice
	BattingReachedOnError
	BattingSacrificeBunt
	BattingSacrificeFly
	BattingInterference
	BattingOther
)

func (v BattingResult) Valid() bool { return v >= BattingSingle && v <= BattingOther }

type RunnerResult uint8

const (
	RunnerAdvance RunnerResult = iota + 1
	RunnerScore
	RunnerForceOut
	RunnerTagOut
	RunnerCaughtStealing
	RunnerPickedOff
	RunnerErrorAdvance
)

func (v RunnerResult) Valid() bool { return v >= RunnerAdvance && v <= RunnerErrorAdvance }

type RunnerOutcome struct {
	ID               RunnerResultID
	Sequence         uint16
	RunnerID         player.ID
	Result           RunnerResult
	FromBase         uint8
	ToBase           *uint8
	OutRecorded      bool
	Scored           bool
	ChargedPitcherID *player.ID
	Earned           *bool
	RBIBatterID      *player.ID
	Description      string
	Version          uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func NewRunnerOutcome(id RunnerResultID, sequence int, runnerID player.ID, result RunnerResult, fromBase int, toBase *int, outRecorded, scored bool, chargedPitcherID *player.ID, earned *bool, rbiBatterID *player.ID, description string, now time.Time) (RunnerOutcome, error) {
	return restoreRunnerOutcome(id, sequence, runnerID, result, fromBase, toBase, outRecorded, scored, chargedPitcherID, earned, rbiBatterID, description, 1, now, now, nil)
}

func RestoreRunnerOutcome(id RunnerResultID, sequence int, runnerID player.ID, result RunnerResult, fromBase int, toBase *int, outRecorded, scored bool, chargedPitcherID *player.ID, earned *bool, rbiBatterID *player.ID, description string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (RunnerOutcome, error) {
	v, err := restoreRunnerOutcome(id, sequence, runnerID, result, fromBase, toBase, outRecorded, scored, chargedPitcherID, earned, rbiBatterID, description, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return RunnerOutcome{}, fmt.Errorf("%w: %v", ErrCorruptedPlay, err)
	}
	return v, nil
}

func restoreRunnerOutcome(id RunnerResultID, sequence int, runnerID player.ID, result RunnerResult, fromBase int, toBase *int, outRecorded, scored bool, chargedPitcherID *player.ID, earned *bool, rbiBatterID *player.ID, description string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (RunnerOutcome, error) {
	if id == "" || runnerID == "" || sequence < 1 || sequence > 65535 || !result.Valid() || fromBase < 0 || fromBase > 3 {
		return RunnerOutcome{}, errors.New("invalid runner result identity or position")
	}
	var target *uint8
	if toBase != nil {
		if *toBase < 1 || *toBase > 4 {
			return RunnerOutcome{}, errors.New("runner target base is out of range")
		}
		v := uint8(*toBase)
		target = &v
	}
	if scored != (result == RunnerScore) || scored && target != nil && *target != 4 || scored && (chargedPitcherID == nil || earned == nil) {
		return RunnerOutcome{}, errors.New("inconsistent runner scoring result")
	}
	if version == 0 || createdAt.IsZero() || updatedAt.Before(createdAt) || deletedAt != nil && deletedAt.Before(createdAt) {
		return RunnerOutcome{}, errors.New("invalid runner result lifecycle")
	}
	return RunnerOutcome{ID: id, Sequence: uint16(sequence), RunnerID: runnerID, Result: result, FromBase: uint8(fromBase), ToBase: target, OutRecorded: outRecorded, Scored: scored, ChargedPitcherID: copyPlayer(chargedPitcherID), Earned: copyBool(earned), RBIBatterID: copyPlayer(rbiBatterID), Description: strings.TrimSpace(description), Version: version, CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: copyTime(deletedAt)}, nil
}

type FieldingResult uint8

const (
	FieldingPutout FieldingResult = iota + 1
	FieldingAssist
	FieldingError
	FieldingDoublePlay
	FieldingTriplePlay
	FieldingPassedBall
	FieldingCatcherInterference
	FieldingOther
)

func (v FieldingResult) Valid() bool { return v >= FieldingPutout && v <= FieldingOther }

type FieldingOutcome struct {
	ID          FieldingResultID
	Sequence    uint16
	FielderID   player.ID
	Position    player.PositionFlags
	Result      FieldingResult
	Description string
	Version     uint64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

func NewFieldingOutcome(id FieldingResultID, sequence int, fielderID player.ID, position player.PositionFlags, result FieldingResult, description string, now time.Time) (FieldingOutcome, error) {
	return restoreFieldingOutcome(id, sequence, fielderID, position, result, description, 1, now, now, nil)
}

func RestoreFieldingOutcome(id FieldingResultID, sequence int, fielderID player.ID, position player.PositionFlags, result FieldingResult, description string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (FieldingOutcome, error) {
	v, err := restoreFieldingOutcome(id, sequence, fielderID, position, result, description, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return FieldingOutcome{}, fmt.Errorf("%w: %v", ErrCorruptedPlay, err)
	}
	return v, nil
}

func restoreFieldingOutcome(id FieldingResultID, sequence int, fielderID player.ID, position player.PositionFlags, result FieldingResult, description string, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (FieldingOutcome, error) {
	if id == "" || fielderID == "" || sequence < 1 || sequence > 65535 || !singlePosition(position) || !result.Valid() {
		return FieldingOutcome{}, errors.New("invalid fielding result")
	}
	if version == 0 || createdAt.IsZero() || updatedAt.Before(createdAt) || deletedAt != nil && deletedAt.Before(createdAt) {
		return FieldingOutcome{}, errors.New("invalid fielding result lifecycle")
	}
	return FieldingOutcome{ID: id, Sequence: uint16(sequence), FielderID: fielderID, Position: position, Result: result, Description: strings.TrimSpace(description), Version: version, CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: copyTime(deletedAt)}, nil
}

type Play struct {
	id                   PlayID
	matchID              MatchID
	sequence             uint32
	inning               uint16
	half                 Half
	battingOrder         uint8
	batterID             player.ID
	startingPitcherID    player.ID
	before, after        Situation
	battingResult        BattingResult
	resultDescription    string
	pitches              []Pitch
	runnerOutcomes       []RunnerOutcome
	fieldingOutcomes     []FieldingOutcome
	version              uint64
	createdAt, updatedAt time.Time
	deletedAt            *time.Time
}

type PlayDraft struct {
	ID                             PlayID
	MatchID                        MatchID
	Sequence, Inning, BattingOrder int
	Half                           Half
	BatterID, StartingPitcherID    player.ID
	Before, After                  Situation
	BattingResult                  BattingResult
	ResultDescription              string
	Pitches                        []Pitch
	RunnerOutcomes                 []RunnerOutcome
	FieldingOutcomes               []FieldingOutcome
}

func NewPlay(d PlayDraft, now time.Time) (Play, error) { return restorePlay(d, 1, now, now, nil) }
func RestorePlay(d PlayDraft, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Play, error) {
	v, err := restorePlay(d, version, createdAt, updatedAt, deletedAt)
	if err != nil {
		return Play{}, fmt.Errorf("%w: %v", ErrCorruptedPlay, err)
	}
	return v, nil
}

func restorePlay(d PlayDraft, version uint64, createdAt, updatedAt time.Time, deletedAt *time.Time) (Play, error) {
	d.ResultDescription = strings.TrimSpace(d.ResultDescription)
	if d.ID == "" || d.MatchID == "" || d.BatterID == "" || d.StartingPitcherID == "" {
		return Play{}, errors.New("play ids are required")
	}
	if d.Sequence < 1 || d.Inning < 1 || d.Inning > 65535 || d.BattingOrder < 1 || d.BattingOrder > 9 || d.Half != Top && d.Half != Bottom {
		return Play{}, errors.New("invalid play position")
	}
	if d.Before.Outs > 2 || d.After.Outs > 3 || d.After.HomeScore < d.Before.HomeScore || d.After.AwayScore < d.Before.AwayScore {
		return Play{}, errors.New("invalid play situation")
	}
	if !d.BattingResult.Valid() || d.ResultDescription == "" || len(d.Pitches) == 0 {
		return Play{}, errors.New("play result and pitches are required")
	}
	if err := validateOrderedChildren(d.Pitches, d.RunnerOutcomes, d.FieldingOutcomes); err != nil {
		return Play{}, err
	}
	last := d.Pitches[len(d.Pitches)-1].Result
	if !pitchEndsResult(last, d.BattingResult) {
		return Play{}, errors.New("last pitch does not match batting result")
	}
	scored := 0
	for _, result := range d.RunnerOutcomes {
		if result.Scored {
			scored++
		}
	}
	if int(d.After.HomeScore-d.Before.HomeScore+d.After.AwayScore-d.Before.AwayScore) != scored {
		return Play{}, errors.New("runner scores do not match situation")
	}
	if version == 0 || createdAt.IsZero() || updatedAt.Before(createdAt) || deletedAt != nil && deletedAt.Before(createdAt) {
		return Play{}, errors.New("invalid play lifecycle")
	}
	return Play{id: d.ID, matchID: d.MatchID, sequence: uint32(d.Sequence), inning: uint16(d.Inning), half: d.Half, battingOrder: uint8(d.BattingOrder), batterID: d.BatterID, startingPitcherID: d.StartingPitcherID, before: copySituation(d.Before), after: copySituation(d.After), battingResult: d.BattingResult, resultDescription: d.ResultDescription, pitches: copyPitches(d.Pitches), runnerOutcomes: copyRunnerOutcomes(d.RunnerOutcomes), fieldingOutcomes: copyFieldingOutcomes(d.FieldingOutcomes), version: version, createdAt: createdAt, updatedAt: updatedAt, deletedAt: copyTime(deletedAt)}, nil
}

func validateOrderedChildren(pitches []Pitch, runners []RunnerOutcome, fielding []FieldingOutcome) error {
	for i, value := range pitches {
		if value.Sequence != uint16(i+1) || value.DeletedAt != nil {
			return errors.New("pitch sequence must be contiguous")
		}
	}
	for i, value := range runners {
		if value.Sequence != uint16(i+1) || value.DeletedAt != nil {
			return errors.New("runner result sequence must be contiguous")
		}
	}
	for i, value := range fielding {
		if value.Sequence != uint16(i+1) || value.DeletedAt != nil {
			return errors.New("fielding result sequence must be contiguous")
		}
	}
	return nil
}

func pitchEndsResult(last PitchResult, result BattingResult) bool {
	switch result {
	case BattingWalk, BattingIntentionalWalk:
		return last == PitchBall || last == PitchIntentionalBall
	case BattingStrikeout:
		return last == PitchCalledStrike || last == PitchSwingingStrike || last == PitchFoulTip
	case BattingHitByPitch:
		return last == PitchHitByPitch
	default:
		return last == PitchInPlay || last == PitchOther
	}
}

func (p Play) ID() PlayID                          { return p.id }
func (p Play) MatchID() MatchID                    { return p.matchID }
func (p Play) Sequence() uint32                    { return p.sequence }
func (p Play) Inning() uint16                      { return p.inning }
func (p Play) Half() Half                          { return p.half }
func (p Play) BattingOrder() uint8                 { return p.battingOrder }
func (p Play) BatterID() player.ID                 { return p.batterID }
func (p Play) StartingPitcherID() player.ID        { return p.startingPitcherID }
func (p Play) Before() Situation                   { return copySituation(p.before) }
func (p Play) After() Situation                    { return copySituation(p.after) }
func (p Play) BattingResult() BattingResult        { return p.battingResult }
func (p Play) ResultDescription() string           { return p.resultDescription }
func (p Play) Pitches() []Pitch                    { return copyPitches(p.pitches) }
func (p Play) RunnerOutcomes() []RunnerOutcome     { return copyRunnerOutcomes(p.runnerOutcomes) }
func (p Play) FieldingOutcomes() []FieldingOutcome { return copyFieldingOutcomes(p.fieldingOutcomes) }
func (p Play) Version() uint64                     { return p.version }
func (p Play) CreatedAt() time.Time                { return p.createdAt }
func (p Play) UpdatedAt() time.Time                { return p.updatedAt }
func (p Play) DeletedAt() *time.Time               { return copyTime(p.deletedAt) }
func (p Play) Draft() PlayDraft {
	return PlayDraft{ID: p.id, MatchID: p.matchID, Sequence: int(p.sequence), Inning: int(p.inning), Half: p.half, BattingOrder: int(p.battingOrder), BatterID: p.batterID, StartingPitcherID: p.startingPitcherID, Before: p.Before(), After: p.After(), BattingResult: p.battingResult, ResultDescription: p.resultDescription, Pitches: p.Pitches(), RunnerOutcomes: p.RunnerOutcomes(), FieldingOutcomes: p.FieldingOutcomes()}
}

func copySituation(v Situation) Situation { v.Runners = copyRunners(v.Runners); return v }
func copyRunners(v [3]*player.ID) [3]*player.ID {
	var r [3]*player.ID
	for i, p := range v {
		r[i] = copyPlayer(p)
	}
	return r
}
func copyPlayer(v *player.ID) *player.ID {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func copyBool(v *bool) *bool {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func copyFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func copyUint8(v *uint8) *uint8 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func copyPitches(v []Pitch) []Pitch                        { return append([]Pitch(nil), v...) }
func copyRunnerOutcomes(v []RunnerOutcome) []RunnerOutcome { return append([]RunnerOutcome(nil), v...) }
func copyFieldingOutcomes(v []FieldingOutcome) []FieldingOutcome {
	return append([]FieldingOutcome(nil), v...)
}
