package player

import (
	"errors"
	"testing"
	"time"
)

func TestFlagsAndPlayerValidation(t *testing.T) {
	t.Parallel()
	if !(HandLeft | HandRight).Valid() || HandFlags(4).Valid() {
		t.Fatal("unexpected hand flag validation")
	}
	positions := PositionPitcher | PositionOutfielder
	if !positions.Valid() || !positions.HasAny(PositionOutfielder) || positions.HasAll(PositionPitcher|PositionCatcher) {
		t.Fatal("unexpected position behavior")
	}
	now := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	value, err := New("player-1", " Player ", HandLeft|HandRight, HandRight, positions, now)
	if err != nil {
		t.Fatal(err)
	}
	if value.Name() != "Player" || value.Version() != 1 {
		t.Fatalf("unexpected player: %#v", value)
	}
	if _, err := New("player-2", "", HandLeft, HandRight, positions, now); err == nil {
		t.Fatal("blank name accepted")
	}
	if _, err := New("player-2", "Player", 0, HandRight, positions, now); err == nil {
		t.Fatal("empty batting flags accepted")
	}
	if _, err := Restore("player-2", "Player", HandLeft, HandRight, PositionFlags(128), true, 1, now, now); !errors.Is(err, ErrCorruptedData) {
		t.Fatalf("restore error=%v", err)
	}
}

func TestPlayerMutationAdvancesVersion(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	value, _ := New("player-1", "Player", HandLeft, HandRight, PositionPitcher, now)
	if err := value.UpdateProfile("Changed", HandRight, HandLeft, PositionCatcher, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if value.Version() != 2 || value.Name() != "Changed" {
		t.Fatalf("unexpected update: %#v", value)
	}
	if err := value.SetActive(false, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if value.Version() != 3 || value.Active() {
		t.Fatalf("unexpected activation: %#v", value)
	}
}

func TestPlayerSoftDeleteIsDistinctFromInactive(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	value, err := New("player", "Player", HandLeft, HandRight, PositionPitcher, now)
	if err != nil {
		t.Fatal(err)
	}
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
