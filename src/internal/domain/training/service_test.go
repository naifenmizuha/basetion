package training

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type fakeTrainingRepository struct {
	created []Record
	err     error
}

func (r *fakeTrainingRepository) Create(_ context.Context, value Record) error {
	if r.err != nil {
		return r.err
	}
	r.created = append(r.created, value)
	return nil
}

func (r *fakeTrainingRepository) Get(_ context.Context, id ID) (Record, error) {
	for _, value := range r.created {
		if value.ID() == id {
			return value, nil
		}
	}
	return Record{}, ErrNotFound
}

func (r *fakeTrainingRepository) Update(_ context.Context, value Record) error {
	return nil
}

type fakePlayerRepository struct {
	players []PlayerWithTeam
	err     error
}

func (r *fakePlayerRepository) Get(_ context.Context, id player.ID) (player.Player, error) {
	for _, value := range r.players {
		if value.Player.ID() == id {
			return value.Player, nil
		}
	}
	return player.Player{}, player.ErrNotFound
}

func (r *fakePlayerRepository) ListByNameWithTeam(_ context.Context, playerName, teamName string) ([]PlayerWithTeam, error) {
	if r.err != nil {
		return nil, r.err
	}
	if playerName != "张三" {
		return nil, nil
	}
	result := make([]PlayerWithTeam, 0, len(r.players))
	for _, value := range r.players {
		if teamName == "" || value.TeamName == teamName {
			result = append(result, value)
		}
	}
	return result, nil
}

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

func TestCreateByNameResolvesSinglePlayer(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	date, _ := ParseDate("2026-08-01")
	p, _ := player.New("p1", "t1", 7, "张三", player.HandRight, player.HandRight, player.PositionFirstBase, now)
	service, err := NewService(&fakeTrainingRepository{}, &fakePlayerRepository{players: []PlayerWithTeam{{Player: p, TeamName: "蜀汉队"}}}, fakeClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Create(context.Background(), "r1", "张三", "", date, "短打练习 40 分钟", "触击方向稳定")
	if err != nil {
		t.Fatal(err)
	}
	if record.PlayerID() != "p1" {
		t.Fatalf("player_id=%q", record.PlayerID())
	}
}

func TestCreateByNameRejectsUnknownPlayer(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	date, _ := ParseDate("2026-08-01")
	service, err := NewService(&fakeTrainingRepository{}, &fakePlayerRepository{}, fakeClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "r1", "李四", "", date, "投球练习", "")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestCreateByNameRejectsAmbiguousPlayer(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	date, _ := ParseDate("2026-08-01")
	p1, _ := player.New("p1", "t1", 7, "张三", player.HandRight, player.HandRight, player.PositionFirstBase, now)
	p2, _ := player.New("p2", "t2", 12, "张三", player.HandLeft, player.HandLeft, player.PositionPitcher, now)
	service, err := NewService(&fakeTrainingRepository{}, &fakePlayerRepository{players: []PlayerWithTeam{{Player: p1, TeamName: "蜀汉队"}, {Player: p2, TeamName: "魏国队"}}}, fakeClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "r1", "张三", "", date, "投球练习", "")
	if err == nil || !strings.Contains(err.Error(), "蜀汉队") || !strings.Contains(err.Error(), "魏国队") {
		t.Fatalf("expected ambiguous error with teams, got %v", err)
	}
}

func TestCreateByNameRejectsEmptyName(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	date, _ := ParseDate("2026-08-01")
	service, err := NewService(&fakeTrainingRepository{}, &fakePlayerRepository{}, fakeClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "r1", "  ", "", date, "投球练习", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateByNameWithTeamResolvesUnique(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	date, _ := ParseDate("2026-08-01")
	p1, _ := player.New("p1", "t1", 7, "张三", player.HandRight, player.HandRight, player.PositionFirstBase, now)
	p2, _ := player.New("p2", "t2", 12, "张三", player.HandLeft, player.HandLeft, player.PositionPitcher, now)
	repository := &fakePlayerRepository{players: []PlayerWithTeam{{Player: p1, TeamName: "蜀汉队"}, {Player: p2, TeamName: "魏国队"}}}
	service, err := NewService(&fakeTrainingRepository{}, repository, fakeClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.Create(context.Background(), "r1", "张三", "蜀汉队", date, "投球练习", "")
	if err != nil {
		t.Fatal(err)
	}
	if record.PlayerID() != "p1" {
		t.Fatalf("player_id=%q", record.PlayerID())
	}
}

func TestCreateByNameTeamNameNoMatch(t *testing.T) {
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	date, _ := ParseDate("2026-08-01")
	p1, _ := player.New("p1", "t1", 7, "张三", player.HandRight, player.HandRight, player.PositionFirstBase, now)
	repository := &fakePlayerRepository{players: []PlayerWithTeam{{Player: p1, TeamName: "蜀汉队"}}}
	service, err := NewService(&fakeTrainingRepository{}, repository, fakeClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(context.Background(), "r1", "张三", "魏国队", date, "投球练习", "")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found with team hint, got %v", err)
	}
}
