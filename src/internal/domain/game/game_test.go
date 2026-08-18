package game

import (
	"errors"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

func TestMatchValidationAndSoftDelete(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	if _, err := NewMatch("match", "team", "team", now, "Field", MatchScheduled, now); err == nil {
		t.Fatal("same teams accepted")
	}
	value, err := NewMatch("match", "home", "away", now, " Field ", MatchScheduled, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.SetStatus(MatchFinal, now.Add(time.Minute)); err != nil || value.Status() != MatchFinal {
		t.Fatalf("set status: value=%#v err=%v", value, err)
	}
	if err := value.SetStatus(MatchStatus(99), now.Add(2*time.Minute)); err == nil {
		t.Fatal("invalid status accepted")
	}
	if err := value.Delete(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if value.DeletedAt() == nil || value.Version() != 3 {
		t.Fatalf("deleted=%v version=%d", value.DeletedAt(), value.Version())
	}
	if err := value.Delete(now.Add(2 * time.Hour)); !errors.Is(err, ErrMatchNotFound) {
		t.Fatalf("second delete=%v", err)
	}
}

func TestLineupValidation(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	one, _ := NewLineupEntry("one", "player-1", 1, player.PositionPitcher)
	two, _ := NewLineupEntry("two", "player-2", 1, player.PositionCatcher)
	if _, err := NewLineup("match", team.ID("team"), LineupBackup, 0, "backup", []LineupEntry{one}, now); err == nil {
		t.Fatal("zero backup number accepted")
	}
	if _, err := NewLineup("match", team.ID("team"), LineupStarter, 0, "starter", []LineupEntry{one, two}, now); err == nil {
		t.Fatal("duplicate batting order accepted")
	}
	if _, err := NewLineupEntry("bad", "player", 2, player.PositionPitcher|player.PositionFirstBase); err == nil {
		t.Fatal("multiple positions accepted")
	}
	if _, err := NewLineup("match", team.ID("team"), LineupStarter, 0, "starter", []LineupEntry{one}, now); err != nil {
		t.Fatal(err)
	}
}

func TestPlayValidatesPitchAndSituation(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	before, _ := NewSituation(0, 0, 0, [3]*player.ID{})
	runner := player.ID("batter")
	after, _ := NewSituation(0, 0, 0, [3]*player.ID{&runner, nil, nil})
	pitch, err := NewPitch("pitch", 1, "pitcher", "batter", PitchInPlay, 0, 0, 0, 0, "", nil, nil, "in play", now)
	if err != nil {
		t.Fatal(err)
	}
	advanceTo := 1
	outcome, err := NewRunnerOutcome("runner-result", 1, "batter", RunnerAdvance, 0, &advanceTo, false, false, nil, nil, nil, "reached first", now)
	if err != nil {
		t.Fatal(err)
	}
	value, err := NewPlay(PlayDraft{ID: "play", MatchID: "match", Sequence: 1, Inning: 1, Half: Top, BattingOrder: 1, BatterID: "batter", StartingPitcherID: "pitcher", Before: before, After: after, BattingResult: BattingSingle, ResultDescription: "single", Pitches: []Pitch{pitch}, RunnerOutcomes: []RunnerOutcome{outcome}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if value.Sequence() != 1 || value.After().Runners[0] == nil {
		t.Fatalf("play=%#v", value)
	}
}
