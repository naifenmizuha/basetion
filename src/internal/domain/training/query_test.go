package training

import (
	"context"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type fakeQueryRepository struct {
	records  []Record
	err      error
	players  []player.ID
	limit    int
	from     *Date
	to       *Date
}

func (r *fakeQueryRepository) List(context.Context, Filter) ([]Record, error) {
	return nil, nil
}

func (r *fakeQueryRepository) ListByPlayers(_ context.Context, playerIDs []player.ID, from, to *Date, limit int) ([]Record, error) {
	r.players = playerIDs
	r.limit = limit
	r.from = from
	r.to = to
	if r.err != nil {
		return nil, r.err
	}
	return r.records, nil
}

type fakePlayerReader struct {
	players []player.Player
	byID    map[player.ID]player.Player
	err     error
}

func (r *fakePlayerReader) ListByName(_ context.Context, name string) ([]player.Player, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.players, nil
}

func (r *fakePlayerReader) GetByIDs(_ context.Context, ids []player.ID) ([]player.Player, error) {
	if r.err != nil {
		return nil, r.err
	}
	result := make([]player.Player, 0, len(ids))
	for _, id := range ids {
		if p, ok := r.byID[id]; ok {
			result = append(result, p)
		}
	}
	return result, nil
}

type fakeTeamReader struct {
	byID map[team.ID]string
	err  error
}

func (r *fakeTeamReader) GetByIDs(_ context.Context, ids []team.ID) (map[team.ID]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.byID, nil
}

func fixturePlayer(t *testing.T, id, teamID player.ID, name string) player.Player {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p, err := player.New(id, team.ID(teamID), 7, name, player.HandRight, player.HandRight, player.PositionFirstBase, now)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fixtureRecord(t *testing.T, id ID, playerID player.ID, date string) Record {
	t.Helper()
	now := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	d, err := ParseDate(date)
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(id, playerID, d, "短打练习", "触击方向稳定", now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestListViewsByNameReturnsMatchingRecords(t *testing.T) {
	p := fixturePlayer(t, "p1", "t1", "张三")
	records := []Record{fixtureRecord(t, "r1", "p1", "2026-08-01"), fixtureRecord(t, "r2", "p1", "2026-08-02")}
	repository := &fakeQueryRepository{records: records}
	players := &fakePlayerReader{players: []player.Player{p}, byID: map[player.ID]player.Player{p.ID(): p}}
	teams := &fakeTeamReader{byID: map[team.ID]string{"t1": "蜀汉队"}}
	service, err := NewQueryService(repository, WithPlayerReader(players), WithTeamReader(teams))
	if err != nil {
		t.Fatal(err)
	}
	from, _ := ParseDate("2026-08-01")
	views, err := service.ListViews(context.Background(), Filter{PlayerName: "张三", From: &from})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 {
		t.Fatalf("views=%d", len(views))
	}
	if views[0].PlayerName != "张三" || views[0].TeamName != "蜀汉队" {
		t.Fatalf("view[0]=%#v", views[0])
	}
}

func TestListViewsByNameAmbiguousReturnsAll(t *testing.T) {
	p1 := fixturePlayer(t, "p1", "t1", "张三")
	p2 := fixturePlayer(t, "p2", "t2", "张三")
	records := []Record{fixtureRecord(t, "r1", "p1", "2026-08-01"), fixtureRecord(t, "r2", "p2", "2026-08-02")}
	repository := &fakeQueryRepository{records: records}
	players := &fakePlayerReader{players: []player.Player{p1, p2}, byID: map[player.ID]player.Player{p1.ID(): p1, p2.ID(): p2}}
	teams := &fakeTeamReader{byID: map[team.ID]string{"t1": "蜀汉队", "t2": "魏国队"}}
	service, err := NewQueryService(repository, WithPlayerReader(players), WithTeamReader(teams))
	if err != nil {
		t.Fatal(err)
	}
	views, err := service.ListViews(context.Background(), Filter{PlayerName: "张三"})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 2 {
		t.Fatalf("views=%d", len(views))
	}
}

func TestListViewsByNameUnknownPlayer(t *testing.T) {
	repository := &fakeQueryRepository{}
	players := &fakePlayerReader{players: []player.Player{}, byID: map[player.ID]player.Player{}}
	teams := &fakeTeamReader{byID: map[team.ID]string{}}
	service, err := NewQueryService(repository, WithPlayerReader(players), WithTeamReader(teams))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ListViews(context.Background(), Filter{PlayerName: "李四"})
	if err == nil {
		t.Fatal("expected error for unknown player")
	}
}

func TestListViewsDateRangeOnlyReturnsAllRecords(t *testing.T) {
	p := fixturePlayer(t, "p1", "t1", "张三")
	records := []Record{fixtureRecord(t, "r1", "p1", "2026-08-01")}
	repository := &fakeQueryRepository{records: records}
	players := &fakePlayerReader{byID: map[player.ID]player.Player{p.ID(): p}}
	teams := &fakeTeamReader{byID: map[team.ID]string{"t1": "蜀汉队"}}
	service, err := NewQueryService(repository, WithPlayerReader(players), WithTeamReader(teams))
	if err != nil {
		t.Fatal(err)
	}
	from, _ := ParseDate("2026-08-01")
	to, _ := ParseDate("2026-08-07")
	views, err := service.ListViews(context.Background(), Filter{From: &from, To: &to})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("views=%d", len(views))
	}
}

func TestListViewsSkipsDeletedPlayerOrTeam(t *testing.T) {
	records := []Record{fixtureRecord(t, "r1", "p1", "2026-08-01")}
	repository := &fakeQueryRepository{records: records}
	players := &fakePlayerReader{byID: map[player.ID]player.Player{}}
	teams := &fakeTeamReader{byID: map[team.ID]string{}}
	service, err := NewQueryService(repository, WithPlayerReader(players), WithTeamReader(teams))
	if err != nil {
		t.Fatal(err)
	}
	views, err := service.ListViews(context.Background(), Filter{PlayerID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("expected deleted player record skipped, got %d", len(views))
	}
}

func TestListViewsRejectsReversedDateRange(t *testing.T) {
	service, err := NewQueryService(&fakeQueryRepository{}, WithPlayerReader(&fakePlayerReader{}), WithTeamReader(&fakeTeamReader{}))
	if err != nil {
		t.Fatal(err)
	}
	from, _ := ParseDate("2026-08-10")
	to, _ := ParseDate("2026-08-01")
	_, err = service.ListViews(context.Background(), Filter{From: &from, To: &to})
	if err == nil {
		t.Fatal("expected error for reversed range")
	}
}

func TestListViewsRequiresSomeFilter(t *testing.T) {
	service, err := NewQueryService(&fakeQueryRepository{}, WithPlayerReader(&fakePlayerReader{}), WithTeamReader(&fakeTeamReader{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ListViews(context.Background(), Filter{})
	if err == nil {
		t.Fatal("expected error for empty filter")
	}
}
