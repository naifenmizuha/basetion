package player

import (
	"errors"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

func TestFlagsAndPlayerValidation(t *testing.T) {
	t.Parallel()
	positions := PositionPitcher | PositionOutfielder
	now := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	value, err := New("player-1", team.ID("team-1"), 18, " Player ", HandLeft|HandRight, HandRight, positions, now)
	if err != nil {
		t.Fatal(err)
	}
	if value.Name() != "Player" || value.TeamID() != "team-1" || value.JerseyNumber() != 18 {
		t.Fatalf("unexpected player: %#v", value)
	}
	if _, err := New("player-2", "", 0, "Player", HandLeft, HandRight, positions, now); err == nil {
		t.Fatal("blank team accepted")
	}
	if _, err := New("player-2", "team-1", 100, "Player", HandLeft, HandRight, positions, now); err == nil {
		t.Fatal("invalid jersey accepted")
	}
	if _, err := Restore("player-2", "team-1", 1, "Player", HandLeft, HandRight, PositionFlags(128), true, now, now); !errors.Is(err, ErrCorruptedData) {
		t.Fatalf("restore error=%v", err)
	}
}
func TestPlayerMutationUpdatesState(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	value, _ := New("player-1", "team-1", 1, "Player", HandLeft, HandRight, PositionPitcher, now)
	if err := value.ChangeJersey(2, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := value.UpdateProfile("Changed", HandRight, HandLeft, PositionCatcher, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if value.Name() != "Changed" || value.JerseyNumber() != 2 {
		t.Fatalf("unexpected update: %#v", value)
	}
}
func TestPlayerSoftDeleteIsDistinctFromInactive(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	value, _ := New("player", "team-1", 1, "Player", HandLeft, HandRight, PositionPitcher, now)
	if err := value.SetActive(false, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if value.Deleted() {
		t.Fatal("inactive player was deleted")
	}
	if err := value.Delete(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !value.Deleted() || value.Active() {
		t.Fatalf("deleted=%v active=%v", value.Deleted(), value.Active())
	}
}
