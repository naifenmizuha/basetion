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
	"github.com/naifenmizuha/basetion/src/internal/infra/postgres/sqlcgen"
)

type MatchRepository struct {
	queries *sqlcgen.Queries
}

func (r *MatchRepository) Create(ctx context.Context, v game.Match) error {
	return r.queries.CreateMatch(ctx, sqlcgen.CreateMatchParams{ID: string(v.ID()), HomeTeamID: string(v.HomeTeamID()), AwayTeamID: string(v.AwayTeamID()), ScheduledAt: timestamptz(v.ScheduledAt()), Location: v.Location(), Status: int16(v.Status()), CreatedAt: timestamptz(v.CreatedAt()), UpdatedAt: timestamptz(v.UpdatedAt()), DeletedAt: timestamptzPointer(v.DeletedAt())})
}
func (r *MatchRepository) Get(ctx context.Context, id game.MatchID) (game.Match, error) {
	return r.get(ctx, id)
}
func (r *MatchRepository) get(ctx context.Context, id game.MatchID) (game.Match, error) {
	row, err := r.queries.GetMatch(ctx, string(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return game.Match{}, game.ErrMatchNotFound
	}
	if err != nil {
		return game.Match{}, err
	}
	return game.RestoreMatch(game.MatchID(row.ID), team.ID(row.HomeTeamID), team.ID(row.AwayTeamID), requiredTimestamp(row.ScheduledAt), row.Location, game.MatchStatus(row.Status), requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
}
func (r *MatchRepository) Update(ctx context.Context, v game.Match) error {
	updated, err := r.queries.UpdateMatch(ctx, sqlcgen.UpdateMatchParams{HomeTeamID: string(v.HomeTeamID()), AwayTeamID: string(v.AwayTeamID()), ScheduledAt: timestamptz(v.ScheduledAt()), Location: v.Location(), Status: int16(v.Status()), UpdatedAt: timestamptz(v.UpdatedAt()), DeletedAt: timestamptzPointer(v.DeletedAt()), ID: string(v.ID())})
	if err != nil {
		return err
	}
	if updated == 0 {
		return game.ErrMatchNotFound
	}
	return nil
}
func (r *MatchRepository) List(ctx context.Context) ([]game.Match, error) {
	rows, err := r.queries.ListStoredMatches(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]game.Match, 0, len(rows))
	for _, row := range rows {
		value, e := game.RestoreMatch(game.MatchID(row.ID), team.ID(row.HomeTeamID), team.ID(row.AwayTeamID), requiredTimestamp(row.ScheduledAt), row.Location, game.MatchStatus(row.Status), requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt))
		if e != nil {
			return nil, e
		}
		values = append(values, value)
	}
	return values, nil
}

type LineupRepository struct {
	queries *sqlcgen.Queries
}

func (r *LineupRepository) Create(ctx context.Context, v game.Lineup) error { return r.insert(ctx, v) }
func (r *LineupRepository) insert(ctx context.Context, v game.Lineup) error {
	for _, x := range v.Entries() {
		e := r.queries.CreateLineupEntry(ctx, sqlcgen.CreateLineupEntryParams{ID: string(x.ID()), MatchID: string(v.MatchID()), TeamID: string(v.TeamID()), Kind: int16(v.Kind()), VariantNumber: int16(v.VariantNumber()), VariantName: v.VariantName(), PlayerID: string(x.PlayerID()), BattingOrder: int16(x.BattingOrder()), Position: int16(x.Position()), CreatedAt: timestamptz(v.CreatedAt()), UpdatedAt: timestamptz(v.UpdatedAt()), DeletedAt: timestamptzPointer(v.DeletedAt())})
		if e != nil {
			return e
		}
	}
	return nil
}
func (r *LineupRepository) Get(ctx context.Context, m game.MatchID, t team.ID, k game.LineupKind, n uint16) (game.Lineup, error) {
	return r.get(ctx, m, t, k, n)
}
func (r *LineupRepository) get(ctx context.Context, m game.MatchID, t team.ID, k game.LineupKind, n uint16) (game.Lineup, error) {
	rows, e := r.queries.GetLineupEntries(ctx, sqlcgen.GetLineupEntriesParams{MatchID: string(m), TeamID: string(t), Kind: int16(k), VariantNumber: int16(n)})
	if e != nil {
		return game.Lineup{}, e
	}
	var entries []game.LineupEntry
	var name string
	var created, updated time.Time
	var deleted *time.Time
	for _, row := range rows {
		x, err := game.NewLineupEntry(game.LineupID(row.ID), player.ID(row.PlayerID), int(row.BattingOrder), player.PositionFlags(row.Position))
		if err != nil {
			return game.Lineup{}, err
		}
		if len(entries) == 0 {
			name, created, updated, deleted = row.VariantName, requiredTimestamp(row.CreatedAt), requiredTimestamp(row.UpdatedAt), timestampTime(row.DeletedAt)
		}
		entries = append(entries, x)
	}
	if len(entries) == 0 {
		return game.Lineup{}, game.ErrLineupNotFound
	}
	return game.RestoreLineup(m, t, k, int(n), name, entries, created, updated, deleted)
}
func (r *LineupRepository) Replace(ctx context.Context, v game.Lineup, now time.Time) error {
	updated, e := r.queries.SoftDeleteLineup(ctx, sqlcgen.SoftDeleteLineupParams{DeletedAt: timestamptz(now), MatchID: string(v.MatchID()), TeamID: string(v.TeamID()), Kind: int16(v.Kind()), VariantNumber: int16(v.VariantNumber())})
	if e != nil {
		return fmt.Errorf("replace lineup: %w", e)
	}
	if updated == 0 {
		return game.ErrLineupNotFound
	}
	return r.insert(ctx, v)
}
func (r *LineupRepository) SoftDelete(ctx context.Context, v game.Lineup, now time.Time) error {
	updated, e := r.queries.SoftDeleteLineup(ctx, sqlcgen.SoftDeleteLineupParams{DeletedAt: timestamptz(now), MatchID: string(v.MatchID()), TeamID: string(v.TeamID()), Kind: int16(v.Kind()), VariantNumber: int16(v.VariantNumber())})
	if e != nil {
		return e
	}
	if updated == 0 {
		return game.ErrLineupNotFound
	}
	return nil
}
func (r *LineupRepository) SoftDeleteByMatch(ctx context.Context, m game.MatchID, now time.Time) error {
	return r.queries.SoftDeleteLineupsByMatch(ctx, sqlcgen.SoftDeleteLineupsByMatchParams{DeletedAt: timestamptz(now), MatchID: string(m)})
}
func (r *LineupRepository) ListByMatch(ctx context.Context, m game.MatchID) ([]game.Lineup, error) {
	rows, e := r.queries.ListLineupKeysByMatch(ctx, string(m))
	if e != nil {
		return nil, e
	}
	type key struct {
		t string
		k game.LineupKind
		n uint16
	}
	var keys []key
	for _, row := range rows {
		keys = append(keys, key{t: row.TeamID, k: game.LineupKind(row.Kind), n: uint16(row.VariantNumber)})
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
	return r.queries.LineupNameOccupied(ctx, sqlcgen.LineupNameOccupiedParams{MatchID: string(m), TeamID: string(t), VariantName: name, Kind: int16(k), VariantNumber: int16(n)})
}

func (s *Store) ListMatches(ctx context.Context, filter game.MatchFilter) ([]game.MatchView, error) {
	matches, err := s.selectedMatchViews(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list match views: %w", err)
	}
	values := make([]game.MatchView, len(matches))
	for index, match := range matches {
		values[index] = match.view
	}
	return values, nil
}
func (s *Store) SummarizeMatches(ctx context.Context, filter game.MatchFilter) ([]game.MatchSummaryView, error) {
	matches, err := s.selectedMatchViews(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.summarizeSelectedMatches(ctx, matches)
}

type selectedMatch struct {
	id   string
	view game.MatchView
}

func (s *Store) selectedMatchViews(ctx context.Context, filter game.MatchFilter) ([]selectedMatch, error) {
	params := matchQueryParams(filter)
	rows, err := s.queryExecutor().ListSelectedMatches(ctx, sqlcgen.ListSelectedMatchesParams(params))
	if err != nil {
		return nil, fmt.Errorf("select matches: %w", err)
	}
	values := make([]selectedMatch, 0, len(rows))
	for _, row := range rows {
		values = append(values, selectedMatch{id: row.MID, view: game.MatchView{ID: game.MatchID(row.MID), ScheduledAt: requiredTimestamp(row.ScheduledAt), HomeTeamName: row.HomeTeamName, AwayTeamName: row.AwayTeamName, Location: row.Location, Status: game.MatchStatus(row.Status)}})
	}
	return values, nil
}

// ListPlays returns the atomic play projection for already selected matches.
// The match identifiers are internal query references and are never exposed by
// model-facing adapters.
func (s *Store) ListPlays(ctx context.Context, matchIDs []game.MatchID) ([]game.PlayEventView, error) {
	ids := make([]string, len(matchIDs))
	for index, id := range matchIDs {
		ids[index] = string(id)
	}
	rows, err := s.queryExecutor().ListMatchRecordEvents(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list plays: %w", err)
	}
	values := make([]game.PlayEventView, 0, len(rows))
	for _, row := range rows {
		values = append(values, game.PlayEventView{
			Sequence:          uint32(row.Sequence),
			Inning:            uint16(row.Inning),
			Half:              game.Half(row.Half),
			BattingOrder:      uint8(row.BattingOrder),
			Batter:            game.PlayerIdentityView{Name: row.BatterName, TeamName: row.BatterTeamName, JerseyNumber: uint8(row.BatterJerseyNumber)},
			StartingPitcher:   game.PlayerIdentityView{Name: row.PitcherName, TeamName: row.PitcherTeamName, JerseyNumber: uint8(row.PitcherJerseyNumber)},
			Situation:         game.SituationView{Outs: uint8(row.AfterOuts), HomeScore: uint16(row.AfterHomeScore), AwayScore: uint16(row.AfterAwayScore)},
			BattingResult:     game.BattingResult(row.BattingResult),
			ResultDescription: row.ResultDescription,
		})
	}
	return values, nil
}

func matchQueryParams(filter game.MatchFilter) sqlcgen.ListSelectedMatchesParams {
	return sqlcgen.ListSelectedMatchesParams{ParticipantNames: filter.ParticipantNames, ScheduledFrom: timestamptzPointer(filter.ScheduledFrom), ScheduledTo: timestamptzPointer(filter.ScheduledTo), RowLimit: int32(filter.Limit)}
}

func (s *Store) summarizeSelectedMatches(ctx context.Context, matches []selectedMatch) ([]game.MatchSummaryView, error) {
	ids := make([]string, len(matches))
	for index, match := range matches {
		ids[index] = match.id
	}
	scores, err := s.queryExecutor().ListMatchFinalScores(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list match final scores: %w", err)
	}
	type score struct{ home, away int }
	byMatchID := make(map[string]score, len(scores))
	for _, row := range scores {
		byMatchID[row.MatchID] = score{home: int(row.AfterHomeScore), away: int(row.AfterAwayScore)}
	}
	result := make([]game.MatchSummaryView, len(matches))
	for index, match := range matches {
		item := game.MatchSummaryView{MatchView: match.view, Result: game.ResultPending}
		if value, ok := byMatchID[match.id]; ok {
			home, away := value.home, value.away
			item.HomeScore, item.AwayScore = &home, &away
		}
		switch {
		case match.view.Status == game.MatchCancelled:
			item.Result = game.ResultCancelled
		case match.view.Status != game.MatchFinal || item.HomeScore == nil || item.AwayScore == nil:
			item.Result = game.ResultPending
		case *item.HomeScore > *item.AwayScore:
			item.Result = game.ResultHomeWin
		case *item.AwayScore > *item.HomeScore:
			item.Result = game.ResultAwayWin
		default:
			item.Result = game.ResultDraw
		}
		result[index] = item
	}
	return result, nil
}

func (s *Store) ListMatchLineups(ctx context.Context, filter game.MatchFilter) ([]game.MatchLineupsView, error) {
	matches, err := s.selectedMatchViews(ctx, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(matches))
	result := make([]game.MatchLineupsView, len(matches))
	indexes := make(map[string]int, len(matches))
	for i, match := range matches {
		ids[i] = match.id
		result[i].Match = match.view
		indexes[match.id] = i
	}
	rows, err := s.queryExecutor().ListMatchLineupRows(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list match lineups: %w", err)
	}
	lineups := make(map[string]*game.LineupView)
	for _, row := range rows {
		value := &result[indexes[row.LMatchID]]
		position := player.PositionFlags(row.Position)
		if position == 0 || position&(position-1) != 0 {
			return nil, fmt.Errorf("match %s lineup has non-single position", row.LMatchID)
		}
		key := fmt.Sprintf("%s/%s/%d/%d", row.LMatchID, row.TeamName, row.Kind, row.VariantNumber)
		lineup := lineups[key]
		if lineup == nil {
			value.Lineups = append(value.Lineups, game.LineupView{TeamName: row.TeamName, Kind: game.LineupKind(row.Kind), VariantNumber: uint16(row.VariantNumber), VariantName: row.VariantName})
			lineup = &value.Lineups[len(value.Lineups)-1]
			lineups[key] = lineup
		}
		lineup.Entries = append(lineup.Entries, game.LineupEntryView{Player: game.PlayerIdentityView{Name: row.PlayerName, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber)}, BattingOrder: uint8(row.BattingOrder), Position: position})
	}
	return result, nil
}

func (s *Store) GetMatchRecords(ctx context.Context, filter game.MatchFilter) ([]game.MatchRecordView, error) {
	matches, err := s.selectedMatchViews(ctx, filter)
	if err != nil {
		return nil, err
	}
	summaries, err := s.summarizeSelectedMatches(ctx, matches)
	if err != nil {
		return nil, err
	}
	matchIDs := make([]string, len(matches))
	result := make([]game.MatchRecordView, len(matches))
	matchIndex := map[string]int{}
	for i, match := range matches {
		matchIDs[i] = match.id
		result[i].Summary = summaries[i]
		matchIndex[match.id] = i
	}
	queries := s.queryExecutor()
	events, err := queries.ListMatchRecordEvents(ctx, matchIDs)
	if err != nil {
		return nil, fmt.Errorf("list match records: %w", err)
	}
	playIDs := make([]string, len(events))
	eventIndex := map[string]*game.PlayEventView{}
	for i, row := range events {
		v := game.PlayEventView{Sequence: uint32(row.Sequence), Inning: uint16(row.Inning), Half: game.Half(row.Half), BattingOrder: uint8(row.BattingOrder), Batter: game.PlayerIdentityView{Name: row.BatterName, TeamName: row.BatterTeamName, JerseyNumber: uint8(row.BatterJerseyNumber)}, StartingPitcher: game.PlayerIdentityView{Name: row.PitcherName, TeamName: row.PitcherTeamName, JerseyNumber: uint8(row.PitcherJerseyNumber)}, Situation: game.SituationView{Outs: uint8(row.AfterOuts), HomeScore: uint16(row.AfterHomeScore), AwayScore: uint16(row.AfterAwayScore)}, BattingResult: game.BattingResult(row.BattingResult), ResultDescription: row.ResultDescription}
		result[matchIndex[row.PMatchID]].Events = append(result[matchIndex[row.PMatchID]].Events, v)
		eventIndex[row.PlayID] = &result[matchIndex[row.PMatchID]].Events[len(result[matchIndex[row.PMatchID]].Events)-1]
		playIDs[i] = row.PlayID
	}
	if err = s.assignRecordChildren(ctx, queries, playIDs, eventIndex); err != nil {
		return nil, err
	}
	return result, nil
}
func (s *Store) assignRecordChildren(ctx context.Context, queries *sqlcgen.Queries, playIDs []string, events map[string]*game.PlayEventView) error {
	pitches, err := queries.ListRecordPitches(ctx, playIDs)
	if err != nil {
		return err
	}
	for _, row := range pitches {
		v := events[row.PPlayID]
		if v == nil {
			continue
		}
		var zone *uint8
		if row.Zone != nil {
			x := uint8(*row.Zone)
			zone = &x
		}
		v.Pitches = append(v.Pitches, game.PitchEventView{Sequence: uint16(row.Sequence), Pitcher: game.PlayerIdentityView{Name: row.PitcherName, TeamName: row.PitcherTeamName, JerseyNumber: uint8(row.PitcherJerseyNumber)}, Batter: game.PlayerIdentityView{Name: row.BatterName, TeamName: row.BatterTeamName, JerseyNumber: uint8(row.BatterJerseyNumber)}, Result: game.PitchResult(row.Result), Balls: uint8(row.BallsAfter), Strikes: uint8(row.StrikesAfter), PitchType: row.PitchType, Velocity: row.Velocity, Zone: zone, Description: row.Description})
	}
	runners, err := queries.ListRecordRunners(ctx, playIDs)
	if err != nil {
		return err
	}
	for _, row := range runners {
		v := events[row.RPlayID]
		if v == nil {
			continue
		}
		item := game.RunnerOutcomeView{Sequence: uint16(row.Sequence), Runner: game.PlayerIdentityView{Name: row.RunnerName, TeamName: row.RunnerTeamName, JerseyNumber: uint8(row.RunnerJerseyNumber)}, Result: game.RunnerResult(row.Result), FromBase: uint8(row.FromBase), OutRecorded: row.OutRecorded, Scored: row.Scored, Earned: row.Earned, Description: row.Description}
		if row.ToBase != nil {
			x := uint8(*row.ToBase)
			item.ToBase = &x
		}
		if row.ChargedPitcherName != nil {
			item.ChargedPitcher = &game.PlayerIdentityView{Name: *row.ChargedPitcherName, TeamName: *row.ChargedPitcherTeamName, JerseyNumber: uint8(*row.ChargedPitcherJerseyNumber)}
		}
		if row.RbiBatterName != nil {
			item.RBIBatter = &game.PlayerIdentityView{Name: *row.RbiBatterName, TeamName: *row.RbiBatterTeamName, JerseyNumber: uint8(*row.RbiBatterJerseyNumber)}
		}
		v.RunnerOutcomes = append(v.RunnerOutcomes, item)
	}
	fielding, err := queries.ListRecordFielding(ctx, playIDs)
	if err != nil {
		return err
	}
	for _, row := range fielding {
		v := events[row.FPlayID]
		if v == nil {
			continue
		}
		v.FieldingOutcomes = append(v.FieldingOutcomes, game.FieldingOutcomeView{Sequence: uint16(row.Sequence), Fielder: game.PlayerIdentityView{Name: row.FielderName, TeamName: row.FielderTeamName, JerseyNumber: uint8(row.FielderJerseyNumber)}, Position: player.PositionFlags(row.Position), Result: game.FieldingResult(row.Result), Description: row.Description})
	}
	return nil
}
func (s *Store) AnalyzeMatchPlayers(ctx context.Context, filter game.MatchFilter) ([]game.MatchPlayerPerformanceView, error) {
	matches, err := s.selectedMatchViews(ctx, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(matches))
	result := make([]game.MatchPlayerPerformanceView, len(matches))
	indexes := map[string]int{}
	for i, match := range matches {
		ids[i] = match.id
		indexes[match.id] = i
		result[i] = game.MatchPlayerPerformanceView{Match: match.view, Limits: game.PerformanceLimitsView{FieldingOpportunitiesUnavailable: true, EarnedRunsRequireExplicitMark: true, UnrecordedPitchFactsExcluded: true}}
	}
	queries := s.queryExecutor()
	offense, err := queries.ListOffenseLines(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range offense {
		v := &result[indexes[row.MatchID]]
		v.Offense = append(v.Offense, offenseLine(row))
	}
	pitching, err := queries.ListPitchingLines(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range pitching {
		v := &result[indexes[row.MatchID]]
		v.Pitching = append(v.Pitching, game.PitchingLineView{Player: game.PlayerIdentityView{Name: row.Name, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber)}, BattersFaced: uint(row.Bf), Pitches: uint(row.Pitches), CalledStrikes: uint(row.CalledStrikes), SwingingStrikes: uint(row.SwingingStrikes), Hits: uint(row.H), HomeRuns: uint(row.Hr), Walks: uint(row.Bb), HitByPitch: uint(row.Hbp), Strikeouts: uint(row.So), Runs: uint(row.Runs), EarnedRuns: uint(row.EarnedRuns), Outs: uint(row.Outs)})
	}
	fielding, err := queries.ListFieldingLines(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range fielding {
		v := &result[indexes[row.PMatchID]]
		v.Fielding = append(v.Fielding, game.FieldingLineView{Player: game.PlayerIdentityView{Name: row.Name, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber)}, Putouts: uint(row.Putouts), Assists: uint(row.Assists), Errors: uint(row.Errors)})
	}
	return result, nil
}
func offenseLine(row sqlcgen.ListOffenseLinesRow) game.OffenseLineView {
	v := game.OffenseLineView{Player: game.PlayerIdentityView{Name: row.Name, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber)}, PA: uint(row.Pa), AB: uint(row.Ab), H: uint(row.H), Singles: uint(row.Singles), Doubles: uint(row.Doubles), Triples: uint(row.Triples), HomeRuns: uint(row.Homers), Walks: uint(row.Walks), HitByPitch: uint(row.Hbp), Strikeouts: uint(row.So), RBI: uint(row.Rbi), Runs: uint(row.Runs), TotalBases: uint(row.Tb)}
	if v.AB > 0 {
		x := float64(v.H) / float64(v.AB)
		v.AVG = &x
		x = float64(v.TotalBases) / float64(v.AB)
		v.SLG = &x
	}
	denom := v.AB + v.Walks + v.HitByPitch
	if denom > 0 {
		x := float64(v.H+v.Walks+v.HitByPitch) / float64(denom)
		v.OBP = &x
		if v.SLG != nil {
			x += *v.SLG
			v.OPS = &x
		}
	}
	return v
}

func (s *Store) offenseLines(ctx context.Context, matchID string) ([]game.OffenseLineView, error) {
	rows, err := s.queryExecutor().ListOffenseLines(ctx, []string{matchID})
	if err != nil {
		return nil, err
	}
	out := make([]game.OffenseLineView, 0, len(rows))
	for _, row := range rows {
		out = append(out, offenseLine(row))
	}
	return out, nil
}

func (s *Store) pitchingLines(ctx context.Context, matchID string) ([]game.PitchingLineView, error) {
	rows, err := s.queryExecutor().ListPitchingLines(ctx, []string{matchID})
	if err != nil {
		return nil, err
	}
	out := make([]game.PitchingLineView, 0, len(rows))
	for _, row := range rows {
		out = append(out, game.PitchingLineView{Player: game.PlayerIdentityView{Name: row.Name, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber)}, BattersFaced: uint(row.Bf), Pitches: uint(row.Pitches), CalledStrikes: uint(row.CalledStrikes), SwingingStrikes: uint(row.SwingingStrikes), Hits: uint(row.H), HomeRuns: uint(row.Hr), Walks: uint(row.Bb), HitByPitch: uint(row.Hbp), Strikeouts: uint(row.So), Runs: uint(row.Runs), EarnedRuns: uint(row.EarnedRuns), Outs: uint(row.Outs)})
	}
	return out, nil
}

func (s *Store) fieldingLines(ctx context.Context, matchID string) ([]game.FieldingLineView, error) {
	rows, err := s.queryExecutor().ListFieldingLines(ctx, []string{matchID})
	if err != nil {
		return nil, err
	}
	out := make([]game.FieldingLineView, 0, len(rows))
	for _, row := range rows {
		out = append(out, game.FieldingLineView{Player: game.PlayerIdentityView{Name: row.Name, TeamName: row.TeamName, JerseyNumber: uint8(row.JerseyNumber)}, Putouts: uint(row.Putouts), Assists: uint(row.Assists), Errors: uint(row.Errors)})
	}
	return out, nil
}
func (s *Store) Matches() *MatchRepository {
	return &MatchRepository{queries: sqlcgen.New(s.pool)}
}
func (s *Store) Lineups() *LineupRepository {
	return &LineupRepository{queries: sqlcgen.New(s.pool)}
}
func (s *Store) Plays() *PlayRepository {
	return &PlayRepository{queries: sqlcgen.New(s.pool)}
}
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
