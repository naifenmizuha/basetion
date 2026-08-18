package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type PlayRepository struct{ db dbtx }

func (r *PlayRepository) Create(ctx context.Context, v game.Play) error {
	if err := r.insertPlay(ctx, v); err != nil {
		return err
	}
	return r.insertChildren(ctx, v)
}

func (r *PlayRepository) insertPlay(ctx context.Context, v game.Play) error {
	b, a := v.Before(), v.After()
	_, err := r.db.Exec(ctx, `INSERT INTO plays(id,match_id,sequence,inning,half,batting_order,batter_id,starting_pitcher_id,batting_result,result_description,before_outs,before_home_score,before_away_score,before_runner_on_first_id,before_runner_on_second_id,before_runner_on_third_id,after_outs,after_home_score,after_away_score,after_runner_on_first_id,after_runner_on_second_id,after_runner_on_third_id,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`, playArgs(v, b, a)...)
	if err != nil {
		return fmt.Errorf("create play: %w", err)
	}
	return nil
}

func playArgs(v game.Play, b, a game.Situation) []any {
	return []any{v.ID(), v.MatchID(), v.Sequence(), v.Inning(), v.Half(), v.BattingOrder(), v.BatterID(), v.StartingPitcherID(), v.BattingResult(), v.ResultDescription(), b.Outs, b.HomeScore, b.AwayScore, playerPointer(b.Runners[0]), playerPointer(b.Runners[1]), playerPointer(b.Runners[2]), a.Outs, a.HomeScore, a.AwayScore, playerPointer(a.Runners[0]), playerPointer(a.Runners[1]), playerPointer(a.Runners[2]), v.Version(), v.CreatedAt(), v.UpdatedAt(), v.DeletedAt()}
}
func playerPointer(v *player.ID) any {
	if v == nil {
		return nil
	}
	return string(*v)
}

func (r *PlayRepository) insertChildren(ctx context.Context, v game.Play) error {
	for _, p := range v.Pitches() {
		_, err := r.db.Exec(ctx, `INSERT INTO pitches(id,play_id,sequence,pitcher_id,batter_id,result,balls_before,strikes_before,balls_after,strikes_after,pitch_type,velocity,zone,description,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, p.ID, v.ID(), p.Sequence, p.PitcherID, p.BatterID, p.Result, p.BallsBefore, p.StrikesBefore, p.BallsAfter, p.StrikesAfter, p.PitchType, p.Velocity, p.Zone, p.Description, p.Version, p.CreatedAt, p.UpdatedAt, p.DeletedAt)
		if err != nil {
			return fmt.Errorf("create pitch: %w", err)
		}
	}
	for _, x := range v.RunnerOutcomes() {
		_, err := r.db.Exec(ctx, `INSERT INTO play_runner_results(id,play_id,sequence,runner_id,result,from_base,to_base,out_recorded,scored,charged_pitcher_id,earned,rbi_batter_id,description,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, x.ID, v.ID(), x.Sequence, x.RunnerID, x.Result, x.FromBase, x.ToBase, x.OutRecorded, x.Scored, playerPointer(x.ChargedPitcherID), x.Earned, playerPointer(x.RBIBatterID), x.Description, x.Version, x.CreatedAt, x.UpdatedAt, x.DeletedAt)
		if err != nil {
			return fmt.Errorf("create runner result: %w", err)
		}
	}
	for _, x := range v.FieldingOutcomes() {
		_, err := r.db.Exec(ctx, `INSERT INTO play_fielding_results(id,play_id,sequence,fielder_id,position,result,description,version,created_at,updated_at,deleted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, x.ID, v.ID(), x.Sequence, x.FielderID, x.Position, x.Result, x.Description, x.Version, x.CreatedAt, x.UpdatedAt, x.DeletedAt)
		if err != nil {
			return fmt.Errorf("create fielding result: %w", err)
		}
	}
	return nil
}

const playSelect = `SELECT id::text,match_id::text,sequence,inning,half,batting_order,batter_id::text,starting_pitcher_id::text,batting_result,result_description,before_outs,before_home_score,before_away_score,before_runner_on_first_id::text,before_runner_on_second_id::text,before_runner_on_third_id::text,after_outs,after_home_score,after_away_score,after_runner_on_first_id::text,after_runner_on_second_id::text,after_runner_on_third_id::text,version,created_at,updated_at,deleted_at FROM plays`

type scanner interface{ Scan(...any) error }

func (r *PlayRepository) Get(ctx context.Context, id game.PlayID) (game.Play, error) {
	return r.get(ctx, id, false)
}
func (r *PlayRepository) GetForUpdate(ctx context.Context, id game.PlayID) (game.Play, error) {
	return r.get(ctx, id, true)
}
func (r *PlayRepository) get(ctx context.Context, id game.PlayID, lock bool) (game.Play, error) {
	q := playSelect + ` WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		q += ` FOR UPDATE`
	}
	return r.scan(ctx, r.db.QueryRow(ctx, q, id))
}

func (r *PlayRepository) scan(ctx context.Context, row scanner) (game.Play, error) {
	var id, mid, batter, pitcher, description string
	var beforeRaw, afterRaw [3]*string
	var sequence, inning, half, order, result, bo, bh, ba, ao, ah, aa int
	var version uint64
	var created, updated time.Time
	var deleted *time.Time
	err := row.Scan(&id, &mid, &sequence, &inning, &half, &order, &batter, &pitcher, &result, &description, &bo, &bh, &ba, &beforeRaw[0], &beforeRaw[1], &beforeRaw[2], &ao, &ah, &aa, &afterRaw[0], &afterRaw[1], &afterRaw[2], &version, &created, &updated, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return game.Play{}, game.ErrPlayNotFound
	}
	if err != nil {
		return game.Play{}, err
	}
	before, e := game.NewSituation(bo, bh, ba, toRunners(beforeRaw))
	if e != nil {
		return game.Play{}, e
	}
	after, e := game.NewSituation(ao, ah, aa, toRunners(afterRaw))
	if e != nil {
		return game.Play{}, e
	}
	pitches, e := r.listPitches(ctx, game.PlayID(id))
	if e != nil {
		return game.Play{}, e
	}
	runners, e := r.listRunnerOutcomes(ctx, game.PlayID(id))
	if e != nil {
		return game.Play{}, e
	}
	fielding, e := r.listFieldingOutcomes(ctx, game.PlayID(id))
	if e != nil {
		return game.Play{}, e
	}
	return game.RestorePlay(game.PlayDraft{ID: game.PlayID(id), MatchID: game.MatchID(mid), Sequence: sequence, Inning: inning, Half: game.Half(half), BattingOrder: order, BatterID: player.ID(batter), StartingPitcherID: player.ID(pitcher), Before: before, After: after, BattingResult: game.BattingResult(result), ResultDescription: description, Pitches: pitches, RunnerOutcomes: runners, FieldingOutcomes: fielding}, version, created, updated, deleted)
}
func toRunners(v [3]*string) [3]*player.ID {
	var out [3]*player.ID
	for i, x := range v {
		if x != nil {
			p := player.ID(*x)
			out[i] = &p
		}
	}
	return out
}
func toPlayer(v *string) *player.ID {
	if v == nil {
		return nil
	}
	x := player.ID(*v)
	return &x
}

func (r *PlayRepository) listPitches(ctx context.Context, id game.PlayID) ([]game.Pitch, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text,sequence,pitcher_id::text,batter_id::text,result,balls_before,strikes_before,balls_after,strikes_after,pitch_type,velocity,zone,description,version,created_at,updated_at,deleted_at FROM pitches WHERE play_id=$1 AND deleted_at IS NULL ORDER BY sequence`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (game.Pitch, error) {
		var raw, p, b, kind, d string
		var seq, result, bb, sb, ba, sa int
		var velocity *float64
		var zone *uint8
		var version uint64
		var c, u time.Time
		var deleted *time.Time
		if e := row.Scan(&raw, &seq, &p, &b, &result, &bb, &sb, &ba, &sa, &kind, &velocity, &zone, &d, &version, &c, &u, &deleted); e != nil {
			return game.Pitch{}, e
		}
		return game.RestorePitch(game.PitchID(raw), seq, player.ID(p), player.ID(b), game.PitchResult(result), bb, sb, ba, sa, kind, velocity, zone, d, version, c, u, deleted)
	})
}
func (r *PlayRepository) listRunnerOutcomes(ctx context.Context, id game.PlayID) ([]game.RunnerOutcome, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text,sequence,runner_id::text,result,from_base,to_base,out_recorded,scored,charged_pitcher_id::text,earned,rbi_batter_id::text,description,version,created_at,updated_at,deleted_at FROM play_runner_results WHERE play_id=$1 AND deleted_at IS NULL ORDER BY sequence`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (game.RunnerOutcome, error) {
		var raw, p, d string
		var charged, rbi *string
		var seq, result, from int
		var to *int
		var out, scored bool
		var earned *bool
		var version uint64
		var c, u time.Time
		var deleted *time.Time
		if e := row.Scan(&raw, &seq, &p, &result, &from, &to, &out, &scored, &charged, &earned, &rbi, &d, &version, &c, &u, &deleted); e != nil {
			return game.RunnerOutcome{}, e
		}
		return game.RestoreRunnerOutcome(game.RunnerResultID(raw), seq, player.ID(p), game.RunnerResult(result), from, to, out, scored, toPlayer(charged), earned, toPlayer(rbi), d, version, c, u, deleted)
	})
}
func (r *PlayRepository) listFieldingOutcomes(ctx context.Context, id game.PlayID) ([]game.FieldingOutcome, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text,sequence,fielder_id::text,position,result,description,version,created_at,updated_at,deleted_at FROM play_fielding_results WHERE play_id=$1 AND deleted_at IS NULL ORDER BY sequence`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (game.FieldingOutcome, error) {
		var raw, p, d string
		var seq, pos, result int
		var version uint64
		var c, u time.Time
		var deleted *time.Time
		if e := row.Scan(&raw, &seq, &p, &pos, &result, &d, &version, &c, &u, &deleted); e != nil {
			return game.FieldingOutcome{}, e
		}
		return game.RestoreFieldingOutcome(game.FieldingResultID(raw), seq, player.ID(p), player.PositionFlags(pos), game.FieldingResult(result), d, version, c, u, deleted)
	})
}

func (r *PlayRepository) ListByMatch(ctx context.Context, id game.MatchID) ([]game.Play, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text FROM plays WHERE match_id=$1 AND deleted_at IS NULL ORDER BY sequence`, id)
	if e != nil {
		return nil, e
	}
	var ids []game.PlayID
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, game.PlayID(raw))
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return nil, e
	}
	rows.Close()
	out := make([]game.Play, 0, len(ids))
	for _, playID := range ids {
		value, err := r.Get(ctx, playID)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}
func (r *PlayRepository) softDeleteChildren(ctx context.Context, id game.PlayID, now time.Time) error {
	for _, table := range []string{"pitches", "play_runner_results", "play_fielding_results"} {
		if _, e := r.db.Exec(ctx, `UPDATE `+table+` SET deleted_at=$1,updated_at=$1,version=version+1 WHERE play_id=$2 AND deleted_at IS NULL`, now, id); e != nil {
			return e
		}
	}
	return nil
}
func (r *PlayRepository) Replace(ctx context.Context, v game.Play, expected uint64, now time.Time) error {
	if e := r.softDeleteChildren(ctx, v.ID(), now); e != nil {
		return e
	}
	b, a := v.Before(), v.After()
	args := playArgs(v, b, a)
	args = append(args, v.ID(), expected)
	tag, e := r.db.Exec(ctx, `UPDATE plays SET match_id=$2,sequence=$3,inning=$4,half=$5,batting_order=$6,batter_id=$7,starting_pitcher_id=$8,batting_result=$9,result_description=$10,before_outs=$11,before_home_score=$12,before_away_score=$13,before_runner_on_first_id=$14,before_runner_on_second_id=$15,before_runner_on_third_id=$16,after_outs=$17,after_home_score=$18,after_away_score=$19,after_runner_on_first_id=$20,after_runner_on_second_id=$21,after_runner_on_third_id=$22,version=$23,created_at=$24,updated_at=$25,deleted_at=$26 WHERE id=$27 AND version=$28 AND deleted_at IS NULL`, args...)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return game.ErrPlayVersionConflict
	}
	return r.insertChildren(ctx, v)
}
func (r *PlayRepository) SoftDelete(ctx context.Context, id game.PlayID, expected uint64, now time.Time) error {
	if e := r.softDeleteChildren(ctx, id, now); e != nil {
		return e
	}
	tag, e := r.db.Exec(ctx, `UPDATE plays SET deleted_at=$1,updated_at=$1,version=version+1 WHERE id=$2 AND version=$3 AND deleted_at IS NULL`, now, id, expected)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return game.ErrPlayVersionConflict
	}
	return nil
}
func (r *PlayRepository) SoftDeleteByMatch(ctx context.Context, id game.MatchID, now time.Time) error {
	rows, e := r.db.Query(ctx, `SELECT id::text FROM plays WHERE match_id=$1 AND deleted_at IS NULL`, id)
	if e != nil {
		return e
	}
	var ids []game.PlayID
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, game.PlayID(raw))
	}
	rows.Close()
	for _, pid := range ids {
		if e = r.softDeleteChildren(ctx, pid, now); e != nil {
			return e
		}
	}
	_, e = r.db.Exec(ctx, `UPDATE plays SET deleted_at=$1,updated_at=$1,version=version+1 WHERE match_id=$2 AND deleted_at IS NULL`, now, id)
	return e
}
