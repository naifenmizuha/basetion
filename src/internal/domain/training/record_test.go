package training

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

var testNow = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

func TestRecordLifecycleAndValidation(t *testing.T) {
	date, _ := ParseDate("2026-08-17")
	value, err := New("record-1", "player-1", date, "  batting practice  ", "  felt good  ", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if value.Content() != "batting practice" || value.Reflection() != "felt good" || value.Version() != 1 {
		t.Fatalf("record=%#v", value)
	}
	if err := value.Update("running", "", testNow.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if value.Content() != "running" || value.Reflection() != "" || value.Version() != 2 {
		t.Fatalf("updated=%#v", value)
	}
	if err := value.Delete(testNow.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !value.Deleted() || value.Version() != 3 {
		t.Fatalf("deleted=%#v", value)
	}
	if err := value.Update("again", "", testNow.Add(3*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update deleted error=%v", err)
	}
	if _, err := New("record-2", "player-1", date, " ", "", testNow); err == nil {
		t.Fatal("empty content accepted")
	}
	if _, err := Restore("record-2", "player-1", date, "work", "", 0, testNow, testNow, nil); !errors.Is(err, ErrCorruptedData) {
		t.Fatalf("restore error=%v", err)
	}
}

type memoryRepository struct {
	value     Record
	createErr error
	updateErr error
	expected  uint64
}

func (r *memoryRepository) Create(_ context.Context, value Record) error {
	r.value = value
	return r.createErr
}
func (r *memoryRepository) Get(_ context.Context, _ ID) (Record, error) {
	if r.value.ID() == "" {
		return Record{}, ErrNotFound
	}
	return r.value, nil
}
func (r *memoryRepository) Update(_ context.Context, value Record, expected uint64) error {
	r.expected = expected
	if r.updateErr != nil {
		return r.updateErr
	}
	r.value = value
	return nil
}

type playerRepository struct {
	value player.Player
	err   error
}

func (r playerRepository) Get(context.Context, player.ID) (player.Player, error) {
	return r.value, r.err
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestServiceCreateUpdateDelete(t *testing.T) {
	p, _ := player.New("player-1", "Player", player.HandRight, player.HandRight, player.PositionPitcher, testNow)
	_ = p.SetActive(false, testNow.Add(time.Minute))
	repository := &memoryRepository{}
	service, err := NewService(repository, playerRepository{value: p}, fixedClock{now: testNow.Add(2 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	date, _ := ParseDate("2026-08-17")
	created, err := service.Create(context.Background(), "record-1", p.ID(), date, "work", "")
	if err != nil {
		t.Fatal(err)
	}
	if !created.TrainingDate().Valid() {
		t.Fatal("missing date")
	}
	updated, err := service.Update(context.Background(), created.ID(), "more work", "good")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version() != 2 || repository.expected != 1 {
		t.Fatalf("updated=%#v expected=%d", updated, repository.expected)
	}
	if err := service.Delete(context.Background(), created.ID()); err != nil {
		t.Fatal(err)
	}
	if !repository.value.Deleted() || repository.expected != 2 {
		t.Fatalf("deleted=%#v expected=%d", repository.value, repository.expected)
	}
}

func TestQueryServiceValidatesFilter(t *testing.T) {
	service, _ := NewQueryService(&queryRepository{})
	from, _ := ParseDate("2026-08-18")
	to, _ := ParseDate("2026-08-17")
	if _, err := service.List(context.Background(), Filter{}); err == nil {
		t.Fatal("empty player accepted")
	}
	if _, err := service.List(context.Background(), Filter{PlayerID: "player-1", From: &from, To: &to}); err == nil {
		t.Fatal("reversed range accepted")
	}
}

type queryRepository struct{}

func (*queryRepository) List(context.Context, Filter) ([]Record, error) { return []Record{}, nil }
