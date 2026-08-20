package game

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

// CompactGameRecordDraft is the model-facing game input. Repeated team
// references and derived baseball state deliberately do not belong here.
type CompactGameRecordDraft struct {
	HomeTeamName, AwayTeamName string
	ScheduledAt, Location      string
	HomeLineup, AwayLineup     CompactLineupDraft
	Plays                      []CompactPlayDraft
}
type CompactLineupDraft struct {
	Name    string
	Entries []CompactLineupEntryDraft
}
type CompactLineupEntryDraft struct {
	JerseyNumber, BattingOrder int
	Position                   player.PositionFlags
}
type CompactPitchDraft struct {
	Result      PitchResult
	PitchType   string
	Velocity    *float64
	Zone        *int
	Description string
}
type CompactRunnerOutcomeDraft struct {
	JerseyNumber, FromBase                            int
	Result                                            RunnerResult
	ToBase                                            *int
	ChargedPitcherJerseyNumber, RBIBatterJerseyNumber *int
	Earned                                            *bool
	Description                                       string
}
type CompactFieldingOutcomeDraft struct {
	JerseyNumber int
	Position     player.PositionFlags
	Result       FieldingResult
	Description  string
}
type CompactPlayDraft struct {
	Inning, BattingOrder, BatterJerseyNumber, PitcherJerseyNumber int
	Half                                                          Half
	BattingResult                                                 BattingResult
	ResultDescription                                             string
	Pitches                                                       []CompactPitchDraft
	RunnerOutcomes                                                []CompactRunnerOutcomeDraft
	FieldingOutcomes                                              []CompactFieldingOutcomeDraft
}

// CreateCompactGameRecord expands the compact protocol before entering the
// established named-record write path, preserving its validation and partial
// write behaviour.
func (s *Service) CreateCompactGameRecord(ctx context.Context, draft CompactGameRecordDraft) (GameCreateProgress, error) {
	named, err := ExpandCompactGameRecord(draft)
	if err != nil {
		return GameCreateProgress{}, err
	}
	return s.CreateNamedGameRecord(ctx, named)
}

func ExpandCompactGameRecord(draft CompactGameRecordDraft) (NamedGameRecordDraft, error) {
	result := NamedGameRecordDraft{HomeTeamName: strings.TrimSpace(draft.HomeTeamName), AwayTeamName: strings.TrimSpace(draft.AwayTeamName), ScheduledAt: strings.TrimSpace(draft.ScheduledAt), Location: draft.Location}
	if result.HomeTeamName == "" || result.AwayTeamName == "" || result.ScheduledAt == "" {
		return result, errors.New("home_team_name, away_team_name and scheduled_at are required")
	}
	lineup := func(teamName string, input CompactLineupDraft) NamedLineupDraft {
		out := NamedLineupDraft{Name: input.Name}
		for _, entry := range input.Entries {
			out.Entries = append(out.Entries, NamedLineupEntryDraft{Player: PlayerReference{TeamName: teamName, JerseyNumber: entry.JerseyNumber}, BattingOrder: entry.BattingOrder, Position: entry.Position})
		}
		return out
	}
	result.HomeLineup, result.AwayLineup = lineup(result.HomeTeamName, draft.HomeLineup), lineup(result.AwayTeamName, draft.AwayLineup)
	state := compactState{}
	for index, input := range draft.Plays {
		attacking, defending := result.AwayTeamName, result.HomeTeamName
		if input.Half == Bottom {
			attacking, defending = result.HomeTeamName, result.AwayTeamName
		}
		if err := state.start(input, index); err != nil {
			return result, fmt.Errorf("derive play %d: %w", index+1, err)
		}
		before := state.situation()
		batter := PlayerReference{TeamName: attacking, JerseyNumber: input.BatterJerseyNumber}
		pitcher := PlayerReference{TeamName: defending, JerseyNumber: input.PitcherJerseyNumber}
		play := NamedPlayDraft{Inning: input.Inning, Half: input.Half, BattingOrder: input.BattingOrder, Batter: batter, StartingPitcher: pitcher, Before: before, BattingResult: input.BattingResult, ResultDescription: input.ResultDescription}
		balls, strikes := 0, 0
		for _, p := range input.Pitches {
			beforeBalls, beforeStrikes := balls, strikes
			balls, strikes = advanceCount(p.Result, balls, strikes)
			play.Pitches = append(play.Pitches, NamedPitchDraft{Pitcher: pitcher, Batter: batter, Result: p.Result, BallsBefore: beforeBalls, StrikesBefore: beforeStrikes, BallsAfter: balls, StrikesAfter: strikes, PitchType: p.PitchType, Velocity: p.Velocity, Zone: p.Zone, Description: p.Description})
		}
		if len(play.Pitches) == 0 {
			return result, fmt.Errorf("derive play %d: pitches are required", index+1)
		}
		if err := state.apply(input, batter, pitcher, attacking, defending, &play); err != nil {
			return result, fmt.Errorf("derive play %d: %w", index+1, err)
		}
		play.After = state.situation()
		result.Plays = append(result.Plays, play)
	}
	if len(result.Plays) == 0 {
		return result, errors.New("plays are required")
	}
	return result, nil
}

type compactState struct {
	initialized      bool
	inning           int
	half             Half
	outs, home, away int
	runners          [3]*PlayerReference
}

func (s *compactState) start(play CompactPlayDraft, index int) error {
	if play.Inning < 1 || (play.Half != Top && play.Half != Bottom) {
		return errors.New("inning and half are required")
	}
	if !s.initialized {
		if play.Inning != 1 || play.Half != Top {
			return errors.New("first play must be top of inning 1")
		}
		s.initialized, s.inning, s.half = true, play.Inning, play.Half
		return nil
	}
	if play.Inning == s.inning && play.Half == s.half {
		if s.outs == 3 {
			return errors.New("half inning is complete")
		}
		return nil
	}
	if s.outs != 3 {
		return errors.New("inning half changed before three outs")
	}
	if (s.half == Top && (play.Half != Bottom || play.Inning != s.inning)) || (s.half == Bottom && (play.Half != Top || play.Inning != s.inning+1)) {
		return errors.New("invalid inning transition")
	}
	s.inning, s.half, s.outs, s.runners = play.Inning, play.Half, 0, [3]*PlayerReference{}
	return nil
}
func (s compactState) situation() NamedSituationDraft {
	return NamedSituationDraft{Outs: s.outs, HomeScore: s.home, AwayScore: s.away, Runners: s.runners}
}
func (s *compactState) apply(input CompactPlayDraft, batter, pitcher PlayerReference, attacking, defending string, play *NamedPlayDraft) error {
	if !input.BattingResult.Valid() || strings.TrimSpace(input.ResultDescription) == "" {
		return errors.New("batting_result and result_description are required")
	}
	seenSources := map[int]bool{}
	for _, value := range input.RunnerOutcomes {
		if value.FromBase < 0 || value.FromBase > 3 || !value.Result.Valid() {
			return errors.New("invalid runner outcome")
		}
		if seenSources[value.FromBase] {
			return errors.New("runner outcome source base is duplicated")
		}
		seenSources[value.FromBase] = true
		var runner PlayerReference
		if value.FromBase == 0 {
			if value.JerseyNumber != batter.JerseyNumber {
				return errors.New("from_base=0 runner must be the batter")
			}
			runner = batter
		} else {
			current := s.runners[value.FromBase-1]
			if current == nil || current.JerseyNumber != value.JerseyNumber {
				return errors.New("runner does not match current base")
			}
			runner = *current
		}
		out := value.Result == RunnerForceOut || value.Result == RunnerTagOut || value.Result == RunnerCaughtStealing || value.Result == RunnerPickedOff
		scored := value.Result == RunnerScore
		charged := (*PlayerReference)(nil)
		if scored {
			charged = &pitcher
			if value.ChargedPitcherJerseyNumber != nil {
				charged = &PlayerReference{TeamName: defending, JerseyNumber: *value.ChargedPitcherJerseyNumber}
			}
			if value.Earned == nil {
				return errors.New("earned is required for a score")
			}
		}
		var rbi *PlayerReference
		if value.RBIBatterJerseyNumber != nil {
			rbi = &PlayerReference{TeamName: attacking, JerseyNumber: *value.RBIBatterJerseyNumber}
		}
		play.RunnerOutcomes = append(play.RunnerOutcomes, NamedRunnerOutcomeDraft{Runner: runner, Result: value.Result, FromBase: value.FromBase, ToBase: value.ToBase, OutRecorded: out, Scored: scored, ChargedPitcher: charged, Earned: value.Earned, RBIBatter: rbi, Description: value.Description})
	}
	// Apply all removals before destinations, allowing simultaneous advances.
	for base := range seenSources {
		if base > 0 {
			s.runners[base-1] = nil
		}
	}
	for _, value := range input.RunnerOutcomes {
		out := value.Result == RunnerForceOut || value.Result == RunnerTagOut || value.Result == RunnerCaughtStealing || value.Result == RunnerPickedOff
		if out {
			s.outs++
			continue
		}
		if value.Result == RunnerScore {
			if input.Half == Top {
				s.away++
			} else {
				s.home++
			}
			continue
		}
		if value.ToBase == nil || *value.ToBase < 1 || *value.ToBase > 3 {
			return errors.New("non-scoring runner outcome requires to_base 1..3")
		}
		if s.runners[*value.ToBase-1] != nil {
			return errors.New("runner destination is occupied")
		}
		teamName := attacking
		s.runners[*value.ToBase-1] = &PlayerReference{TeamName: teamName, JerseyNumber: value.JerseyNumber}
	}
	if !seenSources[0] && battingOut(input.BattingResult) {
		s.outs++
	}
	if s.outs > 3 {
		return errors.New("play produces more than three outs")
	}
	for _, value := range input.FieldingOutcomes {
		play.FieldingOutcomes = append(play.FieldingOutcomes, NamedFieldingOutcomeDraft{Fielder: PlayerReference{TeamName: defending, JerseyNumber: value.JerseyNumber}, Position: value.Position, Result: value.Result, Description: value.Description})
	}
	return nil
}
func battingOut(result BattingResult) bool {
	return result == BattingStrikeout || result == BattingGroundOut || result == BattingFlyOut || result == BattingLineOut || result == BattingSacrificeBunt || result == BattingSacrificeFly
}
func advanceCount(result PitchResult, balls, strikes int) (int, int) {
	switch result {
	case PitchBall, PitchIntentionalBall, PitchPitchout:
		balls++
	case PitchCalledStrike, PitchSwingingStrike, PitchFoulTip:
		strikes++
	case PitchFoul:
		if strikes < 2 {
			strikes++
		}
	}
	return balls, strikes
}
