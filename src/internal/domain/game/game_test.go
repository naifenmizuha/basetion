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

func TestPlatePitchSequenceAndRunners(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	runner := player.ID("runner")
	runners := [3]*player.ID{&runner, nil, nil}
	value, err := NewPlate("plate", "match", 1, 1, Top, 1, "batter", "pitcher", "BSSBF", PlateSingle, "single to right", runners, 1, 2, now)
	if err != nil {
		t.Fatal(err)
	}
	runner = "mutated"
	if got := value.Runners()[0]; got == nil || *got != "runner" {
		t.Fatalf("runner=%v", got)
	}
	if score := value.Score(); score == nil || score.Home != 1 || score.Away != 2 {
		t.Fatalf("score=%#v", score)
	}
	legacy, err := RestorePlate("legacy", "match", 2, 1, Top, 2, "batter", "pitcher", "", PlateOut, "legacy", [3]*player.ID{}, nil, 1, now, now, nil)
	if err != nil || legacy.Score() != nil {
		t.Fatalf("legacy score=%#v err=%v", legacy.Score(), err)
	}
	if err := legacy.Update(2, 1, Top, 2, "batter", "pitcher", "", PlateOut, "corrected", [3]*player.ID{}, 5, 1, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if score := legacy.Score(); score == nil || score.Home != 5 || score.Away != 1 {
		t.Fatalf("updated score=%#v", score)
	}
	if _, err := NewPlate("negative", "match", 3, 1, Top, 3, "batter", "pitcher", "", PlateOut, "bad score", [3]*player.ID{}, -1, 0, now); err == nil {
		t.Fatal("negative score accepted")
	}
	if _, err := NewPlate("bad", "match", 1, 1, Top, 1, "batter", "pitcher", "BSX", PlateOut, "out", [3]*player.ID{}, 0, 0, now); err == nil {
		t.Fatal("invalid pitch accepted")
	}
	if _, err := NewPlate("empty", "match", 2, 1, Top, 2, "batter", "pitcher", "", PlateOut, "first-pitch out", [3]*player.ID{}, 0, 0, now); err != nil {
		t.Fatal(err)
	}
}
