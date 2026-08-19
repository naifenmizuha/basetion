package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
)

const trainingUniqueConstraint = "uq_training_records_active_player_date"

type TrainingRepository struct {
	queries *sqlcgen.Queries
}

func (r *TrainingRepository) Create(ctx context.Context, value training.Record) error {
	err := r.queries.CreateTrainingRecord(ctx, sqlcgen.CreateTrainingRecordParams{ID: string(value.ID()), PlayerID: string(value.PlayerID()), TrainingDate: date(value.TrainingDate().Time()), Content: value.Content(), Reflection: value.Reflection(), CreatedAt: timestamptz(value.CreatedAt()), UpdatedAt: timestamptz(value.UpdatedAt()), DeletedAt: timestamptzPointer(value.DeletedAt())})
	if trainingDuplicate(err) {
		return training.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("create training record: %w", err)
	}
	return nil
}

func (r *TrainingRepository) Get(ctx context.Context, id training.ID) (training.Record, error) {
	row, err := r.queries.GetTrainingRecord(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return training.Record{}, training.ErrNotFound
	}
	if err != nil {
		return training.Record{}, fmt.Errorf("get training record: %w", err)
	}
	return training.Restore(training.ID(row.ID), player.ID(row.PlayerID), training.DateFromTime(row.TrainingDate.Time), row.Content, row.Reflection, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
}

func (r *TrainingRepository) Update(ctx context.Context, value training.Record) error {
	updated, err := r.queries.UpdateTrainingRecord(ctx, sqlcgen.UpdateTrainingRecordParams{Content: value.Content(), Reflection: value.Reflection(), UpdatedAt: timestamptz(value.UpdatedAt()), DeletedAt: timestamptzPointer(value.DeletedAt()), ID: string(value.ID())})
	if trainingDuplicate(err) {
		return training.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("update training record: %w", err)
	}
	if updated == 0 {
		return training.ErrNotFound
	}
	return nil
}

func (r *TrainingRepository) List(ctx context.Context, filter training.Filter) ([]training.Record, error) {
	rows, err := r.queries.ListTrainingRecords(ctx, sqlcgen.ListTrainingRecordsParams{PlayerID: string(filter.PlayerID), Column2: datePointer(filter.From), Column3: datePointer(filter.To)})
	if err != nil {
		return nil, fmt.Errorf("list training records: %w", err)
	}
	values := make([]training.Record, 0, len(rows))
	for _, row := range rows {
		value, err := training.Restore(training.ID(row.ID), player.ID(row.PlayerID), training.DateFromTime(row.TrainingDate.Time), row.Content, row.Reflection, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if err != nil {
			return nil, fmt.Errorf("restore training record: %w", err)
		}
		values = append(values, value)
	}
	return values, nil
}

func (r *TrainingRepository) SoftDeleteByPlayer(ctx context.Context, id player.ID, now time.Time) error {
	err := r.queries.SoftDeleteTrainingRecordsByPlayer(ctx, sqlcgen.SoftDeleteTrainingRecordsByPlayerParams{DeletedAt: timestamptz(now), PlayerID: string(id)})
	if err != nil {
		return fmt.Errorf("soft delete training records by player: %w", err)
	}
	return nil
}

func trainingDuplicate(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == trainingUniqueConstraint
}

func datePointer(value *training.Date) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return date(value.Time())
}
