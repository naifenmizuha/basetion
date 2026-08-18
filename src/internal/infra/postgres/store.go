package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	domainroster "github.com/naifenmizuha/basetion/src/internal/domain/roster"
)
type Store struct{ pool *pgxpool.Pool }

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

func (s *Store) WithinGameTransaction(ctx context.Context, fn func(game.Repositories) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin game transaction: %w", err)
	}
	repositories := game.Repositories{
		Matches: &MatchRepository{db: tx}, Lineups: &LineupRepository{db: tx}, Plays: &PlayRepository{db: tx},
		Teams: &TeamRepository{db: tx}, Players: &PlayerRepository{db: tx},
	}
	if err := fn(repositories); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit game transaction: %w", err)
	}
	return nil
}

func (s *Store) Close() { s.pool.Close() }
func (s *Store) WithinTransaction(ctx context.Context, fn func(domainroster.Repositories) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin roster transaction: %w", err)
	}
	repositories := domainroster.Repositories{
		Teams:       &TeamRepository{db: tx},
		Players:     &PlayerRepository{db: tx},
		Memberships: &membershipRepository{db: tx},
		Training:    &TrainingRepository{db: tx},
	}
	if err := fn(repositories); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit roster transaction: %w", err)
	}
	return nil
}

func (s *Store) Players() *PlayerRepository { return &PlayerRepository{db: s.pool} }
func (s *Store) Teams() *TeamRepository     { return &TeamRepository{db: s.pool} }
func (s *Store) Training() *TrainingRepository {
	return &TrainingRepository{db: s.pool}
}
