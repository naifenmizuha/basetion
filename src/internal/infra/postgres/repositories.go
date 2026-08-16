package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type dbtx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type TeamRepository struct{ db dbtx }

func (r *TeamRepository) Create(ctx context.Context, value team.Team) error {
	_, err := r.db.Exec(ctx, `INSERT INTO teams(id,name,active,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, value.ID(), value.Name(), value.Active(), value.Version(), value.CreatedAt(), value.UpdatedAt(), value.DeletedAt())
	if err != nil {
		return fmt.Errorf("create team: %w", err)
	}
	return nil
}

func (r *TeamRepository) Get(ctx context.Context, id team.ID) (team.Team, error) {
	return r.get(ctx, id, false)
}
func (r *TeamRepository) GetForUpdate(ctx context.Context, id team.ID) (team.Team, error) {
	return r.get(ctx, id, true)
}

func (r *TeamRepository) get(ctx context.Context, id team.ID, lock bool) (team.Team, error) {
	query := `SELECT id::text,name,active,version,created_at,updated_at,deleted_at FROM teams WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		query += ` FOR UPDATE`
	}
	var rawID, name string
	var active bool
	var version uint64
	var createdAt, updatedAt time.Time
	var deletedAt *time.Time
	if err := r.db.QueryRow(ctx, query, id).Scan(&rawID, &name, &active, &version, &createdAt, &updatedAt, &deletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return team.Team{}, team.ErrNotFound
		}
		return team.Team{}, fmt.Errorf("get team: %w", err)
	}
	return team.RestoreDeleted(team.ID(rawID), name, active, version, createdAt, updatedAt, deletedAt)
}

func (r *TeamRepository) Update(ctx context.Context, value team.Team, expected uint64) error {
	tag, err := r.db.Exec(ctx, `UPDATE teams SET name=$1,active=$2,version=$3,updated_at=$4,deleted_at=$5 WHERE id=$6 AND version=$7 AND deleted_at IS NULL`, value.Name(), value.Active(), value.Version(), value.UpdatedAt(), value.DeletedAt(), value.ID(), expected)
	if err != nil {
		return fmt.Errorf("update team: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return team.ErrVersionConflict
	}
	return nil
}

func (r *TeamRepository) List(ctx context.Context, activeOnly bool) ([]team.Team, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text,name,active,version,created_at,updated_at,deleted_at FROM teams WHERE deleted_at IS NULL AND (NOT $1 OR active) ORDER BY name,id`, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()
	values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (team.Team, error) {
		var id, name string
		var active bool
		var version uint64
		var created, updated time.Time
		var deleted *time.Time
		if err := row.Scan(&id, &name, &active, &version, &created, &updated, &deleted); err != nil {
			return team.Team{}, err
		}
		return team.RestoreDeleted(team.ID(id), name, active, version, created, updated, deleted)
	})
	if err != nil {
		return nil, fmt.Errorf("scan teams: %w", err)
	}
	return values, nil
}

type PlayerRepository struct{ db dbtx }

func (r *PlayerRepository) Create(ctx context.Context, value player.Player) error {
	_, err := r.db.Exec(ctx, `INSERT INTO players(id,name,batting_flags,throwing_flags,position_flags,active,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, value.ID(), value.Name(), value.Batting(), value.Throwing(), value.Positions(), value.Active(), value.Version(), value.CreatedAt(), value.UpdatedAt(), value.DeletedAt())
	if err != nil {
		return fmt.Errorf("create player: %w", err)
	}
	return nil
}

func (r *PlayerRepository) Get(ctx context.Context, id player.ID) (player.Player, error) {
	return r.get(ctx, id, false)
}
func (r *PlayerRepository) GetForUpdate(ctx context.Context, id player.ID) (player.Player, error) {
	return r.get(ctx, id, true)
}
func (r *PlayerRepository) get(ctx context.Context, id player.ID, lock bool) (player.Player, error) {
	query := `SELECT id::text,name,batting_flags,throwing_flags,position_flags,active,version,created_at,updated_at,deleted_at FROM players WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		query += ` FOR UPDATE`
	}
	var rawID, name string
	var batting, throwing, positions uint8
	var active bool
	var version uint64
	var createdAt, updatedAt time.Time
	var deletedAt *time.Time
	err := r.db.QueryRow(ctx, query, id).Scan(&rawID, &name, &batting, &throwing, &positions, &active, &version, &createdAt, &updatedAt, &deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return player.Player{}, player.ErrNotFound
	}
	if err != nil {
		return player.Player{}, fmt.Errorf("get player: %w", err)
	}
	return player.RestoreDeleted(player.ID(rawID), name, player.HandFlags(batting), player.HandFlags(throwing), player.PositionFlags(positions), active, version, createdAt, updatedAt, deletedAt)
}

func (r *PlayerRepository) Update(ctx context.Context, value player.Player, expected uint64) error {
	tag, err := r.db.Exec(ctx, `UPDATE players SET name=$1,batting_flags=$2,throwing_flags=$3,position_flags=$4,active=$5,version=$6,updated_at=$7,deleted_at=$8 WHERE id=$9 AND version=$10 AND deleted_at IS NULL`, value.Name(), value.Batting(), value.Throwing(), value.Positions(), value.Active(), value.Version(), value.UpdatedAt(), value.DeletedAt(), value.ID(), expected)
	if err != nil {
		return fmt.Errorf("update player: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return player.ErrVersionConflict
	}
	return nil
}

type membershipRepository struct{ db dbtx }

func (r *membershipRepository) Create(ctx context.Context, value roster.Membership) error {
	_, err := r.db.Exec(ctx, `INSERT INTO memberships(id,team_id,player_id,jersey_number,joined_at,left_at,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, value.ID(), value.TeamID(), value.PlayerID(), value.JerseyNumber(), value.JoinedAt().Time(), datePointerTime(value.LeftAt()), value.Version(), value.CreatedAt(), value.UpdatedAt(), value.DeletedAt())
	if err != nil {
		return fmt.Errorf("create membership: %w", err)
	}
	return nil
}

func (r *membershipRepository) Get(ctx context.Context, id roster.ID) (roster.Membership, error) {
	return r.get(ctx, id, false)
}
func (r *membershipRepository) GetForUpdate(ctx context.Context, id roster.ID) (roster.Membership, error) {
	return r.get(ctx, id, true)
}

func (r *membershipRepository) get(ctx context.Context, id roster.ID, lock bool) (roster.Membership, error) {
	query := `SELECT id::text,team_id::text,player_id::text,jersey_number,joined_at,left_at,version,created_at,updated_at,deleted_at FROM memberships WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		query += ` FOR UPDATE`
	}
	var rawID, teamID, playerID string
	var jersey int
	var joinedAt time.Time
	var leftAt *time.Time
	var version uint64
	var createdAt, updatedAt time.Time
	var deletedAt *time.Time
	err := r.db.QueryRow(ctx, query, id).Scan(&rawID, &teamID, &playerID, &jersey, &joinedAt, &leftAt, &version, &createdAt, &updatedAt, &deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return roster.Membership{}, roster.ErrNotFound
	}
	if err != nil {
		return roster.Membership{}, fmt.Errorf("get membership: %w", err)
	}
	joined := roster.DateFromTime(joinedAt)
	left := dateFromTimePointer(leftAt)
	return roster.RestoreDeleted(roster.ID(rawID), team.ID(teamID), player.ID(playerID), jersey, joined, left, version, createdAt, updatedAt, deletedAt)
}

func (r *membershipRepository) Update(ctx context.Context, value roster.Membership, expected uint64) error {
	tag, err := r.db.Exec(ctx, `UPDATE memberships SET jersey_number=$1,left_at=$2,version=$3,updated_at=$4,deleted_at=$5 WHERE id=$6 AND version=$7 AND deleted_at IS NULL`, value.JerseyNumber(), datePointerTime(value.LeftAt()), value.Version(), value.UpdatedAt(), value.DeletedAt(), value.ID(), expected)
	if err != nil {
		return fmt.Errorf("update membership: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return roster.ErrVersionConflict
	}
	return nil
}

func (r *membershipRepository) CurrentByTeamAndPlayer(ctx context.Context, teamID team.ID, playerID player.ID) (*roster.Membership, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id::text FROM memberships WHERE team_id=$1 AND player_id=$2 AND left_at IS NULL AND deleted_at IS NULL LIMIT 1`, teamID, playerID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find current membership: %w", err)
	}
	value, err := r.Get(ctx, roster.ID(id))
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *membershipRepository) JerseyOccupied(ctx context.Context, teamID team.ID, jersey int, except *roster.ID) (bool, error) {
	var occupied bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships WHERE team_id=$1 AND jersey_number=$2 AND left_at IS NULL AND deleted_at IS NULL AND ($3::uuid IS NULL OR id<>$3))`, teamID, jersey, exceptID(except)).Scan(&occupied)
	if err != nil {
		return false, fmt.Errorf("check jersey occupancy: %w", err)
	}
	return occupied, nil
}

func (r *membershipRepository) Overlaps(ctx context.Context, teamID team.ID, playerID player.ID, joined roster.Date, left *roster.Date, except *roster.ID) (bool, error) {
	var overlaps bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships WHERE team_id=$1 AND player_id=$2 AND joined_at < COALESCE($4::date,'infinity'::date) AND $3::date < COALESCE(left_at,'infinity'::date) AND deleted_at IS NULL AND ($5::uuid IS NULL OR id<>$5))`, teamID, playerID, joined.Time(), datePointerTime(left), exceptID(except)).Scan(&overlaps)
	if err != nil {
		return false, fmt.Errorf("check membership overlap: %w", err)
	}
	return overlaps, nil
}

func (r *membershipRepository) SoftDeleteByTeam(ctx context.Context, id team.ID, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE memberships SET deleted_at=$1,updated_at=$1,version=version+1 WHERE team_id=$2 AND deleted_at IS NULL`, now, id)
	if err != nil {
		return fmt.Errorf("soft delete memberships by team: %w", err)
	}
	return nil
}
func (r *membershipRepository) SoftDeleteByPlayer(ctx context.Context, id player.ID, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE memberships SET deleted_at=$1,updated_at=$1,version=version+1 WHERE player_id=$2 AND deleted_at IS NULL`, now, id)
	if err != nil {
		return fmt.Errorf("soft delete memberships by player: %w", err)
	}
	return nil
}

func datePointerTime(value *roster.Date) *time.Time {
	if value == nil {
		return nil
	}
	result := value.Time()
	return &result
}
func dateFromTimePointer(value *time.Time) *roster.Date {
	if value == nil {
		return nil
	}
	result := roster.DateFromTime(*value)
	return &result
}
func exceptID(value *roster.ID) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
