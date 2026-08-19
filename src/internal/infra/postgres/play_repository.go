package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
)

type PlayRepository struct {
	queries *sqlcgen.Queries
}

func (r *PlayRepository) Create(ctx context.Context, v game.Play) error {
	if err := r.insertPlay(ctx, v); err != nil {
		return err
	}
	return r.insertChildren(ctx, v)
}

func (r *PlayRepository) insertPlay(ctx context.Context, v game.Play) error {
	b, a := v.Before(), v.After()
	err := r.queries.CreatePlay(ctx, sqlcgen.CreatePlayParams{ID: string(v.ID()), MatchID: string(v.MatchID()), Sequence: int32(v.Sequence()), Inning: int16(v.Inning()), Half: int16(v.Half()), BattingOrder: int16(v.BattingOrder()), BatterID: string(v.BatterID()), StartingPitcherID: string(v.StartingPitcherID()), BattingResult: int16(v.BattingResult()), ResultDescription: v.ResultDescription(), BeforeOuts: int16(b.Outs), BeforeHomeScore: int32(b.HomeScore), BeforeAwayScore: int32(b.AwayScore), BeforeRunnerOnFirstID: uuidArgument(playerString(b.Runners[0])), BeforeRunnerOnSecondID: uuidArgument(playerString(b.Runners[1])), BeforeRunnerOnThirdID: uuidArgument(playerString(b.Runners[2])), AfterOuts: int16(a.Outs), AfterHomeScore: int32(a.HomeScore), AfterAwayScore: int32(a.AwayScore), AfterRunnerOnFirstID: uuidArgument(playerString(a.Runners[0])), AfterRunnerOnSecondID: uuidArgument(playerString(a.Runners[1])), AfterRunnerOnThirdID: uuidArgument(playerString(a.Runners[2])), CreatedAt: timestamptz(v.CreatedAt()), UpdatedAt: timestamptz(v.UpdatedAt()), DeletedAt: timestamptzPointer(v.DeletedAt())})
	if err != nil {
		return fmt.Errorf("create play: %w", err)
	}
	return nil
}

func playerString(v *player.ID) *string {
	if v == nil {
		return nil
	}
	value := string(*v)
	return &value
}

func (r *PlayRepository) insertChildren(ctx context.Context, v game.Play) error {
	for _, p := range v.Pitches() {
		var zone *int16
		if p.Zone != nil {
			value := int16(*p.Zone)
			zone = &value
		}
		err := r.queries.CreatePlayPitch(ctx, sqlcgen.CreatePlayPitchParams{ID: string(p.ID), PlayID: string(v.ID()), Sequence: int16(p.Sequence), PitcherID: string(p.PitcherID), BatterID: string(p.BatterID), Result: int16(p.Result), BallsBefore: int16(p.BallsBefore), StrikesBefore: int16(p.StrikesBefore), BallsAfter: int16(p.BallsAfter), StrikesAfter: int16(p.StrikesAfter), PitchType: p.PitchType, Velocity: p.Velocity, Zone: zone, Description: p.Description, CreatedAt: timestamptz(p.CreatedAt), UpdatedAt: timestamptz(p.UpdatedAt), DeletedAt: timestamptzPointer(p.DeletedAt)})
		if err != nil {
			return fmt.Errorf("create pitch: %w", err)
		}
	}
	for _, x := range v.RunnerOutcomes() {
		var to *int16
		if x.ToBase != nil {
			value := int16(*x.ToBase)
			to = &value
		}
		err := r.queries.CreatePlayRunnerOutcome(ctx, sqlcgen.CreatePlayRunnerOutcomeParams{ID: string(x.ID), PlayID: string(v.ID()), Sequence: int16(x.Sequence), RunnerID: string(x.RunnerID), Result: int16(x.Result), FromBase: int16(x.FromBase), ToBase: to, OutRecorded: x.OutRecorded, Scored: x.Scored, ChargedPitcherID: uuidArgument(playerString(x.ChargedPitcherID)), Earned: x.Earned, RbiBatterID: uuidArgument(playerString(x.RBIBatterID)), Description: x.Description, CreatedAt: timestamptz(x.CreatedAt), UpdatedAt: timestamptz(x.UpdatedAt), DeletedAt: timestamptzPointer(x.DeletedAt)})
		if err != nil {
			return fmt.Errorf("create runner result: %w", err)
		}
	}
	for _, x := range v.FieldingOutcomes() {
		err := r.queries.CreatePlayFieldingOutcome(ctx, sqlcgen.CreatePlayFieldingOutcomeParams{ID: string(x.ID), PlayID: string(v.ID()), Sequence: int16(x.Sequence), FielderID: string(x.FielderID), Position: int16(x.Position), Result: int16(x.Result), Description: x.Description, CreatedAt: timestamptz(x.CreatedAt), UpdatedAt: timestamptz(x.UpdatedAt), DeletedAt: timestamptzPointer(x.DeletedAt)})
		if err != nil {
			return fmt.Errorf("create fielding result: %w", err)
		}
	}
	return nil
}

func (r *PlayRepository) Get(ctx context.Context, id game.PlayID) (game.Play, error) {
	return r.get(ctx, id)
}
func (r *PlayRepository) get(ctx context.Context, id game.PlayID) (game.Play, error) {
	row, err := r.queries.GetPlay(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return game.Play{}, game.ErrPlayNotFound
	}
	if err != nil {
		return game.Play{}, err
	}
	return r.restore(ctx, row)
}

func (r *PlayRepository) restore(ctx context.Context, row sqlcgen.GetPlayRow) (game.Play, error) {
	before, e := game.NewSituation(int(row.BeforeOuts), int(row.BeforeHomeScore), int(row.BeforeAwayScore), runners(anyString(row.BeforeRunnerOnFirstID), anyString(row.BeforeRunnerOnSecondID), anyString(row.BeforeRunnerOnThirdID)))
	if e != nil {
		return game.Play{}, e
	}
	after, e := game.NewSituation(int(row.AfterOuts), int(row.AfterHomeScore), int(row.AfterAwayScore), runners(anyString(row.AfterRunnerOnFirstID), anyString(row.AfterRunnerOnSecondID), anyString(row.AfterRunnerOnThirdID)))
	if e != nil {
		return game.Play{}, e
	}
	play_pitching_results, e := r.listPitches(ctx, game.PlayID(row.ID))
	if e != nil {
		return game.Play{}, e
	}
	runnerResults, e := r.listRunnerOutcomes(ctx, game.PlayID(row.ID))
	if e != nil {
		return game.Play{}, e
	}
	fielding, e := r.listFieldingOutcomes(ctx, game.PlayID(row.ID))
	if e != nil {
		return game.Play{}, e
	}
	return game.RestorePlay(game.PlayDraft{ID: game.PlayID(row.ID), MatchID: game.MatchID(row.MatchID), Sequence: int(row.Sequence), Inning: int(row.Inning), Half: game.Half(row.Half), BattingOrder: int(row.BattingOrder), BatterID: player.ID(row.BatterID), StartingPitcherID: player.ID(row.StartingPitcherID), Before: before, After: after, BattingResult: game.BattingResult(row.BattingResult), ResultDescription: row.ResultDescription, Pitches: play_pitching_results, RunnerOutcomes: runnerResults, FieldingOutcomes: fielding}, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
}
func runners(a, b, c string) [3]*player.ID {
	return [3]*player.ID{optionalPlayer(a), optionalPlayer(b), optionalPlayer(c)}
}
func optionalPlayer(value string) *player.ID {
	if value == "" {
		return nil
	}
	id := player.ID(value)
	return &id
}
func anyString(value any) string { text, _ := value.(string); return text }

func (r *PlayRepository) listPitches(ctx context.Context, id game.PlayID) ([]game.Pitch, error) {
	rows, e := r.queries.ListPlayPitches(ctx, string(id))
	if e != nil {
		return nil, e
	}
	values := make([]game.Pitch, 0, len(rows))
	for _, row := range rows {
		var zone *uint8
		if row.Zone != nil {
			value := uint8(*row.Zone)
			zone = &value
		}
		value, e := game.RestorePitch(game.PitchID(row.ID), int(row.Sequence), player.ID(row.PitcherID), player.ID(row.BatterID), game.PitchResult(row.Result), int(row.BallsBefore), int(row.StrikesBefore), int(row.BallsAfter), int(row.StrikesAfter), row.PitchType, row.Velocity, zone, row.Description, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	}
	return values, nil
}
func (r *PlayRepository) listRunnerOutcomes(ctx context.Context, id game.PlayID) ([]game.RunnerOutcome, error) {
	rows, e := r.queries.ListPlayRunnerOutcomes(ctx, string(id))
	if e != nil {
		return nil, e
	}
	values := make([]game.RunnerOutcome, 0, len(rows))
	for _, row := range rows {
		var to *int
		if row.ToBase != nil {
			value := int(*row.ToBase)
			to = &value
		}
		value, e := game.RestoreRunnerOutcome(game.RunnerResultID(row.ID), int(row.Sequence), player.ID(row.RunnerID), game.RunnerResult(row.Result), int(row.FromBase), to, row.OutRecorded, row.Scored, optionalPlayer(anyString(row.ChargedPitcherID)), row.Earned, optionalPlayer(anyString(row.RbiBatterID)), row.Description, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	}
	return values, nil
}
func (r *PlayRepository) listFieldingOutcomes(ctx context.Context, id game.PlayID) ([]game.FieldingOutcome, error) {
	rows, e := r.queries.ListPlayFieldingOutcomes(ctx, string(id))
	if e != nil {
		return nil, e
	}
	values := make([]game.FieldingOutcome, 0, len(rows))
	for _, row := range rows {
		value, e := game.RestoreFieldingOutcome(game.FieldingResultID(row.ID), int(row.Sequence), player.ID(row.FielderID), player.PositionFlags(row.Position), game.FieldingResult(row.Result), row.Description, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	}
	return values, nil
}

func (r *PlayRepository) ListByMatch(ctx context.Context, id game.MatchID) ([]game.Play, error) {
	rows, e := r.queries.ListPlayIDsByMatch(ctx, string(id))
	if e != nil {
		return nil, e
	}
	var ids []game.PlayID
	for _, raw := range rows {
		ids = append(ids, game.PlayID(raw))
	}
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
	args := sqlcgen.SoftDeletePlayPitchesParams{DeletedAt: timestamptz(now), PlayID: string(id)}
	if err := r.queries.SoftDeletePlayPitches(ctx, args); err != nil {
		return err
	}
	if err := r.queries.SoftDeletePlayRunnerOutcomes(ctx, sqlcgen.SoftDeletePlayRunnerOutcomesParams(args)); err != nil {
		return err
	}
	return r.queries.SoftDeletePlayFieldingOutcomes(ctx, sqlcgen.SoftDeletePlayFieldingOutcomesParams(args))
}
func (r *PlayRepository) Replace(ctx context.Context, v game.Play, now time.Time) error {
	if e := r.softDeleteChildren(ctx, v.ID(), now); e != nil {
		return e
	}
	b, a := v.Before(), v.After()
	updated, e := r.queries.UpdatePlay(ctx, sqlcgen.UpdatePlayParams{MatchID: string(v.MatchID()), Sequence: int32(v.Sequence()), Inning: int16(v.Inning()), Half: int16(v.Half()), BattingOrder: int16(v.BattingOrder()), BatterID: string(v.BatterID()), StartingPitcherID: string(v.StartingPitcherID()), BattingResult: int16(v.BattingResult()), ResultDescription: v.ResultDescription(), BeforeOuts: int16(b.Outs), BeforeHomeScore: int32(b.HomeScore), BeforeAwayScore: int32(b.AwayScore), BeforeRunnerOnFirstID: uuidArgument(playerString(b.Runners[0])), BeforeRunnerOnSecondID: uuidArgument(playerString(b.Runners[1])), BeforeRunnerOnThirdID: uuidArgument(playerString(b.Runners[2])), AfterOuts: int16(a.Outs), AfterHomeScore: int32(a.HomeScore), AfterAwayScore: int32(a.AwayScore), AfterRunnerOnFirstID: uuidArgument(playerString(a.Runners[0])), AfterRunnerOnSecondID: uuidArgument(playerString(a.Runners[1])), AfterRunnerOnThirdID: uuidArgument(playerString(a.Runners[2])), CreatedAt: timestamptz(v.CreatedAt()), UpdatedAt: timestamptz(v.UpdatedAt()), DeletedAt: timestamptzPointer(v.DeletedAt()), ID: string(v.ID())})
	if e != nil {
		return e
	}
	if updated == 0 {
		return game.ErrPlayNotFound
	}
	return r.insertChildren(ctx, v)
}
func (r *PlayRepository) SoftDelete(ctx context.Context, v game.Play, now time.Time) error {
	if e := r.softDeleteChildren(ctx, v.ID(), now); e != nil {
		return e
	}
	updated, e := r.queries.SoftDeletePlay(ctx, sqlcgen.SoftDeletePlayParams{DeletedAt: timestamptz(now), ID: string(v.ID())})
	if e != nil {
		return e
	}
	if updated == 0 {
		return game.ErrPlayNotFound
	}
	return nil
}
func (r *PlayRepository) SoftDeleteByMatch(ctx context.Context, id game.MatchID, now time.Time) error {
	rows, e := r.queries.ListPlayIDsByMatch(ctx, string(id))
	if e != nil {
		return e
	}
	var ids []game.PlayID
	for _, raw := range rows {
		ids = append(ids, game.PlayID(raw))
	}
	for _, pid := range ids {
		if e = r.softDeleteChildren(ctx, pid, now); e != nil {
			return e
		}
	}
	return r.queries.SoftDeletePlaysByMatch(ctx, sqlcgen.SoftDeletePlaysByMatchParams{DeletedAt: timestamptz(now), MatchID: string(id)})
}
