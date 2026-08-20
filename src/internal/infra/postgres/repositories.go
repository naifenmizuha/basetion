package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
	"time"
)

type TeamRepository struct {
	queries *sqlcgen.Queries
}

func (r *TeamRepository) Create(ctx context.Context, value team.Team) error {
	err := r.queries.CreateTeam(ctx, sqlcgen.CreateTeamParams{ID: string(value.ID()), Name: value.Name(), Active: value.Active(), CreatedAt: timestamptz(value.CreatedAt()), UpdatedAt: timestamptz(value.UpdatedAt()), DeletedAt: timestamptzPointer(value.DeletedAt())})
	if err != nil {
		return mapTeamNameError(err, "create team")
	}
	return nil
}
func (r *TeamRepository) GetByNames(ctx context.Context, names []string) ([]team.Team, error) {
	rows, err := r.queries.GetTeamsByNames(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("get teams by names: %w", err)
	}
	result := make([]team.Team, 0, len(rows))
	for _, row := range rows {
		value, restoreErr := team.RestoreDeleted(team.ID(row.ID), row.Name, row.Active, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if restoreErr != nil {
			return nil, restoreErr
		}
		result = append(result, value)
	}
	return result, nil
}
func (r *TeamRepository) Get(ctx context.Context, id team.ID) (team.Team, error) {
	row, err := r.queries.GetTeam(ctx, string(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return team.Team{}, team.ErrNotFound
		}
		return team.Team{}, fmt.Errorf("get team: %w", err)
	}
	return team.RestoreDeleted(team.ID(row.ID), row.Name, row.Active, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
}

func (r *TeamRepository) Update(ctx context.Context, value team.Team) error {
	updated, err := r.queries.UpdateTeam(ctx, sqlcgen.UpdateTeamParams{Name: value.Name(), Active: value.Active(), UpdatedAt: timestamptz(value.UpdatedAt()), DeletedAt: timestamptzPointer(value.DeletedAt()), ID: string(value.ID())})
	if err != nil {
		return fmt.Errorf("update team: %w", err)
	}
	if updated == 0 {
		return team.ErrNotFound
	}
	return nil
}
func (r *TeamRepository) List(ctx context.Context, activeOnly bool) ([]team.Team, error) {
	rows, err := r.queries.ListTeams(ctx, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	values := make([]team.Team, 0, len(rows))
	for _, row := range rows {
		value, err := team.RestoreDeleted(team.ID(row.ID), row.Name, row.Active, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if err != nil {
			return nil, fmt.Errorf("restore team: %w", err)
		}
		values = append(values, value)
	}
	return values, nil
}

type PlayerRepository struct {
	queries *sqlcgen.Queries
}

func (r *PlayerRepository) Create(ctx context.Context, value player.Player) error {
	err := r.queries.CreatePlayer(ctx, sqlcgen.CreatePlayerParams{ID: string(value.ID()), TeamID: string(value.TeamID()), JerseyNumber: int16(value.JerseyNumber()), Name: value.Name(), BattingFlags: int16(value.Batting()), ThrowingFlags: int16(value.Throwing()), PositionFlags: int16(value.Positions()), Active: value.Active(), CreatedAt: timestamptz(value.CreatedAt()), UpdatedAt: timestamptz(value.UpdatedAt()), DeletedAt: timestamptzPointer(value.DeletedAt())})
	if err != nil {
		return mapJerseyError(err, "create player")
	}
	return nil
}
func (r *PlayerRepository) Get(ctx context.Context, id player.ID) (player.Player, error) {
	row, err := r.queries.GetPlayer(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return player.Player{}, player.ErrNotFound
	}
	if err != nil {
		return player.Player{}, fmt.Errorf("get player: %w", err)
	}
	return player.RestoreDeleted(player.ID(row.ID), team.ID(row.TeamID), int(row.JerseyNumber), row.Name, player.HandFlags(row.BattingFlags), player.HandFlags(row.ThrowingFlags), player.PositionFlags(row.PositionFlags), row.Active, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
}
func (r *PlayerRepository) GetByTeamAndJerseys(ctx context.Context, keys []game.PlayerJerseyKey) ([]player.Player, error) {
	encoded := make([]string, len(keys))
	for index, key := range keys {
		encoded[index] = fmt.Sprintf("%s/%d", key.TeamID, key.JerseyNumber)
	}
	rows, err := r.queries.GetPlayersByTeamAndJerseys(ctx, encoded)
	if err != nil {
		return nil, fmt.Errorf("get players by team and jerseys: %w", err)
	}
	result := make([]player.Player, 0, len(rows))
	for _, row := range rows {
		value, restoreErr := player.RestoreDeleted(player.ID(row.ID), team.ID(row.TeamID), int(row.JerseyNumber), row.Name, player.HandFlags(row.BattingFlags), player.HandFlags(row.ThrowingFlags), player.PositionFlags(row.PositionFlags), row.Active, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if restoreErr != nil {
			return nil, restoreErr
		}
		result = append(result, value)
	}
	return result, nil
}

func (r *PlayerRepository) Update(ctx context.Context, value player.Player) error {
	updated, err := r.queries.UpdatePlayer(ctx, sqlcgen.UpdatePlayerParams{JerseyNumber: int16(value.JerseyNumber()), Name: value.Name(), BattingFlags: int16(value.Batting()), ThrowingFlags: int16(value.Throwing()), PositionFlags: int16(value.Positions()), Active: value.Active(), UpdatedAt: timestamptz(value.UpdatedAt()), DeletedAt: timestamptzPointer(value.DeletedAt()), ID: string(value.ID())})
	if err != nil {
		return mapJerseyError(err, "update player")
	}
	if updated == 0 {
		return player.ErrNotFound
	}
	return nil
}
func (r *PlayerRepository) JerseyOccupied(ctx context.Context, teamID team.ID, jersey int, except *player.ID) (bool, error) {
	var excluded *string
	if except != nil {
		value := string(*except)
		excluded = &value
	}
	occupied, err := r.queries.PlayerJerseyOccupied(ctx, sqlcgen.PlayerJerseyOccupiedParams{TeamID: string(teamID), JerseyNumber: int16(jersey), ExceptPlayerID: uuidArgument(excluded)})
	if err != nil {
		return false, fmt.Errorf("check jersey occupancy: %w", err)
	}
	return occupied, nil
}
func (r *PlayerRepository) SoftDeleteByTeam(ctx context.Context, id team.ID, now time.Time) error {
	err := r.queries.SoftDeletePlayersByTeam(ctx, sqlcgen.SoftDeletePlayersByTeamParams{DeletedAt: timestamptz(now), TeamID: string(id)})
	if err != nil {
		return fmt.Errorf("soft delete players by team: %w", err)
	}
	return nil
}
func mapJerseyError(err error, operation string) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" && databaseError.ConstraintName == "uq_players_active_team_jersey" {
		return player.ErrJerseyOccupied
	}
	return fmt.Errorf("%s: %w", operation, err)
}
func mapTeamNameError(err error, operation string) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" && databaseError.ConstraintName == "uq_teams_active_name" {
		return team.ErrNameOccupied
	}
	return fmt.Errorf("%s: %w", operation, err)
}
