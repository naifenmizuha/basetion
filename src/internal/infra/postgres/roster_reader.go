package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type RosterReader struct{ db dbtx }

func (s *Store) RosterReader() *RosterReader { return &RosterReader{db: s.pool} }

func (r *RosterReader) ListPlayers(ctx context.Context, filter roster.PlayerFilter) ([]roster.Member, error) {
	if filter.TeamID == "" {
		return nil, fmt.Errorf("team id is required")
	}
	var onDate any
	if filter.OnDate != nil {
		onDate = filter.OnDate.Time()
	}
	rows, err := r.db.Query(ctx, `
SELECT m.id::text,m.team_id::text,p.id::text,t.name,t.active,t.version,t.created_at,t.updated_at,
       p.name,p.batting_flags,p.throwing_flags,p.position_flags,p.active,p.version,p.created_at,p.updated_at,
       m.jersey_number,m.joined_at,m.left_at,m.version,m.created_at,m.updated_at
FROM memberships m
JOIN teams t ON t.id=m.team_id
JOIN players p ON p.id=m.player_id
WHERE m.team_id=$1
  AND m.deleted_at IS NULL AND t.deleted_at IS NULL AND p.deleted_at IS NULL
  AND m.joined_at <= COALESCE($2::date,current_date)
  AND (m.left_at IS NULL OR COALESCE($2::date,current_date) < m.left_at)
  AND ($3::smallint=0 OR (p.position_flags & $3)<>0)
ORDER BY m.jersey_number,p.name,p.id`, filter.TeamID, onDate, int16(filter.PositionAny))
	if err != nil {
		return nil, fmt.Errorf("list roster players: %w", err)
	}
	defer rows.Close()
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (roster.Member, error) {
		var value roster.Member
		var membershipID, teamID, playerID, teamName, playerName string
		var teamActive, playerActive bool
		var teamVersion, playerVersion, membershipVersion uint64
		var teamCreated, teamUpdated, playerCreated, playerUpdated, membershipCreated, membershipUpdated time.Time
		var batting, throwing, positions uint8
		var jersey int
		var joined time.Time
		var left *time.Time
		if err := row.Scan(&membershipID, &teamID, &playerID, &teamName, &teamActive, &teamVersion, &teamCreated, &teamUpdated,
			&playerName, &batting, &throwing, &positions, &playerActive, &playerVersion, &playerCreated, &playerUpdated,
			&jersey, &joined, &left, &membershipVersion, &membershipCreated, &membershipUpdated); err != nil {
			return value, err
		}
		var err error
		value.Team, err = team.Restore(team.ID(teamID), teamName, teamActive, teamVersion, teamCreated, teamUpdated)
		if err != nil {
			return value, err
		}
		value.Player, err = player.Restore(player.ID(playerID), playerName, player.HandFlags(batting), player.HandFlags(throwing), player.PositionFlags(positions), playerActive, playerVersion, playerCreated, playerUpdated)
		if err != nil {
			return value, err
		}
		value.Membership, err = roster.Restore(roster.ID(membershipID), team.ID(teamID), player.ID(playerID), jersey, roster.DateFromTime(joined), dateFromTimePointer(left), membershipVersion, membershipCreated, membershipUpdated)
		if err != nil {
			return value, err
		}
		return value, nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan roster players: %w", err)
	}
	return result, nil
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
		{"orphan_team", `SELECT m.team_id::text,m.player_id::text,m.jersey_number,1 FROM memberships m LEFT JOIN teams t ON t.id=m.team_id AND t.deleted_at IS NULL WHERE m.deleted_at IS NULL AND t.id IS NULL`},
		{"orphan_player", `SELECT m.team_id::text,m.player_id::text,m.jersey_number,1 FROM memberships m LEFT JOIN players p ON p.id=m.player_id AND p.deleted_at IS NULL WHERE m.deleted_at IS NULL AND p.id IS NULL`},
		{"duplicate_current_jersey", `SELECT team_id::text,''::text,jersey_number,count(*)::int FROM memberships WHERE left_at IS NULL AND deleted_at IS NULL GROUP BY team_id,jersey_number HAVING count(*)>1`},
		{"overlapping_membership", `SELECT a.team_id::text,a.player_id::text,a.jersey_number,count(*)::int FROM memberships a JOIN memberships b ON a.id<>b.id AND a.team_id=b.team_id AND a.player_id=b.player_id AND a.deleted_at IS NULL AND b.deleted_at IS NULL AND a.joined_at<COALESCE(b.left_at,'infinity'::date) AND b.joined_at<COALESCE(a.left_at,'infinity'::date) GROUP BY a.team_id,a.player_id,a.jersey_number`},
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
