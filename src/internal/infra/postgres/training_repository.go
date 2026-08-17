package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

const trainingUniqueConstraint = "uq_training_records_active_player_date"

type TrainingRepository struct{ db dbtx }

func (r *TrainingRepository) Create(ctx context.Context, value training.Record) error {
	_, err := r.db.Exec(ctx, `INSERT INTO training_records(id,player_id,training_date,content,reflection,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, value.ID(), value.PlayerID(), value.TrainingDate().Time(), value.Content(), value.Reflection(), value.Version(), value.CreatedAt(), value.UpdatedAt(), value.DeletedAt())
	if trainingDuplicate(err) {
		return training.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("create training record: %w", err)
	}
	return nil
}

func (r *TrainingRepository) Get(ctx context.Context, id training.ID) (training.Record, error) {
	var rawID, playerID, content, reflection string
	var date, created, updated time.Time
	var version uint64
	var deleted *time.Time
	err := r.db.QueryRow(ctx, `SELECT id::text,player_id::text,training_date,content,reflection,version,created_at,updated_at,deleted_at FROM training_records WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&rawID, &playerID, &date, &content, &reflection, &version, &created, &updated, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Record{}, training.ErrNotFound
	}
	if err != nil {
		return training.Record{}, fmt.Errorf("get training record: %w", err)
	}
	return training.Restore(training.ID(rawID), player.ID(playerID), training.DateFromTime(date), content, reflection, version, created, updated, deleted)
}

func (r *TrainingRepository) Update(ctx context.Context, value training.Record, expected uint64) error {
	tag, err := r.db.Exec(ctx, `UPDATE training_records SET content=$1,reflection=$2,version=$3,updated_at=$4,deleted_at=$5 WHERE id=$6 AND version=$7 AND deleted_at IS NULL`, value.Content(), value.Reflection(), value.Version(), value.UpdatedAt(), value.DeletedAt(), value.ID(), expected)
	if trainingDuplicate(err) {
		return training.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("update training record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return training.ErrVersionConflict
	}
	return nil
}

func (r *TrainingRepository) List(ctx context.Context, filter training.Filter) ([]training.Record, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text,player_id::text,training_date,content,reflection,version,created_at,updated_at,deleted_at FROM training_records WHERE player_id=$1 AND deleted_at IS NULL AND ($2::date IS NULL OR training_date >= $2) AND ($3::date IS NULL OR training_date <= $3) ORDER BY training_date DESC,id`, filter.PlayerID, trainingDateArgument(filter.From), trainingDateArgument(filter.To))
	if err != nil {
		return nil, fmt.Errorf("list training records: %w", err)
	}
	defer rows.Close()
	values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (training.Record, error) {
		var id, playerID, content, reflection string
		var date, created, updated time.Time
		var version uint64
		var deleted *time.Time
		if err := row.Scan(&id, &playerID, &date, &content, &reflection, &version, &created, &updated, &deleted); err != nil {
			return training.Record{}, err
		}
		return training.Restore(training.ID(id), player.ID(playerID), training.DateFromTime(date), content, reflection, version, created, updated, deleted)
	})
	if err != nil {
		return nil, fmt.Errorf("scan training records: %w", err)
	}
	return values, nil
}

func (r *TrainingRepository) SoftDeleteByPlayer(ctx context.Context, id player.ID, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE training_records SET deleted_at=$1,updated_at=$1,version=version+1 WHERE player_id=$2 AND deleted_at IS NULL`, now, id)
	if err != nil {
		return fmt.Errorf("soft delete training records by player: %w", err)
	}
	return nil
}

func trainingDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == trainingUniqueConstraint
}

func trainingDateArgument(value *training.Date) any {
	if value == nil {
		return nil
	}
	return value.Time()
}
