package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/teamops"
)

type RosterReader struct{ db dbtx }

func (s *Store) RosterReader() *RosterReader { return &RosterReader{db: s.pool} }

func (r *RosterReader) ListTeams(ctx context.Context, activeOnly bool) ([]teamops.TeamView, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text,name,active FROM teams WHERE NOT $1 OR active ORDER BY name,id`, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (teamops.TeamView, error) {
		var value teamops.TeamView
		err := row.Scan(&value.ID, &value.Name, &value.Active)
		return value, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan teams: %w", err)
	}
	return result, nil
}

func (r *RosterReader) ListPlayers(ctx context.Context, filter teamops.RosterPlayerFilter) ([]teamops.RosterPlayerView, error) {
	if filter.TeamID == "" {
		return nil, fmt.Errorf("team id is required")
	}
	var onDate any
	if filter.OnDate != nil {
		onDate = filter.OnDate.Time()
	}
	rows, err := r.db.Query(ctx, `
SELECT m.id::text,m.team_id::text,p.id::text,p.name,m.jersey_number,
       p.batting_flags,p.throwing_flags,p.position_flags,m.joined_at,m.left_at,
       (t.active AND p.active AND m.joined_at <= COALESCE($2::date,current_date)
        AND (m.left_at IS NULL OR COALESCE($2::date,current_date) < m.left_at)) AS active
FROM memberships m
JOIN teams t ON t.id=m.team_id
JOIN players p ON p.id=m.player_id
WHERE m.team_id=$1
  AND m.joined_at <= COALESCE($2::date,current_date)
  AND (m.left_at IS NULL OR COALESCE($2::date,current_date) < m.left_at)
  AND ($3::smallint=0 OR (p.position_flags & $3)<>0)
ORDER BY m.jersey_number,p.name,p.id`, filter.TeamID, onDate, int16(filter.PositionAny))
	if err != nil {
		return nil, fmt.Errorf("list roster players: %w", err)
	}
	defer rows.Close()
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (teamops.RosterPlayerView, error) {
		var value teamops.RosterPlayerView
		var batting, throwing, positions uint8
		var joined time.Time
		var left *time.Time
		if err := row.Scan(&value.MembershipID, &value.TeamID, &value.PlayerID, &value.Name, &value.JerseyNumber, &batting, &throwing, &positions, &joined, &left, &value.Active); err != nil {
			return value, err
		}
		if !player.HandFlags(batting).Valid() || !player.HandFlags(throwing).Valid() || !player.PositionFlags(positions).Valid() || value.JerseyNumber < 0 || value.JerseyNumber > 99 {
			return value, fmt.Errorf("corrupted roster player %s", value.PlayerID)
		}
		value.BattingHands = handNames(player.HandFlags(batting))
		value.ThrowingHands = handNames(player.HandFlags(throwing))
		value.Positions = positionFlagNames(player.PositionFlags(positions))
		value.JoinedAt = roster.DateFromTime(joined).String()
		if left != nil {
			if !joined.Before(*left) {
				return value, fmt.Errorf("corrupted membership period %s", value.MembershipID)
			}
			text := roster.DateFromTime(*left).String()
			value.LeftAt = &text
		}
		return value, nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan roster players: %w", err)
	}
	return result, nil
}

func handNames(flags player.HandFlags) []string {
	result := make([]string, 0, 2)
	if flags.Has(player.HandLeft) {
		result = append(result, "left")
	}
	if flags.Has(player.HandRight) {
		result = append(result, "right")
	}
	return result
}

func positionFlagNames(flags player.PositionFlags) []string {
	ordered := []struct {
		name string
		flag player.PositionFlags
	}{{"pitcher", player.PositionPitcher}, {"catcher", player.PositionCatcher}, {"first_base", player.PositionFirstBase}, {"second_base", player.PositionSecondBase}, {"shortstop", player.PositionShortstop}, {"third_base", player.PositionThirdBase}, {"outfielder", player.PositionOutfielder}}
	result := make([]string, 0, 7)
	for _, entry := range ordered {
		if flags.HasAny(entry.flag) {
			result = append(result, entry.name)
		}
	}
	return result
}

type AuditIssue struct {
	Kind         string
	TeamID       string
	PlayerID     string
	JerseyNumber int
	Count        int
}

func (s *Store) AuditRoster(ctx context.Context) ([]AuditIssue, error) {
	queries := []struct{ kind, query string }{
		{"orphan_team", `SELECT m.team_id::text,m.player_id::text,m.jersey_number,1 FROM memberships m LEFT JOIN teams t ON t.id=m.team_id WHERE t.id IS NULL`},
		{"orphan_player", `SELECT m.team_id::text,m.player_id::text,m.jersey_number,1 FROM memberships m LEFT JOIN players p ON p.id=m.player_id WHERE p.id IS NULL`},
		{"duplicate_current_jersey", `SELECT team_id::text,''::text,jersey_number,count(*)::int FROM memberships WHERE left_at IS NULL GROUP BY team_id,jersey_number HAVING count(*)>1`},
		{"overlapping_membership", `SELECT a.team_id::text,a.player_id::text,a.jersey_number,count(*)::int FROM memberships a JOIN memberships b ON a.id<>b.id AND a.team_id=b.team_id AND a.player_id=b.player_id AND a.joined_at<COALESCE(b.left_at,'infinity'::date) AND b.joined_at<COALESCE(a.left_at,'infinity'::date) GROUP BY a.team_id,a.player_id,a.jersey_number`},
	}
	var issues []AuditIssue
	for _, check := range queries {
		rows, err := s.pool.Query(ctx, check.query)
		if err != nil {
			return nil, fmt.Errorf("audit %s: %w", check.kind, err)
		}
		values, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AuditIssue, error) {
			v := AuditIssue{Kind: check.kind}
			err := row.Scan(&v.TeamID, &v.PlayerID, &v.JerseyNumber, &v.Count)
			return v, err
		})
		if err != nil {
			return nil, fmt.Errorf("scan audit %s: %w", check.kind, err)
		}
		issues = append(issues, values...)
	}
	return issues, nil
}
