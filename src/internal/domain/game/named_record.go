package game

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

type PlayerReference struct {
	TeamName     string
	JerseyNumber int
}

type NamedSituationDraft struct {
	Outs, HomeScore, AwayScore int
	Runners                    [3]*PlayerReference
}

type NamedPitchDraft struct {
	Pitcher, Batter            PlayerReference
	Result                     PitchResult
	BallsBefore, StrikesBefore int
	BallsAfter, StrikesAfter   int
	PitchType                  string
	Velocity                   *float64
	Zone                       *int
	Description                string
}

type NamedRunnerOutcomeDraft struct {
	Runner              PlayerReference
	Result              RunnerResult
	FromBase            int
	ToBase              *int
	OutRecorded, Scored bool
	ChargedPitcher      *PlayerReference
	Earned              *bool
	RBIBatter           *PlayerReference
	Description         string
}

type NamedFieldingOutcomeDraft struct {
	Fielder     PlayerReference
	Position    player.PositionFlags
	Result      FieldingResult
	Description string
}

type NamedPlayDraft struct {
	Inning, BattingOrder    int
	Half                    Half
	Batter, StartingPitcher PlayerReference
	Before, After           NamedSituationDraft
	BattingResult           BattingResult
	ResultDescription       string
	Pitches                 []NamedPitchDraft
	RunnerOutcomes          []NamedRunnerOutcomeDraft
	FieldingOutcomes        []NamedFieldingOutcomeDraft
}

type NamedLineupEntryDraft struct {
	Player       PlayerReference
	BattingOrder int
	Position     player.PositionFlags
}

type NamedLineupDraft struct {
	Name    string
	Entries []NamedLineupEntryDraft
}

type NamedGameRecordDraft struct {
	HomeTeamName, AwayTeamName string
	ScheduledAt                string
	Location                   string
	HomeLineup, AwayLineup     NamedLineupDraft
	Plays                      []NamedPlayDraft
}

type GameCreateProgress struct {
	Status                 string `json:"status"`
	HomeTeamName           string `json:"home_team_name"`
	AwayTeamName           string `json:"away_team_name"`
	MatchCreated           bool   `json:"match_created"`
	CompletedLineups       int    `json:"completed_lineups"`
	CompletedPlays         int    `json:"completed_plays"`
	StoppedStage           string `json:"stopped_stage,omitempty"`
	StoppedPlayIndex       *int   `json:"stopped_play_index,omitempty"`
	Error                  string `json:"error,omitempty"`
	PartialDetailsPossible bool   `json:"partial_details_possible,omitempty"`
}

type resolvedPlayers struct {
	teams   map[string]team.Team
	players map[string]player.Player
}

func (s *Service) CreateNamedGameRecord(ctx context.Context, draft NamedGameRecordDraft) (GameCreateProgress, error) {
	if s.direct == nil || s.idGenerator == nil {
		return GameCreateProgress{}, errors.New("direct game repositories and id generator are required")
	}
	resolved, err := s.resolveNamedReferences(ctx, draft)
	if err != nil {
		return GameCreateProgress{}, err
	}
	home, err := resolved.team(draft.HomeTeamName)
	if err != nil {
		return GameCreateProgress{}, fmt.Errorf("resolve home team: %w", err)
	}
	away, err := resolved.team(draft.AwayTeamName)
	if err != nil {
		return GameCreateProgress{}, fmt.Errorf("resolve away team: %w", err)
	}
	if home.ID() == away.ID() {
		return GameCreateProgress{}, errors.New("home and away teams must differ")
	}
	now := s.clock.Now()
	scheduled, err := time.Parse(time.RFC3339, strings.TrimSpace(draft.ScheduledAt))
	if err != nil {
		return GameCreateProgress{}, err
	}
	matchID := MatchID(s.idGenerator())
	match, err := NewMatch(matchID, home.ID(), away.ID(), scheduled, draft.Location, MatchFinal, now)
	if err != nil {
		return GameCreateProgress{}, err
	}
	homeLineup, err := s.resolveLineup(resolved, matchID, home, draft.HomeLineup, now)
	if err != nil {
		return GameCreateProgress{}, fmt.Errorf("resolve home lineup: %w", err)
	}
	awayLineup, err := s.resolveLineup(resolved, matchID, away, draft.AwayLineup, now)
	if err != nil {
		return GameCreateProgress{}, fmt.Errorf("resolve away lineup: %w", err)
	}
	plays := make([]Play, 0, len(draft.Plays))
	for index, value := range draft.Plays {
		play, e := s.resolvePlay(resolved, matchID, index+1, value, now)
		if e != nil {
			return GameCreateProgress{}, fmt.Errorf("resolve play %d: %w", index+1, e)
		}
		plays = append(plays, play)
	}
	record := GameRecord{Match: match, Lineups: []Lineup{homeLineup, awayLineup}, Plays: plays}
	if err := validateGameRecord(record); err != nil {
		return GameCreateProgress{}, err
	}

	progress := GameCreateProgress{Status: "succeeded", HomeTeamName: home.Name(), AwayTeamName: away.Name()}
	if err := s.direct.Matches.Create(ctx, match); err != nil {
		return partialProgress(progress, "match", nil, err), nil
	}
	progress.MatchCreated = true
	for index, lineup := range record.Lineups {
		if err := s.direct.Lineups.Create(ctx, lineup); err != nil {
			return partialProgress(progress, "lineup", nil, err), nil
		}
		progress.CompletedLineups = index + 1
	}
	for index, play := range record.Plays {
		if err := s.direct.Plays.Create(ctx, play); err != nil {
			playNumber := index + 1
			return partialProgress(progress, "play", &playNumber, err), nil
		}
		progress.CompletedPlays = index + 1
	}
	return progress, nil
}

func partialProgress(progress GameCreateProgress, stage string, playIndex *int, err error) GameCreateProgress {
	progress.Status, progress.StoppedStage, progress.StoppedPlayIndex, progress.Error = "partial", stage, playIndex, err.Error()
	progress.PartialDetailsPossible = stage == "lineup" || stage == "play"
	return progress
}

func (r *resolvedPlayers) team(name string) (team.Team, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return team.Team{}, errors.New("team name is required")
	}
	if value, ok := r.teams[name]; ok {
		return value, nil
	}
	return team.Team{}, fmt.Errorf("team %q was not resolved", name)
}

func (r *resolvedPlayers) player(ref PlayerReference) (player.Player, error) {
	t, err := r.team(ref.TeamName)
	if err != nil {
		return player.Player{}, err
	}
	key := fmt.Sprintf("%s/%d", t.ID(), ref.JerseyNumber)
	if value, ok := r.players[key]; ok {
		return value, nil
	}
	return player.Player{}, fmt.Errorf("player %s #%d was not resolved", t.Name(), ref.JerseyNumber)
}

func (s *Service) resolveNamedReferences(ctx context.Context, draft NamedGameRecordDraft) (*resolvedPlayers, error) {
	homeName, awayName := strings.TrimSpace(draft.HomeTeamName), strings.TrimSpace(draft.AwayTeamName)
	if homeName == "" || awayName == "" {
		return nil, errors.New("home and away team names are required")
	}
	teamNames := []string{homeName}
	if awayName != homeName {
		teamNames = append(teamNames, awayName)
	}
	teams, err := s.direct.Teams.GetByNames(ctx, teamNames)
	if err != nil {
		return nil, fmt.Errorf("resolve teams: %w", err)
	}
	resolved := &resolvedPlayers{teams: make(map[string]team.Team, len(teams)), players: map[string]player.Player{}}
	for _, value := range teams {
		resolved.teams[value.Name()] = value
	}
	missingTeams := make([]string, 0)
	for _, name := range teamNames {
		if _, ok := resolved.teams[name]; !ok {
			missingTeams = append(missingTeams, name)
		}
	}
	if len(missingTeams) > 0 {
		return nil, fmt.Errorf("teams not found: %s", strings.Join(missingTeams, ", "))
	}
	if resolved.teams[homeName].ID() == resolved.teams[awayName].ID() {
		return nil, errors.New("home and away teams must differ")
	}

	references := collectPlayerReferences(draft)
	keysByString := make(map[string]PlayerJerseyKey)
	labels := make(map[string]string)
	for _, ref := range references {
		name := strings.TrimSpace(ref.TeamName)
		value, ok := resolved.teams[name]
		if !ok {
			return nil, fmt.Errorf("player team %q does not participate in match", name)
		}
		if ref.JerseyNumber < 0 || ref.JerseyNumber > 99 {
			return nil, fmt.Errorf("player %s jersey number must be between 0 and 99", name)
		}
		key := playerLookupKey(value.ID(), ref.JerseyNumber)
		keysByString[key] = PlayerJerseyKey{TeamID: value.ID(), JerseyNumber: ref.JerseyNumber}
		labels[key] = fmt.Sprintf("%s #%d", name, ref.JerseyNumber)
	}
	lookupKeys := make([]string, 0, len(keysByString))
	for key := range keysByString {
		lookupKeys = append(lookupKeys, key)
	}
	sort.Strings(lookupKeys)
	requests := make([]PlayerJerseyKey, 0, len(lookupKeys))
	for _, key := range lookupKeys {
		requests = append(requests, keysByString[key])
	}
	players, err := s.direct.Players.GetByTeamAndJerseys(ctx, requests)
	if err != nil {
		return nil, fmt.Errorf("resolve players: %w", err)
	}
	for _, value := range players {
		resolved.players[playerLookupKey(value.TeamID(), int(value.JerseyNumber()))] = value
	}
	missingPlayers := make([]string, 0)
	for _, key := range lookupKeys {
		if _, ok := resolved.players[key]; !ok {
			missingPlayers = append(missingPlayers, labels[key])
		}
	}
	if len(missingPlayers) > 0 {
		return nil, fmt.Errorf("players not found: %s", strings.Join(missingPlayers, ", "))
	}
	return resolved, nil
}

func playerLookupKey(teamID team.ID, jersey int) string { return fmt.Sprintf("%s/%d", teamID, jersey) }

func collectPlayerReferences(draft NamedGameRecordDraft) []PlayerReference {
	result := make([]PlayerReference, 0)
	add := func(ref *PlayerReference) {
		if ref != nil {
			result = append(result, *ref)
		}
	}
	for _, lineup := range []NamedLineupDraft{draft.HomeLineup, draft.AwayLineup} {
		for _, entry := range lineup.Entries {
			add(&entry.Player)
		}
	}
	for _, play := range draft.Plays {
		add(&play.Batter)
		add(&play.StartingPitcher)
		for _, situation := range []NamedSituationDraft{play.Before, play.After} {
			for _, runner := range situation.Runners {
				add(runner)
			}
		}
		for _, pitch := range play.Pitches {
			add(&pitch.Pitcher)
			add(&pitch.Batter)
		}
		for _, runner := range play.RunnerOutcomes {
			add(&runner.Runner)
			add(runner.ChargedPitcher)
			add(runner.RBIBatter)
		}
		for _, fielding := range play.FieldingOutcomes {
			add(&fielding.Fielder)
		}
	}
	return result
}

func (s *Service) resolveLineup(refs *resolvedPlayers, matchID MatchID, expectedTeam team.Team, draft NamedLineupDraft, nowTime time.Time) (Lineup, error) {
	entries := make([]LineupEntry, 0, len(draft.Entries))
	for _, entry := range draft.Entries {
		value, err := refs.player(entry.Player)
		if err != nil {
			return Lineup{}, err
		}
		if value.TeamID() != expectedTeam.ID() {
			return Lineup{}, errors.New("lineup player belongs to another team")
		}
		converted, err := NewLineupEntry(LineupID(s.idGenerator()), value.ID(), entry.BattingOrder, entry.Position)
		if err != nil {
			return Lineup{}, err
		}
		entries = append(entries, converted)
	}
	return NewLineup(matchID, expectedTeam.ID(), LineupStarter, 0, draft.Name, entries, nowTime)
}

func (s *Service) resolveSituation(refs *resolvedPlayers, draft NamedSituationDraft) (Situation, error) {
	var runners [3]*player.ID
	for index, ref := range draft.Runners {
		if ref == nil {
			continue
		}
		value, err := refs.player(*ref)
		if err != nil {
			return Situation{}, err
		}
		id := value.ID()
		runners[index] = &id
	}
	return NewSituation(draft.Outs, draft.HomeScore, draft.AwayScore, runners)
}

func (s *Service) resolvePlay(refs *resolvedPlayers, matchID MatchID, sequence int, draft NamedPlayDraft, now time.Time) (Play, error) {
	batter, err := refs.player(draft.Batter)
	if err != nil {
		return Play{}, err
	}
	pitcher, err := refs.player(draft.StartingPitcher)
	if err != nil {
		return Play{}, err
	}
	before, err := s.resolveSituation(refs, draft.Before)
	if err != nil {
		return Play{}, err
	}
	after, err := s.resolveSituation(refs, draft.After)
	if err != nil {
		return Play{}, err
	}
	pitches := make([]Pitch, 0, len(draft.Pitches))
	for index, value := range draft.Pitches {
		p, e := refs.player(value.Pitcher)
		if e != nil {
			return Play{}, e
		}
		b, e := refs.player(value.Batter)
		if e != nil {
			return Play{}, e
		}
		converted, e := NewPitch(PitchID(s.idGenerator()), index+1, p.ID(), b.ID(), value.Result, value.BallsBefore, value.StrikesBefore, value.BallsAfter, value.StrikesAfter, value.PitchType, value.Velocity, value.Zone, value.Description, now)
		if e != nil {
			return Play{}, e
		}
		pitches = append(pitches, converted)
	}
	runners := make([]RunnerOutcome, 0, len(draft.RunnerOutcomes))
	for index, value := range draft.RunnerOutcomes {
		runner, e := refs.player(value.Runner)
		if e != nil {
			return Play{}, e
		}
		charged, e := resolveOptionalPlayer(refs, value.ChargedPitcher)
		if e != nil {
			return Play{}, e
		}
		rbi, e := resolveOptionalPlayer(refs, value.RBIBatter)
		if e != nil {
			return Play{}, e
		}
		converted, e := NewRunnerOutcome(RunnerResultID(s.idGenerator()), index+1, runner.ID(), value.Result, value.FromBase, value.ToBase, value.OutRecorded, value.Scored, charged, value.Earned, rbi, value.Description, now)
		if e != nil {
			return Play{}, e
		}
		runners = append(runners, converted)
	}
	fielding := make([]FieldingOutcome, 0, len(draft.FieldingOutcomes))
	for index, value := range draft.FieldingOutcomes {
		fielder, e := refs.player(value.Fielder)
		if e != nil {
			return Play{}, e
		}
		converted, e := NewFieldingOutcome(FieldingResultID(s.idGenerator()), index+1, fielder.ID(), value.Position, value.Result, value.Description, now)
		if e != nil {
			return Play{}, e
		}
		fielding = append(fielding, converted)
	}
	return NewPlay(PlayDraft{ID: PlayID(s.idGenerator()), MatchID: matchID, Sequence: sequence, Inning: draft.Inning, Half: draft.Half, BattingOrder: draft.BattingOrder, BatterID: batter.ID(), StartingPitcherID: pitcher.ID(), Before: before, After: after, BattingResult: draft.BattingResult, ResultDescription: draft.ResultDescription, Pitches: pitches, RunnerOutcomes: runners, FieldingOutcomes: fielding}, now)
}

func resolveOptionalPlayer(refs *resolvedPlayers, ref *PlayerReference) (*player.ID, error) {
	if ref == nil {
		return nil, nil
	}
	value, err := refs.player(*ref)
	if err != nil {
		return nil, err
	}
	id := value.ID()
	return &id, nil
}
