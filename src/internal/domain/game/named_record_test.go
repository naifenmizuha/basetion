package game

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type namedClock struct{ now time.Time }

func (c namedClock) Now() time.Time { return c.now }

type unusedGameUOW struct{}

func (unusedGameUOW) WithinGameTransaction(context.Context, func(Repositories) error) error {
	panic("transaction must not be used")
}

type sequenceIDs struct{ next int }

func (s *sequenceIDs) New() string { s.next++; return fmt.Sprintf("id-%d", s.next) }

type namedTeamRepo struct {
	values map[string]team.Team
	calls  int
}

func (r *namedTeamRepo) GetByNames(_ context.Context, names []string) ([]team.Team, error) {
	r.calls++
	result := make([]team.Team, 0, len(names))
	for _, name := range names {
		if value, ok := r.values[name]; ok {
			result = append(result, value)
		}
	}
	return result, nil
}

type namedPlayerRepo struct {
	values    map[string]player.Player
	calls     int
	requested int
}

func (r *namedPlayerRepo) GetByTeamAndJerseys(_ context.Context, keys []PlayerJerseyKey) ([]player.Player, error) {
	r.calls++
	r.requested = len(keys)
	result := make([]player.Player, 0, len(keys))
	for _, key := range keys {
		if value, ok := r.values[fmt.Sprintf("%s/%d", key.TeamID, key.JerseyNumber)]; ok {
			result = append(result, value)
		}
	}
	return result, nil
}

type recordingMatchRepo struct{ created []Match }

func (r *recordingMatchRepo) Create(_ context.Context, v Match) error {
	r.created = append(r.created, v)
	return nil
}
func (*recordingMatchRepo) Get(context.Context, MatchID) (Match, error) { panic("unused") }
func (*recordingMatchRepo) Update(context.Context, Match) error         { panic("unused") }
func (*recordingMatchRepo) List(context.Context) ([]Match, error)       { panic("unused") }

type recordingLineupRepo struct{ created []Lineup }

func (r *recordingLineupRepo) Create(_ context.Context, v Lineup) error {
	r.created = append(r.created, v)
	return nil
}
func (*recordingLineupRepo) Get(context.Context, MatchID, team.ID, LineupKind, uint16) (Lineup, error) {
	panic("unused")
}
func (*recordingLineupRepo) Replace(context.Context, Lineup, time.Time) error    { panic("unused") }
func (*recordingLineupRepo) SoftDelete(context.Context, Lineup, time.Time) error { panic("unused") }
func (*recordingLineupRepo) SoftDeleteByMatch(context.Context, MatchID, time.Time) error {
	panic("unused")
}
func (*recordingLineupRepo) ListByMatch(context.Context, MatchID) ([]Lineup, error) { panic("unused") }
func (*recordingLineupRepo) NameOccupied(context.Context, MatchID, team.ID, string, LineupKind, uint16) (bool, error) {
	panic("unused")
}

type recordingPlayRepo struct {
	created []Play
	fail    bool
}

func (r *recordingPlayRepo) Create(_ context.Context, v Play) error {
	if r.fail {
		return errors.New("play write failed")
	}
	r.created = append(r.created, v)
	return nil
}
func (*recordingPlayRepo) Get(context.Context, PlayID) (Play, error)         { panic("unused") }
func (*recordingPlayRepo) Replace(context.Context, Play, time.Time) error    { panic("unused") }
func (*recordingPlayRepo) SoftDelete(context.Context, Play, time.Time) error { panic("unused") }
func (*recordingPlayRepo) SoftDeleteByMatch(context.Context, MatchID, time.Time) error {
	panic("unused")
}
func (*recordingPlayRepo) ListByMatch(context.Context, MatchID) ([]Play, error) { panic("unused") }

func namedRecordFixture(t *testing.T, failPlay bool) (*Service, NamedGameRecordDraft, *recordingMatchRepo, *recordingLineupRepo, *recordingPlayRepo, *namedTeamRepo, *namedPlayerRepo) {
	t.Helper()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	home, _ := team.New("home", "蜀汉队", now)
	away, _ := team.New("away", "曹魏队", now)
	batter, _ := player.New("batter", home.ID(), 18, "刘备", player.HandLeft, player.HandRight, player.PositionFirstBase, now)
	pitcher, _ := player.New("pitcher", away.ID(), 2, "荀彧", player.HandLeft, player.HandRight, player.PositionPitcher, now)
	matches, lineups, plays := &recordingMatchRepo{}, &recordingLineupRepo{}, &recordingPlayRepo{fail: failPlay}
	ids := &sequenceIDs{}
	teamRepo := &namedTeamRepo{values: map[string]team.Team{"蜀汉队": home, "曹魏队": away}}
	playerRepo := &namedPlayerRepo{values: map[string]player.Player{"home/18": batter, "away/2": pitcher}}
	service, err := NewService(unusedGameUOW{}, namedClock{now}, WithDirectRepositories(DirectRepositories{Matches: matches, Lineups: lineups, Plays: plays, Teams: teamRepo, Players: playerRepo}), WithIDGenerator(ids.New))
	if err != nil {
		t.Fatal(err)
	}
	homeRef := PlayerReference{TeamName: "蜀汉队", JerseyNumber: 18}
	awayRef := PlayerReference{TeamName: "曹魏队", JerseyNumber: 2}
	draft := NamedGameRecordDraft{HomeTeamName: "蜀汉队", AwayTeamName: "曹魏队", ScheduledAt: "2026-09-01T19:00:00+08:00", Location: "成都棒球场", HomeLineup: NamedLineupDraft{Name: "首发", Entries: []NamedLineupEntryDraft{{Player: homeRef, BattingOrder: 1, Position: player.PositionFirstBase}}}, AwayLineup: NamedLineupDraft{Name: "首发", Entries: []NamedLineupEntryDraft{{Player: awayRef, BattingOrder: 1, Position: player.PositionPitcher}}}, Plays: []NamedPlayDraft{{Inning: 1, Half: Top, BattingOrder: 1, Batter: homeRef, StartingPitcher: awayRef, Before: NamedSituationDraft{}, After: NamedSituationDraft{Outs: 1}, BattingResult: BattingStrikeout, ResultDescription: "挥棒落空三振", Pitches: []NamedPitchDraft{{Pitcher: awayRef, Batter: homeRef, Result: PitchSwingingStrike, StrikesBefore: 2, StrikesAfter: 3}}, FieldingOutcomes: []NamedFieldingOutcomeDraft{{Fielder: awayRef, Position: player.PositionPitcher, Result: FieldingPutout}}}}}
	return service, draft, matches, lineups, plays, teamRepo, playerRepo
}

func TestCreateNamedGameRecordWithoutTransaction(t *testing.T) {
	service, draft, matches, lineups, plays, teams, players := namedRecordFixture(t, false)
	result, err := service.CreateNamedGameRecord(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || !result.MatchCreated || result.CompletedLineups != 2 || result.CompletedPlays != 1 {
		t.Fatalf("result=%#v", result)
	}
	if len(matches.created) != 1 || matches.created[0].Status() != MatchFinal || len(lineups.created) != 2 || len(plays.created) != 1 {
		t.Fatalf("writes=%d/%d/%d", len(matches.created), len(lineups.created), len(plays.created))
	}
	if teams.calls != 1 || players.calls != 1 || players.requested != 2 {
		t.Fatalf("batch lookups=%d/%d requested=%d", teams.calls, players.calls, players.requested)
	}
}
func TestCreateNamedGameRecordReportsPartialWrite(t *testing.T) {
	service, draft, matches, lineups, _, _, _ := namedRecordFixture(t, true)
	result, err := service.CreateNamedGameRecord(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.StoppedStage != "play" || result.StoppedPlayIndex == nil || *result.StoppedPlayIndex != 1 || !result.PartialDetailsPossible || len(matches.created) != 1 || len(lineups.created) != 2 {
		t.Fatalf("result=%#v", result)
	}
}
func TestCreateNamedGameRecordValidatesBeforeWriting(t *testing.T) {
	service, draft, matches, _, _, teams, players := namedRecordFixture(t, false)
	draft.Plays[0].Batter = PlayerReference{TeamName: "蜀汉队", JerseyNumber: 99}
	if _, err := service.CreateNamedGameRecord(context.Background(), draft); err == nil || err.Error() != "players not found: 蜀汉队 #99" {
		t.Fatalf("err=%v", err)
	}
	if len(matches.created) != 0 {
		t.Fatal("match was written before validation completed")
	}
	if teams.calls != 1 || players.calls != 1 {
		t.Fatalf("batch lookups=%d/%d", teams.calls, players.calls)
	}
}
