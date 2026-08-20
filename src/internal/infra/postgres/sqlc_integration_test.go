//go:build integration

package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
)

type sqlcClock struct{ now time.Time }

func (c sqlcClock) Now() time.Time { return c.now }

type queryCounter struct{ count atomic.Int32 }

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *queryCounter) Reset() { c.count.Store(0) }

func integrationStore(t *testing.T) (*Store, *queryCounter) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test source path")
	}
	configPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "config", "config.toml")
	config, err := appconfig.LoadDatabaseFile(configPath)
	if err != nil {
		t.Fatalf("load integration database configuration: %v", err)
	}
	poolConfig, err := pgxpool.ParseConfig(config.Dev.URL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	counter := &queryCounter{}
	poolConfig.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("ping integration database: %v", err)
	}
	t.Cleanup(pool.Close)
	return &Store{pool: pool}, counter
}

type integrationGameUnitOfWork struct{ repositories game.Repositories }

func (u integrationGameUnitOfWork) WithinGameTransaction(_ context.Context, fn func(game.Repositories) error) error {
	return fn(u.repositories)
}

func TestSQLCPostgresRepositories(t *testing.T) {
	ctx := context.Background()
	store, counter := integrationStore(t)

	rollbackID := team.ID(uuid.NewString())
	rollbackTeam, err := team.New(rollbackID, "sqlc rollback "+uuid.NewString(), time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Teams().Create(ctx, rollbackTeam); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		value, err := store.Teams().Get(ctx, rollbackID)
		if err != nil {
			return
		}
		if err := value.Delete(time.Date(2026, 8, 19, 9, 1, 0, 0, time.UTC)); err == nil {
			_ = store.Teams().Update(ctx, value)
		}
	})
	rollbackPlayer, err := player.New(player.ID(uuid.NewString()), rollbackID, 99, "sqlc rolled back player", player.HandRight, player.HandRight, player.PositionCatcher, time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rollbackSentinel := errors.New("rollback integration transaction")
	err = store.WithinTransaction(ctx, func(repositories player.Repositories) error {
		if err := repositories.Players.Create(ctx, rollbackPlayer); err != nil {
			return err
		}
		return rollbackSentinel
	})
	if !errors.Is(err, rollbackSentinel) {
		t.Fatalf("transaction error = %v, want rollback sentinel", err)
	}
	if _, err := store.Players().Get(ctx, rollbackPlayer.ID()); !errors.Is(err, player.ErrNotFound) {
		t.Fatalf("rolled back player error = %v, want %v", err, player.ErrNotFound)
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	queries := sqlcgen.New(tx)
	teams := &TeamRepository{queries: queries}
	players := &PlayerRepository{queries: queries}
	trainingRecords := &TrainingRepository{queries: queries}

	now := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	homeID, awayID := team.ID(uuid.NewString()), team.ID(uuid.NewString())
	homeName, awayName := "sqlc home "+uuid.NewString(), "sqlc away "+uuid.NewString()
	for _, value := range []struct {
		id   team.ID
		name string
	}{{homeID, homeName}, {awayID, awayName}} {
		created, err := team.New(value.id, value.name, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := teams.Create(ctx, created); err != nil {
			t.Fatal(err)
		}
	}
	duplicateTeam, err := team.New(team.ID(uuid.NewString()), homeName, now)
	if err != nil {
		t.Fatal(err)
	}
	duplicateTeamTx, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicateTeamErr := (&TeamRepository{queries: sqlcgen.New(duplicateTeamTx)}).Create(ctx, duplicateTeam)
	if err := duplicateTeamTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(duplicateTeamErr, team.ErrNameOccupied) {
		t.Fatalf("duplicate team name error = %v, want %v", duplicateTeamErr, team.ErrNameOccupied)
	}
	reusableName := "sqlc reusable " + uuid.NewString()
	softDeleted, err := team.New(team.ID(uuid.NewString()), reusableName, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := teams.Create(ctx, softDeleted); err != nil {
		t.Fatal(err)
	}
	if err := softDeleted.Delete(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := teams.Update(ctx, softDeleted); err != nil {
		t.Fatal(err)
	}
	replacement, err := team.New(team.ID(uuid.NewString()), reusableName, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := teams.Create(ctx, replacement); err != nil {
		t.Fatalf("reuse soft-deleted team name: %v", err)
	}

	batterID, pitcherID := player.ID(uuid.NewString()), player.ID(uuid.NewString())
	for _, value := range []struct {
		id     player.ID
		teamID team.ID
		jersey int
		name   string
		pos    player.PositionFlags
	}{{batterID, homeID, 7, "sqlc batter", player.PositionOutfielder}, {pitcherID, awayID, 8, "sqlc pitcher", player.PositionPitcher}} {
		created, err := player.New(value.id, value.teamID, value.jersey, value.name, player.HandRight, player.HandRight, value.pos, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := players.Create(ctx, created); err != nil {
			t.Fatal(err)
		}
	}
	counter.Reset()
	batchTeams, err := teams.GetByNames(ctx, []string{awayName, homeName})
	if err != nil || len(batchTeams) != 2 {
		t.Fatalf("get teams by names: values=%#v err=%v", batchTeams, err)
	}
	batchPlayers, err := players.GetByTeamAndJerseys(ctx, []game.PlayerJerseyKey{{TeamID: homeID, JerseyNumber: 7}, {TeamID: awayID, JerseyNumber: 8}})
	if err != nil || len(batchPlayers) != 2 {
		t.Fatalf("get players by team and jerseys: values=%#v err=%v", batchPlayers, err)
	}
	if got := counter.count.Load(); got != 2 {
		t.Fatalf("batch identity query count = %d, want 2", got)
	}
	duplicate, err := player.New(player.ID(uuid.NewString()), homeID, 7, "sqlc duplicate", player.HandRight, player.HandRight, player.PositionCatcher, now)
	if err != nil {
		t.Fatal(err)
	}
	duplicateTx, err := tx.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duplicateErr := (&PlayerRepository{queries: sqlcgen.New(duplicateTx)}).Create(ctx, duplicate)
	if err := duplicateTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(duplicateErr, player.ErrJerseyOccupied) {
		t.Fatalf("duplicate jersey error = %v, want %v", duplicateErr, player.ErrJerseyOccupied)
	}

	trainingDate, err := training.ParseDate("2026-08-19")
	if err != nil {
		t.Fatal(err)
	}
	record, err := training.New(training.ID(uuid.NewString()), batterID, trainingDate, "batting", "steady", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := trainingRecords.Create(ctx, record); err != nil {
		t.Fatal(err)
	}
	current, err := trainingRecords.Get(ctx, record.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Update("fielding", "improved", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := trainingRecords.Update(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := current.Delete(now.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := trainingRecords.Update(ctx, current); err != nil {
		t.Fatal(err)
	}
	if _, err := trainingRecords.Get(ctx, record.ID()); !errors.Is(err, training.ErrNotFound) {
		t.Fatalf("soft-deleted training error = %v, want %v", err, training.ErrNotFound)
	}

	gameRepositories := game.Repositories{
		Matches: &MatchRepository{queries: queries},
		Lineups: &LineupRepository{queries: queries},
		Plays:   &PlayRepository{queries: queries},
		Teams:   teams,
		Players: players,
	}
	gameService, err := game.NewService(integrationGameUnitOfWork{repositories: gameRepositories}, sqlcClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	namedService, err := game.NewService(
		integrationGameUnitOfWork{repositories: gameRepositories}, sqlcClock{now: now},
		game.WithDirectRepositories(game.DirectRepositories{Matches: gameRepositories.Matches, Lineups: gameRepositories.Lineups, Plays: gameRepositories.Plays, Teams: teams, Players: players}),
		game.WithIDGenerator(uuid.NewString),
	)
	if err != nil {
		t.Fatal(err)
	}
	homeRef := game.PlayerReference{TeamName: homeName, JerseyNumber: 7}
	awayRef := game.PlayerReference{TeamName: awayName, JerseyNumber: 8}
	namedDraft := game.NamedGameRecordDraft{
		HomeTeamName: homeName, AwayTeamName: awayName, ScheduledAt: now.Add(30 * time.Hour).Format(time.RFC3339), Location: "named game field",
		HomeLineup: game.NamedLineupDraft{Name: "home starters", Entries: []game.NamedLineupEntryDraft{{Player: homeRef, BattingOrder: 1, Position: player.PositionOutfielder}}},
		AwayLineup: game.NamedLineupDraft{Name: "away starters", Entries: []game.NamedLineupEntryDraft{{Player: awayRef, BattingOrder: 1, Position: player.PositionPitcher}}},
		Plays:      []game.NamedPlayDraft{{Inning: 1, Half: game.Top, BattingOrder: 1, Batter: awayRef, StartingPitcher: homeRef, Before: game.NamedSituationDraft{}, After: game.NamedSituationDraft{Outs: 1}, BattingResult: game.BattingStrikeout, ResultDescription: "named strikeout", Pitches: []game.NamedPitchDraft{{Pitcher: homeRef, Batter: awayRef, Result: game.PitchSwingingStrike, StrikesBefore: 2, StrikesAfter: 3}}, FieldingOutcomes: []game.NamedFieldingOutcomeDraft{{Fielder: homeRef, Position: player.PositionOutfielder, Result: game.FieldingPutout}}}},
	}
	namedProgress, err := namedService.CreateNamedGameRecord(ctx, namedDraft)
	if err != nil || namedProgress.Status != "succeeded" || namedProgress.CompletedLineups != 2 || namedProgress.CompletedPlays != 1 {
		t.Fatalf("create named game: progress=%#v err=%v", namedProgress, err)
	}
	readStore := &Store{queries: queries}
	dateFrom, dateTo := now.Add(29*time.Hour), now.Add(31*time.Hour)
	namedRecords, err := readStore.GetMatchRecords(ctx, game.MatchFilter{ParticipantNames: []string{homeName}, ScheduledFrom: &dateFrom, ScheduledTo: &dateTo})
	if err != nil || len(namedRecords) != 1 || namedRecords[0].Summary.Location != "named game field" || len(namedRecords[0].Events) != 1 {
		t.Fatalf("read named game: records=%#v err=%v", namedRecords, err)
	}
	matchIDs := make([]game.MatchID, 0, 2)
	for index := 0; index < 2; index++ {
		matchID := game.MatchID(uuid.NewString())
		match, err := game.NewMatch(matchID, homeID, awayID, now.Add(time.Duration(index+1)*time.Hour), "sqlc field", game.MatchFinal, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := gameService.CreateMatch(ctx, match); err != nil {
			t.Fatal(err)
		}
		entry, err := game.NewLineupEntry(game.LineupID(uuid.NewString()), batterID, 1, player.PositionOutfielder)
		if err != nil {
			t.Fatal(err)
		}
		lineup, err := game.NewLineup(matchID, homeID, game.LineupStarter, 0, "sqlc starters", []game.LineupEntry{entry}, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := gameService.CreateLineup(ctx, lineup); err != nil {
			t.Fatal(err)
		}
		before, err := game.NewSituation(0, 0, 0, [3]*player.ID{})
		if err != nil {
			t.Fatal(err)
		}
		after, err := game.NewSituation(1, 0, 0, [3]*player.ID{})
		if err != nil {
			t.Fatal(err)
		}
		pitch, err := game.NewPitch(game.PitchID(uuid.NewString()), 1, pitcherID, batterID, game.PitchSwingingStrike, 0, 2, 0, 3, "", nil, nil, "strike three", now)
		if err != nil {
			t.Fatal(err)
		}
		play, err := game.NewPlay(game.PlayDraft{ID: game.PlayID(uuid.NewString()), MatchID: matchID, Sequence: 1, Inning: 1, Half: game.Top, BattingOrder: 1, BatterID: batterID, StartingPitcherID: pitcherID, Before: before, After: after, BattingResult: game.BattingStrikeout, ResultDescription: "strikeout", Pitches: []game.Pitch{pitch}}, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := gameService.CreatePlay(ctx, play); err != nil {
			t.Fatal(err)
		}
		matchIDs = append(matchIDs, matchID)
	}

	readStore = &Store{queries: queries}
	originalGamesEnd := now.Add(3 * time.Hour)
	filter := game.MatchFilter{ParticipantNames: []string{homeName}, ScheduledTo: &originalGamesEnd}
	counter.Reset()
	records, err := readStore.GetMatchRecords(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || len(records[0].Events) != 1 || len(records[1].Events) != 1 {
		t.Fatalf("match records = %#v", records)
	}
	if got := counter.count.Load(); got != 6 {
		t.Fatalf("record query count = %d, want 6 fixed batch queries", got)
	}
	counter.Reset()
	lineups, err := readStore.ListMatchLineups(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(lineups) != 2 || len(lineups[0].Lineups) != 1 || len(lineups[1].Lineups) != 1 {
		t.Fatalf("match lineups = %#v", lineups)
	}
	if got := counter.count.Load(); got != 2 {
		t.Fatalf("lineup query count = %d, want 2 fixed batch queries", got)
	}
	counter.Reset()
	performances, err := readStore.AnalyzeMatchPlayers(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(performances) != 2 || len(performances[0].Offense) != 1 || len(performances[1].Pitching) != 1 {
		t.Fatalf("match performances = %#v", performances)
	}
	if got := counter.count.Load(); got != 4 {
		t.Fatalf("performance query count = %d, want 4 fixed batch queries", got)
	}

	if err := gameService.DeleteMatch(ctx, matchIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := gameRepositories.Matches.Get(ctx, matchIDs[0]); !errors.Is(err, game.ErrMatchNotFound) {
		t.Fatalf("soft-deleted match error = %v, want %v", err, game.ErrMatchNotFound)
	}
	if _, err := gameRepositories.Lineups.Get(ctx, matchIDs[0], homeID, game.LineupStarter, 0); !errors.Is(err, game.ErrLineupNotFound) {
		t.Fatalf("soft-deleted lineup error = %v, want %v", err, game.ErrLineupNotFound)
	}
	if views, err := readStore.GetMatchRecords(ctx, filter); err != nil || len(views) != 1 {
		t.Fatalf("records after soft delete = %#v, error = %v", views, err)
	}
}
