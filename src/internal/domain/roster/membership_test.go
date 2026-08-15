package roster

import (
	"errors"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

func mustDate(t *testing.T, value string) Date {
	t.Helper()
	result, err := ParseDate(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMembershipHalfOpenPeriodAndLeave(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)
	joined := mustDate(t, "2026-08-01")
	value, err := New("membership-1", team.ID("team-1"), player.ID("player-1"), 18, joined, now)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ActiveOn(joined) || !value.Current() {
		t.Fatal("membership not active on joined date")
	}
	left := mustDate(t, "2026-08-15")
	if err := value.Leave(left, left, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if value.ActiveOn(left) || value.Current() || value.Version() != 2 {
		t.Fatalf("unexpected ended membership: %#v", value)
	}
	if err := value.ChangeJersey(19, now); err == nil {
		t.Fatal("past membership jersey changed")
	}
}

func TestMembershipRejectsInvalidAndCorruptedData(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	joined := mustDate(t, "2026-08-10")
	if _, err := New("membership-1", "team", "player", 100, joined, now); err == nil {
		t.Fatal("invalid jersey accepted")
	}
	left := joined
	if _, err := Restore("membership-1", "team", "player", 1, joined, &left, 1, now, now); !errors.Is(err, ErrCorruptedData) {
		t.Fatalf("restore error=%v", err)
	}
	value, _ := New("membership-1", "team", "player", 1, joined, now)
	if err := value.Leave(mustDate(t, "2026-08-20"), mustDate(t, "2026-08-15"), now); err == nil {
		t.Fatal("future leave accepted")
	}
}
