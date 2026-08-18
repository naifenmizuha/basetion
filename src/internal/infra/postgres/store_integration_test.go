package postgres

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appconfig "github.com/naifenmizuha/basetion/src/internal/config"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func integrationStore(t *testing.T) *Store {
	t.Helper()
	return integrationStoreWithConfig(t, integrationProfile(t))
}

func integrationProfile(t *testing.T) appconfig.DatabaseProfileConfig {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test source path")
	}
	configPath := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "config", "config.toml")
	databaseConfig, err := appconfig.LoadDatabaseFile(configPath)
	if err != nil {
		t.Fatalf("load integration database configuration: %v", err)
	}
	if databaseConfig.Dev.Mode != appconfig.DatabaseModeTemporary {
		t.Skip("database.dev.mode must be temporary for PostgreSQL integration tests")
	}
	return databaseConfig.Dev
}

func integrationStoreWithConfig(t *testing.T, profile appconfig.DatabaseProfileConfig, options ...TemporaryOption) *Store {
	t.Helper()
	ctx := context.Background()
	managed, err := OpenTemporary(ctx, profile.AdminURL, profile.TemporaryPrefix, options...)
	if err != nil {
		t.Fatalf("create integration database: %v", err)
	}
	name := managed.DatabaseName()
	// Cleanup callbacks are LIFO: close/drop first, then verify absence.
	t.Cleanup(func() {
		admin, err := pgxpool.New(context.Background(), profile.AdminURL)
		if err != nil {
			t.Errorf("open admin pool to verify cleanup: %v", err)
			return
		}
		defer admin.Close()
		var exists bool
		if err := admin.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, name).Scan(&exists); err != nil {
			t.Errorf("verify temporary database cleanup: %v", err)
		} else if exists {
			t.Errorf("temporary database %q still exists", name)
		}
	})
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := managed.Close(cleanupCtx); err != nil {
			t.Errorf("drop integration database: %v", err)
		}
	})
	return managed.Store
}

func TestPostgresTrainingRecordLifecycle(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	playerID := player.ID("00000000-0000-0000-0000-000000000501")
	playerService, _ := player.NewService(store.Players(), fixedClock{now: now})
	if _, err := playerService.Create(ctx, playerID, "Training Player", player.HandRight, player.HandRight, player.PositionPitcher); err != nil {
		t.Fatal(err)
	}
	repository := store.Training()
	service, _ := training.NewService(repository, store.Players(), fixedClock{now: now.Add(time.Hour)})
	firstDate, _ := training.ParseDate("2026-08-16")
	secondDate, _ := training.ParseDate("2026-08-17")
	first, err := service.Create(ctx, "00000000-0000-0000-0000-000000000511", playerID, firstDate, "跑步", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, "00000000-0000-0000-0000-000000000512", playerID, firstDate, "重复", ""); !errors.Is(err, training.ErrAlreadyExists) {
		t.Fatalf("duplicate error=%v", err)
	}
	second, err := service.Create(ctx, "00000000-0000-0000-0000-000000000513", playerID, secondDate, "打击", "顺畅")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(ctx, second.ID(), "守备", "稳定")
	if err != nil || updated.Version() != 2 {
		t.Fatalf("updated=%#v error=%v", updated, err)
	}
	from, _ := training.ParseDate("2026-08-16")
	values, err := repository.List(ctx, training.Filter{PlayerID: playerID, From: &from})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].ID() != second.ID() || values[1].ID() != first.ID() {
		t.Fatalf("values=%#v", values)
	}
	if err := service.Delete(ctx, first.ID()); err != nil {
		t.Fatal(err)
	}
	values, err = repository.List(ctx, training.Filter{PlayerID: playerID})
	if err != nil || len(values) != 1 {
		t.Fatalf("after delete=%#v error=%v", values, err)
	}
	if _, err := service.Create(ctx, "00000000-0000-0000-0000-000000000514", playerID, firstDate, "重新记录", ""); err != nil {
		t.Fatal(err)
	}
	rosterService, _ := roster.NewService(store, fixedClock{now: now.Add(2 * time.Hour)})
	if err := rosterService.DeletePlayer(ctx, playerID); err != nil {
		t.Fatal(err)
	}
	values, err = repository.List(ctx, training.Filter{PlayerID: playerID})
	if err != nil || len(values) != 0 {
		t.Fatalf("after player delete=%#v error=%v", values, err)
	}
}

func TestPostgresDevelopmentFixtures(t *testing.T) {
	store := integrationStoreWithConfig(t, integrationProfile(t), WithDevelopmentFixtures())
	teams, err := store.Teams().List(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 2 {
		t.Fatalf("teams=%#v", teams)
	}
	teamIDs := map[string]string{}
	for _, value := range teams {
		teamIDs[value.Name()] = string(value.ID())
	}
	players, err := store.RosterReader().ListPlayers(context.Background(), roster.PlayerFilter{TeamID: team.ID(teamIDs["蜀汉队"])})
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 20 {
		t.Fatalf("players=%#v", players)
	}
	weiPlayers, err := store.RosterReader().ListPlayers(context.Background(), roster.PlayerFilter{TeamID: team.ID(teamIDs["曹魏队"])})
	if err != nil {
		t.Fatal(err)
	}
	if len(weiPlayers) != 20 {
		t.Fatalf("wei players=%#v", weiPlayers)
	}
	trainingRecords, err := store.Training().List(context.Background(), training.Filter{PlayerID: "00000000-0000-0000-0000-000000000011"})
	if err != nil {
		t.Fatal(err)
	}
	var databaseToday time.Time
	if err := store.pool.QueryRow(context.Background(), `SELECT current_date`).Scan(&databaseToday); err != nil {
		t.Fatal(err)
	}
	if len(trainingRecords) != 3 || trainingRecords[0].TrainingDate() != training.DateFromTime(databaseToday) {
		t.Fatalf("shu training records=%#v", trainingRecords)
	}
	trainingRecords, err = store.Training().List(context.Background(), training.Filter{PlayerID: "00000000-0000-0000-0000-000000000031"})
	if err != nil {
		t.Fatal(err)
	}
	if len(trainingRecords) != 2 || trainingRecords[0].TrainingDate() != training.DateFromTime(databaseToday) {
		t.Fatalf("wei training records=%#v", trainingRecords)
	}
	var trainingCount int
	if err := store.pool.QueryRow(context.Background(), `SELECT count(*) FROM training_records WHERE deleted_at IS NULL`).Scan(&trainingCount); err != nil {
		t.Fatal(err)
	}
	noTraining, err := store.Training().List(context.Background(), training.Filter{PlayerID: "00000000-0000-0000-0000-000000000021"})
	if err != nil || len(noTraining) != 0 {
		t.Fatalf("player without training records=%#v error=%v", noTraining, err)
	}
	if trainingCount != 46 {
		t.Fatalf("training record count=%d", trainingCount)
	}
	detail, err := store.GameDetail(context.Background(), "00000000-0000-0000-0000-000000000201")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Lineups) != 4 || len(detail.Plays) != 1 {
		t.Fatalf("game detail=%#v", detail)
	}
	if detail.Match.Status() != game.MatchFinal {
		t.Fatalf("match status=%v", detail.Match.Status())
	}
	lastPlay := detail.Plays[len(detail.Plays)-1]
	if lastPlay.Inning() != 9 || lastPlay.Half() != game.Bottom {
		t.Fatalf("last fixture play=%#v", lastPlay)
	}
	if score := lastPlay.After(); score.HomeScore != 5 || score.AwayScore != 4 {
		t.Fatalf("fixture score=%#v", score)
	}
}

func TestPostgresFixedStoreDoesNotOwnDatabase(t *testing.T) {
	profile := integrationProfile(t)
	ctx := context.Background()
	temporary, err := OpenTemporary(ctx, profile.AdminURL, profile.TemporaryPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := temporary.Close(cleanupCtx); err != nil {
			t.Errorf("cleanup temporary database: %v", err)
		}
	})
	poolConfig, err := pgxpool.ParseConfig(profile.AdminURL)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.Database = temporary.DatabaseName()
	fixed, err := OpenFixed(ctx, poolConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	if err := fixed.Close(ctx); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := temporary.admin.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, temporary.DatabaseName()).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("fixed store close dropped a database it did not own")
	}
}

func seedRoster(t *testing.T, store *Store, clock fixedClock) *roster.Service {
	t.Helper()
	ctx := context.Background()
	teamService, _ := team.NewService(store.Teams(), clock)
	playerService, _ := player.NewService(store.Players(), clock)
	if _, err := teamService.Create(ctx, team.ID("00000000-0000-0000-0000-000000000001"), "Team"); err != nil {
		t.Fatal(err)
	}
	for index, id := range []player.ID{"00000000-0000-0000-0000-000000000011", "00000000-0000-0000-0000-000000000012"} {
		if _, err := playerService.Create(ctx, id, fmt.Sprintf("Player %d", index+1), player.HandLeft, player.HandRight, player.PositionPitcher|player.PositionOutfielder); err != nil {
			t.Fatal(err)
		}
	}
	service, err := roster.NewService(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestPostgresConcurrentJerseyAssignmentAndRead(t *testing.T) {
	store := integrationStore(t)
	clock := fixedClock{now: time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)}
	service := seedRoster(t, store, clock)
	joined, _ := roster.ParseDate("2026-08-01")
	teamID := team.ID("00000000-0000-0000-0000-000000000001")
	players := []player.ID{"00000000-0000-0000-0000-000000000011", "00000000-0000-0000-0000-000000000012"}
	memberships := []roster.ID{"00000000-0000-0000-0000-000000000021", "00000000-0000-0000-0000-000000000022"}
	errorsFound := make([]error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for index := range players {
		go func(index int) {
			defer wait.Done()
			_, errorsFound[index] = service.Assign(context.Background(), memberships[index], teamID, players[index], 18, joined)
		}(index)
	}
	wait.Wait()
	successes, occupied := 0, 0
	for _, err := range errorsFound {
		if err == nil {
			successes++
		} else if errors.Is(err, roster.ErrJerseyOccupied) {
			occupied++
		} else {
			t.Fatalf("unexpected assignment error: %v", err)
		}
	}
	if successes != 1 || occupied != 1 {
		t.Fatalf("successes=%d occupied=%d errors=%v", successes, occupied, errorsFound)
	}
	views, err := store.RosterReader().ListPlayers(context.Background(), roster.PlayerFilter{TeamID: teamID})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Membership.JerseyNumber() != 18 || views[0].Membership.JoinedAt().String() != "2026-08-01" {
		t.Fatalf("views=%#v", views)
	}
}

func TestPostgresRejoinAndAudit(t *testing.T) {
	store := integrationStore(t)
	clock := fixedClock{now: time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC)}
	service := seedRoster(t, store, clock)
	ctx := context.Background()
	joined, _ := roster.ParseDate("2026-08-01")
	teamID := team.ID("00000000-0000-0000-0000-000000000001")
	playerID := player.ID("00000000-0000-0000-0000-000000000011")
	first, err := service.Assign(ctx, "00000000-0000-0000-0000-000000000021", teamID, playerID, 18, joined)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := roster.ParseDate("2026-08-10")
	if _, err := service.Leave(ctx, teamID, first.ID(), left); err != nil {
		t.Fatal(err)
	}
	rejoined, _ := roster.ParseDate("2026-08-10")
	if _, err := service.Assign(ctx, "00000000-0000-0000-0000-000000000022", teamID, playerID, 19, rejoined); err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `INSERT INTO memberships(id,team_id,player_id,jersey_number,joined_at,left_at,version,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000099','00000000-0000-0000-0000-000000000099',$1,1,current_date,NULL,1,now(),now())`, playerID); err != nil {
		t.Fatal(err)
	}
	issues, err := store.AuditRoster(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range issues {
		found = found || issue.Kind == "orphan_team"
	}
	if !found {
		t.Fatalf("issues=%#v", issues)
	}
}

func TestPostgresSchemaHasNoBusinessConstraints(t *testing.T) {
	store := integrationStore(t)
	var count int
	err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM pg_constraint c
JOIN pg_class r ON r.oid=c.conrelid
WHERE r.relname IN ('teams','players','memberships')
  AND c.contype <> 'p'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("found %d non-primary-key constraints", count)
	}
}

func TestPostgresGameCRUDAndExplicitSoftDelete(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{now: now}
	teamService, _ := team.NewService(store.Teams(), clock)
	playerService, _ := player.NewService(store.Players(), clock)
	home := team.ID("00000000-0000-0000-0000-000000000101")
	away := team.ID("00000000-0000-0000-0000-000000000102")
	for _, item := range []struct {
		id   team.ID
		name string
	}{{home, "Home"}, {away, "Away"}} {
		if _, err := teamService.Create(ctx, item.id, item.name); err != nil {
			t.Fatal(err)
		}
	}
	players := []player.ID{"00000000-0000-0000-0000-000000000111", "00000000-0000-0000-0000-000000000112"}
	for i, id := range players {
		if _, err := playerService.Create(ctx, id, fmt.Sprintf("Game Player %d", i+1), player.HandLeft, player.HandRight, player.PositionPitcher|player.PositionOutfielder); err != nil {
			t.Fatal(err)
		}
	}
	service, err := game.NewService(store, clock)
	if err != nil {
		t.Fatal(err)
	}
	match, _ := game.NewMatch("00000000-0000-0000-0000-000000000121", home, away, now.Add(time.Hour), "Field", game.MatchScheduled, now)
	if err = service.CreateMatch(ctx, match); err != nil {
		t.Fatal(err)
	}
	one, _ := game.NewLineupEntry("00000000-0000-0000-0000-000000000131", players[0], 1, player.PositionPitcher)
	lineup, _ := game.NewLineup(match.ID(), home, game.LineupStarter, 0, "Starter", []game.LineupEntry{one}, now)
	if err = service.CreateLineup(ctx, lineup); err != nil {
		t.Fatal(err)
	}
	before, _ := game.NewSituation(0, 0, 0, [3]*player.ID{})
	after, _ := game.NewSituation(1, 0, 0, [3]*player.ID{})
	pitch, _ := game.NewPitch("00000000-0000-0000-0000-000000000151", 1, players[1], players[0], game.PitchInPlay, 0, 0, 0, 0, "", nil, nil, "in play", now)
	play, _ := game.NewPlay(game.PlayDraft{ID: "00000000-0000-0000-0000-000000000141", MatchID: match.ID(), Sequence: 1, Inning: 1, Half: game.Top, BattingOrder: 1, BatterID: players[0], StartingPitcherID: players[1], Before: before, After: after, BattingResult: game.BattingGroundOut, ResultDescription: "Ground out", Pitches: []game.Pitch{pitch}}, now)
	if err = service.CreatePlay(ctx, play); err != nil {
		t.Fatal(err)
	}
	detail, err := store.GameDetail(ctx, match.ID())
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Lineups) != 1 || len(detail.Plays) != 1 {
		t.Fatalf("detail=%#v", detail)
	}
	clock.now = now.Add(2 * time.Hour)
	service, _ = game.NewService(store, clock)
	if err = service.DeleteMatch(ctx, match.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Matches().Get(ctx, match.ID()); !errors.Is(err, game.ErrMatchNotFound) {
		t.Fatalf("deleted match error=%v", err)
	}
	for table, want := range map[string]int{"matches": 1, "lineups": 1, "plays": 1, "pitches": 1} {
		var got int
		if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE deleted_at IS NOT NULL`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s deleted=%d", table, got)
		}
	}
}

func TestPostgresTeamSoftDeleteAlsoDeletesMemberships(t *testing.T) {
	store := integrationStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	service := seedRoster(t, store, fixedClock{now: now})
	joined, _ := roster.ParseDate("2026-08-01")
	teamID := team.ID("00000000-0000-0000-0000-000000000001")
	playerID := player.ID("00000000-0000-0000-0000-000000000011")
	if _, err := service.Assign(ctx, "00000000-0000-0000-0000-000000000021", teamID, playerID, 18, joined); err != nil {
		t.Fatal(err)
	}
	deleteService, _ := roster.NewService(store, fixedClock{now: now.Add(time.Hour)})
	if err := deleteService.DeleteTeam(ctx, teamID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Teams().Get(ctx, teamID); !errors.Is(err, team.ErrNotFound) {
		t.Fatalf("team error=%v", err)
	}
	var teamActive bool
	var deletedMemberships int
	if err := store.pool.QueryRow(ctx, `SELECT active FROM teams WHERE id=$1`, teamID).Scan(&teamActive); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE team_id=$1 AND deleted_at IS NOT NULL`, teamID).Scan(&deletedMemberships); err != nil {
		t.Fatal(err)
	}
	if !teamActive || deletedMemberships != 1 {
		t.Fatalf("active=%v deleted memberships=%d", teamActive, deletedMemberships)
	}
}
