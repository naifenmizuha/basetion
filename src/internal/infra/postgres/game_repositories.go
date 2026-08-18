package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type MatchRepository struct{ db dbtx }

func (r *MatchRepository) Create(ctx context.Context, v game.Match) error {
	_, err := r.db.Exec(ctx, `INSERT INTO matches(id,home_team_id,away_team_id,scheduled_at,location,status,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID(), v.HomeTeamID(), v.AwayTeamID(), v.ScheduledAt(), v.Location(), v.Status(), v.Version(), v.CreatedAt(), v.UpdatedAt(), v.DeletedAt())
	return err
}
func (r *MatchRepository) Get(ctx context.Context, id game.MatchID) (game.Match, error) {
	return r.get(ctx, id, false)
}
func (r *MatchRepository) GetForUpdate(ctx context.Context, id game.MatchID) (game.Match, error) {
	return r.get(ctx, id, true)
}
func (r *MatchRepository) get(ctx context.Context, id game.MatchID, lock bool) (game.Match, error) {
	q := `SELECT id::text,home_team_id::text,away_team_id::text,scheduled_at,location,status,version,created_at,updated_at,deleted_at FROM matches WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		q += ` FOR UPDATE`
	}
	var raw, home, away, location string
	var scheduled, created, updated time.Time
	var deleted *time.Time
	var status game.MatchStatus
	var version uint64
	err := r.db.QueryRow(ctx, q, id).Scan(&raw, &home, &away, &scheduled, &location, &status, &version, &created, &updated, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return game.Match{}, game.ErrMatchNotFound
	}
	if err != nil {
		return game.Match{}, err
	}
	return game.RestoreMatch(game.MatchID(raw), team.ID(home), team.ID(away), scheduled, location, status, version, created, updated, deleted)
}
func (r *MatchRepository) Update(ctx context.Context, v game.Match, expected uint64) error {
	tag, err := r.db.Exec(ctx, `UPDATE matches SET home_team_id=$1,away_team_id=$2,scheduled_at=$3,location=$4,status=$5,version=$6,updated_at=$7,deleted_at=$8 WHERE id=$9 AND version=$10 AND deleted_at IS NULL`, v.HomeTeamID(), v.AwayTeamID(), v.ScheduledAt(), v.Location(), v.Status(), v.Version(), v.UpdatedAt(), v.DeletedAt(), v.ID(), expected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return game.ErrMatchVersionConflict
	}
	return nil
}
func (r *MatchRepository) List(ctx context.Context) ([]game.Match, error) {
	rows, err := r.db.Query(ctx, `SELECT id::text,home_team_id::text,away_team_id::text,scheduled_at,location,status,version,created_at,updated_at,deleted_at FROM matches WHERE deleted_at IS NULL ORDER BY scheduled_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (game.Match, error) {
		var id, h, a, l string
		var s, c, u time.Time
		var d *time.Time
		var st game.MatchStatus
		var v uint64
		if e := row.Scan(&id, &h, &a, &s, &l, &st, &v, &c, &u, &d); e != nil {
			return game.Match{}, e
		}
		return game.RestoreMatch(game.MatchID(id), team.ID(h), team.ID(a), s, l, st, v, c, u, d)
	})
}

type LineupRepository struct{ db dbtx }

func (r *LineupRepository) Create(ctx context.Context, v game.Lineup) error { return r.insert(ctx, v) }
func (r *LineupRepository) insert(ctx context.Context, v game.Lineup) error {
	for _, x := range v.Entries() {
		_, e := r.db.Exec(ctx, `INSERT INTO lineups(id,match_id,team_id,kind,variant_number,variant_name,player_id,batting_order,position,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, x.ID(), v.MatchID(), v.TeamID(), v.Kind(), v.VariantNumber(), v.VariantName(), x.PlayerID(), x.BattingOrder(), x.Position(), v.Version(), v.CreatedAt(), v.UpdatedAt(), v.DeletedAt())
		if e != nil {
			return e
		}
	}
	return nil
}
func (r *LineupRepository) Get(ctx context.Context, m game.MatchID, t team.ID, k game.LineupKind, n uint16) (game.Lineup, error) {
	return r.get(ctx, m, t, k, n, false)
}
func (r *LineupRepository) GetForUpdate(ctx context.Context, m game.MatchID, t team.ID, k game.LineupKind, n uint16) (game.Lineup, error) {
	return r.get(ctx, m, t, k, n, true)
}
func (r *LineupRepository) get(ctx context.Context, m game.MatchID, t team.ID, k game.LineupKind, n uint16, lock bool) (game.Lineup, error) {
	q := `SELECT id::text,player_id::text,batting_order,position,variant_name,version,created_at,updated_at,deleted_at FROM lineups WHERE match_id=$1 AND team_id=$2 AND kind=$3 AND variant_number=$4 AND deleted_at IS NULL ORDER BY batting_order,id`
	if lock {
		q += ` FOR UPDATE`
	}
	rows, e := r.db.Query(ctx, q, m, t, k, n)
	if e != nil {
		return game.Lineup{}, e
	}
	defer rows.Close()
	var entries []game.LineupEntry
	var name string
	var version uint64
	var created, updated time.Time
	var deleted *time.Time
	for rows.Next() {
		var id, pid, rn string
		var order, pos int
		var rv uint64
		var rc, ru time.Time
		var rd *time.Time
		if e = rows.Scan(&id, &pid, &order, &pos, &rn, &rv, &rc, &ru, &rd); e != nil {
			return game.Lineup{}, e
		}
		x, err := game.NewLineupEntry(game.LineupID(id), player.ID(pid), order, player.PositionFlags(pos))
		if err != nil {
			return game.Lineup{}, err
		}
		if len(entries) == 0 {
			name, version, created, updated, deleted = rn, rv, rc, ru, rd
		}
		entries = append(entries, x)
	}
	if len(entries) == 0 {
		return game.Lineup{}, game.ErrLineupNotFound
	}
	return game.RestoreLineup(m, t, k, int(n), name, entries, version, created, updated, deleted)
}
func (r *LineupRepository) Replace(ctx context.Context, v game.Lineup, expected uint64, now time.Time) error {
	tag, e := r.db.Exec(ctx, `UPDATE lineups SET deleted_at=$1,updated_at=$1,version=version+1 WHERE match_id=$2 AND team_id=$3 AND kind=$4 AND variant_number=$5 AND version=$6 AND deleted_at IS NULL`, now, v.MatchID(), v.TeamID(), v.Kind(), v.VariantNumber(), expected)
	if e != nil {
		return fmt.Errorf("replace lineup: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return game.ErrLineupVersionConflict
	}
	return r.insert(ctx, v)
}
func (r *LineupRepository) SoftDelete(ctx context.Context, m game.MatchID, t team.ID, k game.LineupKind, n uint16, expected uint64, now time.Time) error {
	tag, e := r.db.Exec(ctx, `UPDATE lineups SET deleted_at=$1,updated_at=$1,version=version+1 WHERE match_id=$2 AND team_id=$3 AND kind=$4 AND variant_number=$5 AND version=$6 AND deleted_at IS NULL`, now, m, t, k, n, expected)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return game.ErrLineupVersionConflict
	}
	return nil
}
func (r *LineupRepository) SoftDeleteByMatch(ctx context.Context, m game.MatchID, now time.Time) error {
	_, e := r.db.Exec(ctx, `UPDATE lineups SET deleted_at=$1,updated_at=$1,version=version+1 WHERE match_id=$2 AND deleted_at IS NULL`, now, m)
	return e
}
func (r *LineupRepository) ListByMatch(ctx context.Context, m game.MatchID) ([]game.Lineup, error) {
	rows, e := r.db.Query(ctx, `SELECT DISTINCT team_id::text,kind,variant_number FROM lineups WHERE match_id=$1 AND deleted_at IS NULL ORDER BY 1,2,3`, m)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	type key struct {
		t string
		k game.LineupKind
		n uint16
	}
	var keys []key
	for rows.Next() {
		var x key
		if e = rows.Scan(&x.t, &x.k, &x.n); e != nil {
			return nil, e
		}
		keys = append(keys, x)
	}
	var out []game.Lineup
	for _, x := range keys {
		v, err := r.Get(ctx, m, team.ID(x.t), x.k, x.n)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
func (r *LineupRepository) NameOccupied(ctx context.Context, m game.MatchID, t team.ID, name string, k game.LineupKind, n uint16) (bool, error) {
	var v bool
	e := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lineups WHERE match_id=$1 AND team_id=$2 AND variant_name=$3 AND deleted_at IS NULL AND NOT(kind=$4 AND variant_number=$5))`, m, t, name, k, n).Scan(&v)
	return v, e
}

func (s *Store) ListMatches(ctx context.Context) ([]game.Match, error) { return s.Matches().List(ctx) }
func (s *Store) GetMatch(ctx context.Context, id game.MatchID) (game.Match, error) {
	return s.Matches().Get(ctx, id)
}
func (s *Store) ListLineups(ctx context.Context, id game.MatchID) ([]game.Lineup, error) {
	return s.Lineups().ListByMatch(ctx, id)
}
func (s *Store) ListPlays(ctx context.Context, id game.MatchID) ([]game.Play, error) {
	return s.Plays().ListByMatch(ctx, id)
}
func (s *Store) Matches() *MatchRepository  { return &MatchRepository{db: s.pool} }
func (s *Store) Lineups() *LineupRepository { return &LineupRepository{db: s.pool} }
func (s *Store) Plays() *PlayRepository     { return &PlayRepository{db: s.pool} }
func (s *Store) GameDetail(ctx context.Context, id game.MatchID) (game.Detail, error) {
	m, e := s.Matches().Get(ctx, id)
	if e != nil {
		return game.Detail{}, e
	}
	ls, e := s.Lineups().ListByMatch(ctx, id)
	if e != nil {
		return game.Detail{}, e
	}
	ps, e := s.Plays().ListByMatch(ctx, id)
	if e != nil {
		return game.Detail{}, e
	}
	return game.Detail{Match: m, Lineups: ls, Plays: ps}, nil
}
