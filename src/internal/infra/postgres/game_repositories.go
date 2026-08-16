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
	_, e := r.db.Exec(ctx, `INSERT INTO matches(id,home_team_id,away_team_id,scheduled_at,location,status,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID(), v.HomeTeamID(), v.AwayTeamID(), v.ScheduledAt(), v.Location(), v.Status(), v.Version(), v.CreatedAt(), v.UpdatedAt(), v.DeletedAt())
	if e != nil {
		return fmt.Errorf("create match: %w", e)
	}
	return nil
}
func (r *MatchRepository) Get(ctx context.Context, id game.MatchID) (game.Match, error) {
	return r.get(ctx, id, false)
}
func (r *MatchRepository) GetForUpdate(ctx context.Context, id game.MatchID) (game.Match, error) {
	return r.get(ctx, id, true)
}
func (r *MatchRepository) get(ctx context.Context, id game.MatchID, lock bool) (game.Match, error) {
	q := `SELECT m.id::text,m.home_team_id::text,m.away_team_id::text,m.scheduled_at,m.location,m.status,m.version,m.created_at,m.updated_at,m.deleted_at FROM matches m JOIN teams h ON h.id=m.home_team_id AND h.deleted_at IS NULL JOIN teams a ON a.id=m.away_team_id AND a.deleted_at IS NULL WHERE m.id=$1 AND m.deleted_at IS NULL`
	if lock {
		q += ` FOR UPDATE OF m`
	}
	var raw, home, away, location string
	var scheduled, created, updated time.Time
	var deleted *time.Time
	var version uint64
	var status game.MatchStatus
	e := r.db.QueryRow(ctx, q, id).Scan(&raw, &home, &away, &scheduled, &location, &status, &version, &created, &updated, &deleted)
	if errors.Is(e, pgx.ErrNoRows) {
		return game.Match{}, game.ErrMatchNotFound
	}
	if e != nil {
		return game.Match{}, fmt.Errorf("get match: %w", e)
	}
	return game.RestoreMatch(game.MatchID(raw), team.ID(home), team.ID(away), scheduled, location, status, version, created, updated, deleted)
}
func (r *MatchRepository) Update(ctx context.Context, v game.Match, expected uint64) error {
	tag, e := r.db.Exec(ctx, `UPDATE matches SET home_team_id=$1,away_team_id=$2,scheduled_at=$3,location=$4,status=$5,version=$6,updated_at=$7,deleted_at=$8 WHERE id=$9 AND version=$10 AND deleted_at IS NULL`, v.HomeTeamID(), v.AwayTeamID(), v.ScheduledAt(), v.Location(), v.Status(), v.Version(), v.UpdatedAt(), v.DeletedAt(), v.ID(), expected)
	if e != nil {
		return fmt.Errorf("update match: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return game.ErrMatchVersionConflict
	}
	return nil
}
func (r *MatchRepository) List(ctx context.Context) ([]game.Match, error) {
	rows, e := r.db.Query(ctx, `SELECT m.id::text,m.home_team_id::text,m.away_team_id::text,m.scheduled_at,m.location,m.status,m.version,m.created_at,m.updated_at,m.deleted_at FROM matches m JOIN teams h ON h.id=m.home_team_id AND h.deleted_at IS NULL JOIN teams a ON a.id=m.away_team_id AND a.deleted_at IS NULL WHERE m.deleted_at IS NULL ORDER BY m.scheduled_at,m.id`)
	if e != nil {
		return nil, fmt.Errorf("list matches: %w", e)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanMatch)
}
func scanMatch(row pgx.CollectableRow) (game.Match, error) {
	var raw, home, away, location string
	var scheduled, created, updated time.Time
	var deleted *time.Time
	var version uint64
	var status game.MatchStatus
	if e := row.Scan(&raw, &home, &away, &scheduled, &location, &status, &version, &created, &updated, &deleted); e != nil {
		return game.Match{}, e
	}
	return game.RestoreMatch(game.MatchID(raw), team.ID(home), team.ID(away), scheduled, location, status, version, created, updated, deleted)
}

type LineupRepository struct{ db dbtx }

func (r *LineupRepository) Create(ctx context.Context, v game.Lineup) error { return r.insert(ctx, v) }
func (r *LineupRepository) insert(ctx context.Context, v game.Lineup) error {
	for _, entry := range v.Entries() {
		_, e := r.db.Exec(ctx, `INSERT INTO lineups(id,match_id,team_id,kind,variant_number,variant_name,player_id,batting_order,position,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, entry.ID(), v.MatchID(), v.TeamID(), v.Kind(), v.VariantNumber(), v.VariantName(), entry.PlayerID(), entry.BattingOrder(), entry.Position(), v.Version(), v.CreatedAt(), v.UpdatedAt(), v.DeletedAt())
		if e != nil {
			return fmt.Errorf("create lineup: %w", e)
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
	q := `SELECT l.id::text,l.player_id::text,l.batting_order,l.position,l.variant_name,l.version,l.created_at,l.updated_at,l.deleted_at FROM lineups l JOIN matches m ON m.id=l.match_id AND m.deleted_at IS NULL JOIN teams t ON t.id=l.team_id AND t.deleted_at IS NULL JOIN players p ON p.id=l.player_id AND p.deleted_at IS NULL WHERE l.match_id=$1 AND l.team_id=$2 AND l.kind=$3 AND l.variant_number=$4 AND l.deleted_at IS NULL ORDER BY l.batting_order,l.id`
	if lock {
		q += ` FOR UPDATE OF l`
	}
	rows, e := r.db.Query(ctx, q, m, t, k, n)
	if e != nil {
		return game.Lineup{}, fmt.Errorf("get lineup: %w", e)
	}
	defer rows.Close()
	var entries []game.LineupEntry
	var name string
	var version uint64
	var created, updated time.Time
	var deleted *time.Time
	for rows.Next() {
		var id, pid string
		var order, pos int
		var rowName string
		var rowVersion uint64
		var rowCreated, rowUpdated time.Time
		var rowDeleted *time.Time
		if e = rows.Scan(&id, &pid, &order, &pos, &rowName, &rowVersion, &rowCreated, &rowUpdated, &rowDeleted); e != nil {
			return game.Lineup{}, e
		}
		entry, x := game.NewLineupEntry(game.LineupID(id), player.ID(pid), order, player.PositionFlags(pos))
		if x != nil {
			return game.Lineup{}, x
		}
		if len(entries) == 0 {
			name, version, created, updated, deleted = rowName, rowVersion, rowCreated, rowUpdated, rowDeleted
		} else if name != rowName || version != rowVersion || !created.Equal(rowCreated) || !updated.Equal(rowUpdated) {
			return game.Lineup{}, fmt.Errorf("%w: inconsistent lineup rows", game.ErrCorruptedLineup)
		}
		entries = append(entries, entry)
	}
	if e = rows.Err(); e != nil {
		return game.Lineup{}, e
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
		return fmt.Errorf("delete lineup: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return game.ErrLineupVersionConflict
	}
	return nil
}
func (r *LineupRepository) SoftDeleteByMatch(ctx context.Context, m game.MatchID, now time.Time) error {
	_, e := r.db.Exec(ctx, `UPDATE lineups SET deleted_at=$1,updated_at=$1,version=version+1 WHERE match_id=$2 AND deleted_at IS NULL`, now, m)
	if e != nil {
		return fmt.Errorf("delete match lineups: %w", e)
	}
	return nil
}
func (r *LineupRepository) ListByMatch(ctx context.Context, m game.MatchID) ([]game.Lineup, error) {
	rows, e := r.db.Query(ctx, `SELECT DISTINCT l.team_id::text,l.kind,l.variant_number FROM lineups l JOIN matches m ON m.id=l.match_id AND m.deleted_at IS NULL JOIN teams t ON t.id=l.team_id AND t.deleted_at IS NULL JOIN players p ON p.id=l.player_id AND p.deleted_at IS NULL WHERE l.match_id=$1 AND l.deleted_at IS NULL ORDER BY 1,2,3`, m)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	type key struct {
		team   string
		kind   game.LineupKind
		number uint16
	}
	var keys []key
	for rows.Next() {
		var k key
		if e = rows.Scan(&k.team, &k.kind, &k.number); e != nil {
			return nil, e
		}
		keys = append(keys, k)
	}
	var result []game.Lineup
	for _, k := range keys {
		v, e := r.Get(ctx, m, team.ID(k.team), k.kind, k.number)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, nil
}

func (r *LineupRepository) NameOccupied(ctx context.Context, matchID game.MatchID, teamID team.ID, name string, exceptKind game.LineupKind, exceptNumber uint16) (bool, error) {
	var occupied bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lineups WHERE match_id=$1 AND team_id=$2 AND variant_name=$3 AND deleted_at IS NULL AND NOT (kind=$4 AND variant_number=$5))`, matchID, teamID, name, exceptKind, exceptNumber).Scan(&occupied)
	if err != nil {
		return false, fmt.Errorf("check lineup name: %w", err)
	}
	return occupied, nil
}

type PlateRepository struct{ db dbtx }

func (r *PlateRepository) Create(ctx context.Context, v game.Plate) error {
	_, e := r.db.Exec(ctx, `INSERT INTO plates(id,match_id,sequence,inning,half,batting_order,batter_id,pitcher_id,pitch_sequence,plate_type,result_description,runner_on_first_id,runner_on_second_id,runner_on_third_id,home_score,away_score,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, plateArgs(v)...)
	if e != nil {
		return fmt.Errorf("create plate: %w", e)
	}
	return nil
}
func plateArgs(v game.Plate) []any {
	r := v.Runners()
	var home, away any
	if score := v.Score(); score != nil {
		home, away = score.Home, score.Away
	}
	return []any{v.ID(), v.MatchID(), v.Sequence(), v.Inning(), v.Half(), v.BattingOrder(), v.BatterID(), v.PitcherID(), v.PitchSequence(), v.Type(), v.ResultDescription(), playerPointer(r[0]), playerPointer(r[1]), playerPointer(r[2]), home, away, v.Version(), v.CreatedAt(), v.UpdatedAt(), v.DeletedAt()}
}
func playerPointer(v *player.ID) any {
	if v == nil {
		return nil
	}
	return string(*v)
}
func (r *PlateRepository) Get(ctx context.Context, id game.PlateID) (game.Plate, error) {
	return r.get(ctx, id, false)
}
func (r *PlateRepository) GetForUpdate(ctx context.Context, id game.PlateID) (game.Plate, error) {
	return r.get(ctx, id, true)
}
func (r *PlateRepository) get(ctx context.Context, id game.PlateID, lock bool) (game.Plate, error) {
	q := plateSelect + plateVisibleJoins + ` WHERE x.id=$1 AND x.deleted_at IS NULL AND (x.runner_on_first_id IS NULL OR r1.id IS NOT NULL) AND (x.runner_on_second_id IS NULL OR r2.id IS NOT NULL) AND (x.runner_on_third_id IS NULL OR r3.id IS NOT NULL)`
	if lock {
		q += ` FOR UPDATE OF x`
	}
	v, e := scanPlate(r.db.QueryRow(ctx, q, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return game.Plate{}, game.ErrPlateNotFound
	}
	if e != nil {
		return game.Plate{}, fmt.Errorf("get plate: %w", e)
	}
	return v, nil
}

const plateSelect = `SELECT x.id::text,x.match_id::text,x.sequence,x.inning,x.half,x.batting_order,x.batter_id::text,x.pitcher_id::text,x.pitch_sequence,x.plate_type,x.result_description,x.runner_on_first_id::text,x.runner_on_second_id::text,x.runner_on_third_id::text,x.home_score,x.away_score,x.version,x.created_at,x.updated_at,x.deleted_at FROM plates x`
const plateVisibleJoins = ` JOIN matches m ON m.id=x.match_id AND m.deleted_at IS NULL JOIN players b ON b.id=x.batter_id AND b.deleted_at IS NULL JOIN players p ON p.id=x.pitcher_id AND p.deleted_at IS NULL LEFT JOIN players r1 ON r1.id=x.runner_on_first_id AND r1.deleted_at IS NULL LEFT JOIN players r2 ON r2.id=x.runner_on_second_id AND r2.deleted_at IS NULL LEFT JOIN players r3 ON r3.id=x.runner_on_third_id AND r3.deleted_at IS NULL`

type scanner interface{ Scan(...any) error }

func scanPlate(row scanner) (game.Plate, error) {
	var id, mid, batter, pitcher, pitches, description string
	var first, second, third *string
	var home, away *int
	var sequence, inning, half, order, kind int
	var version uint64
	var created, updated time.Time
	var deleted *time.Time
	e := row.Scan(&id, &mid, &sequence, &inning, &half, &order, &batter, &pitcher, &pitches, &kind, &description, &first, &second, &third, &home, &away, &version, &created, &updated, &deleted)
	if e != nil {
		return game.Plate{}, e
	}
	runners := [3]*player.ID{toPlayer(first), toPlayer(second), toPlayer(third)}
	var score *game.Score
	if home != nil && away != nil {
		if *home < 0 || *home > 65535 || *away < 0 || *away > 65535 {
			return game.Plate{}, fmt.Errorf("%w: score out of range", game.ErrCorruptedPlate)
		}
		score = &game.Score{Home: uint16(*home), Away: uint16(*away)}
	} else if home != nil || away != nil {
		return game.Plate{}, fmt.Errorf("%w: incomplete score", game.ErrCorruptedPlate)
	}
	return game.RestorePlate(game.PlateID(id), game.MatchID(mid), sequence, inning, game.Half(half), order, player.ID(batter), player.ID(pitcher), pitches, game.PlateType(kind), description, runners, score, version, created, updated, deleted)
}
func toPlayer(v *string) *player.ID {
	if v == nil {
		return nil
	}
	x := player.ID(*v)
	return &x
}
func (r *PlateRepository) Update(ctx context.Context, v game.Plate, expected uint64) error {
	a := plateArgs(v)
	a = append(a, v.ID(), expected)
	tag, e := r.db.Exec(ctx, `UPDATE plates SET match_id=$2,sequence=$3,inning=$4,half=$5,batting_order=$6,batter_id=$7,pitcher_id=$8,pitch_sequence=$9,plate_type=$10,result_description=$11,runner_on_first_id=$12,runner_on_second_id=$13,runner_on_third_id=$14,home_score=$15,away_score=$16,version=$17,created_at=$18,updated_at=$19,deleted_at=$20 WHERE id=$21 AND version=$22 AND deleted_at IS NULL`, a...)
	if e != nil {
		return fmt.Errorf("update plate: %w", e)
	}
	if tag.RowsAffected() == 0 {
		return game.ErrPlateVersionConflict
	}
	return nil
}
func (r *PlateRepository) SoftDeleteByMatch(ctx context.Context, m game.MatchID, now time.Time) error {
	_, e := r.db.Exec(ctx, `UPDATE plates SET deleted_at=$1,updated_at=$1,version=version+1 WHERE match_id=$2 AND deleted_at IS NULL`, now, m)
	if e != nil {
		return fmt.Errorf("delete match plates: %w", e)
	}
	return nil
}
func (r *PlateRepository) ListByMatch(ctx context.Context, m game.MatchID) ([]game.Plate, error) {
	rows, e := r.db.Query(ctx, plateSelect+plateVisibleJoins+` WHERE x.match_id=$1 AND x.deleted_at IS NULL AND (x.runner_on_first_id IS NULL OR r1.id IS NOT NULL) AND (x.runner_on_second_id IS NULL OR r2.id IS NOT NULL) AND (x.runner_on_third_id IS NULL OR r3.id IS NOT NULL) ORDER BY x.sequence,x.id`, m)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (game.Plate, error) { return scanPlate(row) })
}

type GameReader struct {
	matches *MatchRepository
	lineups *LineupRepository
	plates  *PlateRepository
}

func (s *Store) ListMatches(ctx context.Context) ([]game.Match, error) { return s.Matches().List(ctx) }
func (s *Store) GetMatch(ctx context.Context, id game.MatchID) (game.Match, error) {
	return s.Matches().Get(ctx, id)
}
func (s *Store) ListLineups(ctx context.Context, id game.MatchID) ([]game.Lineup, error) {
	return s.Lineups().ListByMatch(ctx, id)
}
func (s *Store) ListPlates(ctx context.Context, id game.MatchID) ([]game.Plate, error) {
	return s.Plates().ListByMatch(ctx, id)
}

func (s *Store) Matches() *MatchRepository  { return &MatchRepository{db: s.pool} }
func (s *Store) Lineups() *LineupRepository { return &LineupRepository{db: s.pool} }
func (s *Store) Plates() *PlateRepository   { return &PlateRepository{db: s.pool} }
func (s *Store) GameDetail(ctx context.Context, id game.MatchID) (game.Detail, error) {
	m, e := s.Matches().Get(ctx, id)
	if e != nil {
		return game.Detail{}, e
	}
	ls, e := s.Lineups().ListByMatch(ctx, id)
	if e != nil {
		return game.Detail{}, e
	}
	ps, e := s.Plates().ListByMatch(ctx, id)
	if e != nil {
		return game.Detail{}, e
	}
	return game.Detail{Match: m, Lineups: ls, Plates: ps}, nil
}
