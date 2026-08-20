package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, errors.New("database URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("configure PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	return &Store{pool: pool}, nil
}
func (s *Store) Close() { s.pool.Close() }

func (s *Store) queryExecutor() *sqlcgen.Queries {
	if s.queries != nil {
		return s.queries
	}
	return sqlcgen.New(s.pool)
}
func (s *Store) WithinTransaction(ctx context.Context, fn func(player.Repositories) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin player transaction: %w", err)
	}
	queries := sqlcgen.New(tx)
	repositories := player.Repositories{Teams: &TeamRepository{queries: queries}, Players: &PlayerRepository{queries: queries}, Training: &TrainingRepository{queries: queries}}
	if err := fn(repositories); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit player transaction: %w", err)
	}
	return nil
}
func (s *Store) WithinGameTransaction(ctx context.Context, fn func(game.Repositories) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin game transaction: %w", err)
	}
	queries := sqlcgen.New(tx)
	repositories := game.Repositories{Matches: &MatchRepository{queries: queries}, Lineups: &LineupRepository{queries: queries}, Plays: &PlayRepository{queries: queries}, Teams: &TeamRepository{queries: queries}, Players: &PlayerRepository{queries: queries}}
	if err := fn(repositories); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit game transaction: %w", err)
	}
	return nil
}
func (s *Store) Players() *PlayerRepository {
	return &PlayerRepository{queries: sqlcgen.New(s.pool)}
}
func (s *Store) Teams() *TeamRepository {
	return &TeamRepository{queries: sqlcgen.New(s.pool)}
}
func (s *Store) Training() *TrainingRepository {
	return &TrainingRepository{queries: sqlcgen.New(s.pool)}
}
func (s *Store) DirectGameRepositories() game.DirectRepositories {
	queries := sqlcgen.New(s.pool)
	return game.DirectRepositories{
		Matches: &MatchRepository{queries: queries}, Lineups: &LineupRepository{queries: queries},
		Plays: &PlayRepository{queries: queries}, Teams: &TeamRepository{queries: queries},
		Players: &PlayerRepository{queries: queries},
	}
}
