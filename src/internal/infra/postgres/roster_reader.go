package postgres

import (
	"context"
	"fmt"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
)

type PlayerReader struct{ queries *sqlcgen.Queries }

func (s *Store) PlayerReader() *PlayerReader { return &PlayerReader{queries: sqlcgen.New(s.pool)} }
func (r *PlayerReader) List(ctx context.Context, filter player.PlayerFilter) ([]player.PlayerView, error) {
	var jersey *int16
	if filter.JerseyNumber != nil {
		value := int16(*filter.JerseyNumber)
		jersey = &value
	}
	rows, err := r.queries.ListPlayerViews(ctx, sqlcgen.ListPlayerViewsParams{TeamName: filter.TeamName, PositionAny: int16(filter.PositionAny), JerseyNumber: jersey})
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	values := make([]player.PlayerView, 0, len(rows))
	for _, row := range rows {
		values = append(values, player.PlayerView{Name: row.Name, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber), Positions: player.PositionFlags(row.PositionFlags), Active: row.Active})
	}
	return values, nil
}

type AuditIssue struct {
	Kind, TeamID, PlayerID string
	JerseyNumber, Count    int
}

func (s *Store) AuditRoster(ctx context.Context) ([]AuditIssue, error) {
	queries := sqlcgen.New(s.pool)
	orphans, err := queries.AuditOrphanTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit orphan_team: %w", err)
	}
	duplicates, err := queries.AuditDuplicateJerseys(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit duplicate_jersey: %w", err)
	}
	issues := make([]AuditIssue, 0, len(orphans)+len(duplicates))
	for _, row := range orphans {
		issues = append(issues, AuditIssue{Kind: "orphan_team", TeamID: row.TeamID, PlayerID: row.PlayerID, JerseyNumber: int(row.JerseyNumber), Count: int(row.IssueCount)})
	}
	for _, row := range duplicates {
		issues = append(issues, AuditIssue{Kind: "duplicate_jersey", TeamID: row.TeamID, PlayerID: row.PlayerID, JerseyNumber: int(row.JerseyNumber), Count: int(row.IssueCount)})
	}
	return issues, nil
}
