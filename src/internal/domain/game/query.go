package game

import (
	"context"
	"errors"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

type MatchFilter struct {
	ParticipantNames           []string
	ScheduledFrom, ScheduledTo *time.Time
	Limit                      int
}
type MatchView struct {
	ID                                   MatchID
	ScheduledAt                          time.Time
	HomeTeamName, AwayTeamName, Location string
	Status                               MatchStatus
}
type Result uint8

const (
	ResultPending Result = iota
	ResultHomeWin
	ResultAwayWin
	ResultDraw
	ResultCancelled
)

type MatchSummaryView struct {
	MatchView
	HomeScore, AwayScore *int
	Result               Result
}
type PlayerIdentityView struct {
	Name, TeamName string
	JerseyNumber   uint8
}
type SituationView struct {
	Outs                 uint8
	HomeScore, AwayScore uint16
	Runners              [3]*PlayerIdentityView
}
type PitchEventView struct {
	Sequence        uint16
	Pitcher, Batter PlayerIdentityView
	Result          PitchResult
	Balls, Strikes  uint8
	PitchType       string
	Velocity        *float64
	Zone            *uint8
	Description     string
}
type RunnerOutcomeView struct {
	Sequence            uint16
	Runner              PlayerIdentityView
	Result              RunnerResult
	FromBase            uint8
	ToBase              *uint8
	OutRecorded, Scored bool
	ChargedPitcher      *PlayerIdentityView
	Earned              *bool
	RBIBatter           *PlayerIdentityView
	Description         string
}
type FieldingOutcomeView struct {
	Sequence    uint16
	Fielder     PlayerIdentityView
	Position    player.PositionFlags
	Result      FieldingResult
	Description string
}
type PlayEventView struct {
	Sequence                uint32
	Inning                  uint16
	Half                    Half
	BattingOrder            uint8
	Batter, StartingPitcher PlayerIdentityView
	Situation               SituationView
	BattingResult           BattingResult
	ResultDescription       string
	Pitches                 []PitchEventView
	RunnerOutcomes          []RunnerOutcomeView
	FieldingOutcomes        []FieldingOutcomeView
}
type MatchRecordView struct {
	Summary MatchSummaryView
	Events  []PlayEventView
}
type MatchLineupsView struct {
	Match   MatchView
	Lineups []LineupView
}
type LineupView struct {
	TeamName      string
	Kind          LineupKind
	VariantNumber uint16
	VariantName   string
	Entries       []LineupEntryView
}
type LineupEntryView struct {
	Player       PlayerIdentityView
	BattingOrder uint8
	Position     player.PositionFlags
}
type OffenseLineView struct {
	Player                                                                                               PlayerIdentityView
	PA, AB, H, Singles, Doubles, Triples, HomeRuns, Walks, HitByPitch, Strikeouts, RBI, Runs, TotalBases uint
	AVG, OBP, SLG, OPS                                                                                   *float64
}
type PitchingLineView struct {
	Player                                                                                                                       PlayerIdentityView
	BattersFaced, Pitches, CalledStrikes, SwingingStrikes, Hits, HomeRuns, Walks, HitByPitch, Strikeouts, Runs, EarnedRuns, Outs uint
}
type FieldingLineView struct {
	Player                   PlayerIdentityView
	Putouts, Assists, Errors uint
}
type PerformanceLimitsView struct {
	FieldingOpportunitiesUnavailable bool
	EarnedRunsRequireExplicitMark    bool
	UnrecordedPitchFactsExcluded     bool
}
type MatchPlayerPerformanceView struct {
	Match    MatchView
	Offense  []OffenseLineView
	Pitching []PitchingLineView
	Fielding []FieldingLineView
	Limits   PerformanceLimitsView
}

type QueryRepository interface {
	ListMatches(context.Context, MatchFilter) ([]MatchView, error)
	ListPlays(context.Context, []MatchID) ([]PlayEventView, error)
	SummarizeMatches(context.Context, MatchFilter) ([]MatchSummaryView, error)
	GetMatchRecords(context.Context, MatchFilter) ([]MatchRecordView, error)
	ListMatchLineups(context.Context, MatchFilter) ([]MatchLineupsView, error)
	AnalyzeMatchPlayers(context.Context, MatchFilter) ([]MatchPlayerPerformanceView, error)
}
type QueryService struct{ repository QueryRepository }

func NewQueryService(repository QueryRepository) (*QueryService, error) {
	if repository == nil {
		return nil, errors.New("game query repository is required")
	}
	return &QueryService{repository: repository}, nil
}
func (s *QueryService) ListMatches(ctx context.Context, filter MatchFilter) ([]MatchView, error) {
	if err := validateMatchFilter(filter); err != nil {
		return nil, err
	}
	return s.repository.ListMatches(ctx, filter)
}
func (s *QueryService) ListPlays(ctx context.Context, matchIDs []MatchID) ([]PlayEventView, error) {
	if len(matchIDs) == 0 {
		return nil, errors.New("match ids are required")
	}
	return s.repository.ListPlays(ctx, matchIDs)
}
func (s *QueryService) SummarizeMatches(ctx context.Context, filter MatchFilter) ([]MatchSummaryView, error) {
	if err := validateMatchFilter(filter); err != nil {
		return nil, err
	}
	return s.repository.SummarizeMatches(ctx, filter)
}
func (s *QueryService) GetMatchRecords(ctx context.Context, filter MatchFilter) ([]MatchRecordView, error) {
	if err := validateMatchFilter(filter); err != nil {
		return nil, err
	}
	return s.repository.GetMatchRecords(ctx, filter)
}
func (s *QueryService) ListMatchLineups(ctx context.Context, filter MatchFilter) ([]MatchLineupsView, error) {
	if err := validateMatchFilter(filter); err != nil {
		return nil, err
	}
	return s.repository.ListMatchLineups(ctx, filter)
}
func (s *QueryService) AnalyzeMatchPlayers(ctx context.Context, filter MatchFilter) ([]MatchPlayerPerformanceView, error) {
	if err := validateMatchFilter(filter); err != nil {
		return nil, err
	}
	return s.repository.AnalyzeMatchPlayers(ctx, filter)
}
func validateMatchFilter(filter MatchFilter) error {
	if len(filter.ParticipantNames) > 2 {
		return errors.New("match filter accepts at most two participant names")
	}
	if filter.ScheduledFrom != nil && filter.ScheduledTo != nil && filter.ScheduledFrom.After(*filter.ScheduledTo) {
		return errors.New("match scheduled range is invalid")
	}
	if filter.Limit < 0 {
		return errors.New("match limit must not be negative")
	}
	return nil
}
