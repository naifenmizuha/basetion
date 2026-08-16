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
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
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

func TestPostgresDevelopmentFixtures(t *testing.T) {
	store := integrationStoreWithConfig(t, integrationProfile(t), WithDevelopmentFixtures())
	teams, err := store.RosterReader().ListTeams(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].Name != "季汉队" {
		t.Fatalf("teams=%#v", teams)
	}
	players, err := store.RosterReader().ListPlayers(context.Background(), teamquery.RosterPlayerFilter{TeamID: teams[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 20 || players[0].Name != "张飞" || players[19].Name != "庞统" {
		t.Fatalf("players=%#v", players)
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
	views, err := store.RosterReader().ListPlayers(context.Background(), teamquery.RosterPlayerFilter{TeamID: string(teamID)})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].JerseyNumber != 18 || views[0].JoinedAt != "2026-08-01" {
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
