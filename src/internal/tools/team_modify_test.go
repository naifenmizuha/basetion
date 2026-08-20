package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

type teamModifyTestService struct {
	teamCreateCalls int
	failTeamCreate  int
	gameProgress    game.GameCreateProgress
}

func (s *teamModifyTestService) Create(_ context.Context, id team.ID, name string) (team.Team, error) {
	s.teamCreateCalls++
	if s.teamCreateCalls == s.failTeamCreate {
		return team.Team{}, errors.New("team create failed")
	}
	return team.New(id, name, time.Now())
}

type testPlayerModifier struct{}

func (testPlayerModifier) Create(context.Context, player.ID, team.ID, int, string, player.HandFlags, player.HandFlags, player.PositionFlags) (player.Player, error) {
	return player.Player{}, errors.New("not implemented")
}

func (testPlayerModifier) Update(context.Context, player.ID, string, player.HandFlags, player.HandFlags, player.PositionFlags) (player.Player, error) {
	return player.Player{}, errors.New("not implemented")
}

func (testPlayerModifier) SetActive(context.Context, player.ID, bool) (player.Player, error) {
	return player.Player{}, errors.New("not implemented")
}

func (testPlayerModifier) ChangeJersey(context.Context, player.ID, int) (player.Player, error) {
	return player.Player{}, errors.New("not implemented")
}

func (s *teamModifyTestService) CreateCompactGameRecord(context.Context, game.CompactGameRecordDraft) (game.GameCreateProgress, error) {
	return s.gameProgress, nil
}

func (*teamModifyTestService) CreateNamedGameRecord(context.Context, game.NamedGameRecordDraft) (game.GameCreateProgress, error) {
	return game.GameCreateProgress{}, errors.New("not implemented")
}

func (*teamModifyTestService) CreateMatchWith(context.Context, game.MatchID, team.ID, team.ID, time.Time, string, game.MatchStatus) (game.Match, error) {
	return game.Match{}, errors.New("not implemented")
}

func (*teamModifyTestService) UpdateMatch(context.Context, game.MatchID, team.ID, team.ID, time.Time, string) (game.Match, error) {
	return game.Match{}, errors.New("not implemented")
}

func (*teamModifyTestService) SetMatchStatus(context.Context, game.MatchID, game.MatchStatus) (game.Match, error) {
	return game.Match{}, errors.New("not implemented")
}

func (*teamModifyTestService) DeleteMatch(context.Context, game.MatchID) error {
	return errors.New("not implemented")
}

func (*teamModifyTestService) CreateLineupWith(context.Context, game.MatchID, team.ID, game.LineupKind, int, string, []game.LineupEntry) (game.Lineup, error) {
	return game.Lineup{}, errors.New("not implemented")
}

func (*teamModifyTestService) ReplaceLineupWith(context.Context, game.MatchID, team.ID, game.LineupKind, int, string, []game.LineupEntry) (game.Lineup, error) {
	return game.Lineup{}, errors.New("not implemented")
}

func (*teamModifyTestService) DeleteLineup(context.Context, game.MatchID, team.ID, game.LineupKind, uint16) error {
	return errors.New("not implemented")
}

type testTrainingModifier struct{}

func (testTrainingModifier) Create(context.Context, training.ID, player.ID, training.Date, string, string) (training.Record, error) {
	return training.Record{}, errors.New("not implemented")
}

func (testTrainingModifier) Update(context.Context, training.ID, string, string) (training.Record, error) {
	return training.Record{}, errors.New("not implemented")
}

func (testTrainingModifier) Delete(context.Context, training.ID) error {
	return errors.New("not implemented")
}

func newTestTeamModifyTool(t *testing.T, service *teamModifyTestService) tool.InvokableTool {
	t.Helper()
	value, err := NewTeamModify(service, testPlayerModifier{}, service, testTrainingModifier{}, WithTeamModifyIDGenerator(func() string { return "generated-id" }))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTeamModifySchemaIsExecuteOnly(t *testing.T) {
	value := newTestTeamModifyTool(t, &teamModifyTestService{})
	info, err := value.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != TeamModifyToolName {
		t.Fatalf("tool name=%q", info.Name)
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"confirmed", "operations"} {
		if _, ok := parameters.Properties.Get(field); !ok {
			t.Fatalf("missing %q: %#v", field, parameters)
		}
	}
	for _, field := range []string{"topics", "mode", "operation", "arguments"} {
		if _, ok := parameters.Properties.Get(field); ok {
			t.Fatalf("unexpected %q: %#v", field, parameters)
		}
	}
}

func TestTeamModifyExecuteUsesOperationsForSingleAndBatch(t *testing.T) {
	service := &teamModifyTestService{}
	value := newTestTeamModifyTool(t, service)
	output, err := value.InvokableRun(context.Background(), `{"confirmed":true,"operations":[{"key":"first","operation":"team.create","arguments":{"name":"江东队"}},{"key":"second","operation":"team.create","arguments":{"name":"蜀汉队"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded teamModifyOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "succeeded" || len(decoded.Results) != 2 || decoded.Results[0].Status != "succeeded" || decoded.Results[1].Status != "succeeded" || service.teamCreateCalls != 2 {
		t.Fatalf("output=%#v calls=%d", decoded, service.teamCreateCalls)
	}
}

func TestTeamModifyExecuteRejectsInvalidRequests(t *testing.T) {
	value := newTestTeamModifyTool(t, &teamModifyTestService{})
	for _, input := range []string{
		`{"confirmed":false,"operations":[{"key":"one","operation":"team.create","arguments":{"name":"江东队"}}]}`,
		`{"confirmed":true,"operations":[]}`,
		`{"confirmed":true,"operations":[{"key":"same","operation":"team.create","arguments":{"name":"江东队"}},{"key":"same","operation":"team.create","arguments":{"name":"蜀汉队"}}]}`,
		`{"confirmed":true,"operations":[{"key":"unknown","operation":"unknown","arguments":{}}]}`,
		`{"confirmed":true,"operations":[{"key":"missing","operation":"team.create"}]}`,
	} {
		if _, err := value.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func TestTeamModifyExecuteStopsBatchAndRetainsGameCreateProgress(t *testing.T) {
	service := &teamModifyTestService{failTeamCreate: 2}
	value := newTestTeamModifyTool(t, service)
	output, err := value.InvokableRun(context.Background(), `{"confirmed":true,"operations":[{"key":"first","operation":"team.create","arguments":{"name":"江东队"}},{"key":"second","operation":"team.create","arguments":{"name":"蜀汉队"}},{"key":"third","operation":"team.create","arguments":{"name":"魏国队"}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	var stopped teamModifyOutput
	if err := json.Unmarshal([]byte(output), &stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "stopped" || stopped.StoppedAt == nil || stopped.StoppedAt.Index != 1 || len(stopped.Results) != 3 || stopped.Results[0].Status != "succeeded" || stopped.Results[1].Status != "failed" || stopped.Results[2].Status != "skipped" {
		t.Fatalf("stopped output=%#v", stopped)
	}

	const gameInput = `{"confirmed":true,"operations":[{"key":"record-game","operation":"game.create","arguments":{"home_team_name":"主队","away_team_name":"客队","scheduled_at":"2026-09-01T19:00:00+08:00","location":"主场","home_lineup":{"name":"主队首发","entries":[{"jersey_number":1,"batting_order":1,"position":"pitcher"}]},"away_lineup":{"name":"客队首发","entries":[{"jersey_number":2,"batting_order":1,"position":"first_base"}]},"plays":[{"inning":1,"half":"top","batting_order":1,"batter_jersey_number":2,"pitcher_jersey_number":1,"batting_result":"strikeout","result_description":"三振","pitches":[{"result":"swinging_strike"}]}]}}]}`
	service = &teamModifyTestService{gameProgress: game.GameCreateProgress{Status: "succeeded", HomeTeamName: "主队", AwayTeamName: "客队", MatchCreated: true, CompletedLineups: 2, CompletedPlays: 4}}
	value = newTestTeamModifyTool(t, service)
	output, err = value.InvokableRun(context.Background(), gameInput)
	if err != nil {
		t.Fatal(err)
	}
	var succeeded teamModifyOutput
	if err := json.Unmarshal([]byte(output), &succeeded); err != nil {
		t.Fatal(err)
	}
	if succeeded.Status != "succeeded" || len(succeeded.Results) != 1 || succeeded.Results[0].Status != "succeeded" || succeeded.ContextReceipt == nil || succeeded.ContextReceipt.Operation != "game.create" {
		t.Fatalf("succeeded output=%#v", succeeded)
	}

	service = &teamModifyTestService{gameProgress: game.GameCreateProgress{Status: "partial", Error: "write play failed", MatchCreated: true, CompletedLineups: 2, CompletedPlays: 4}}
	value = newTestTeamModifyTool(t, service)
	output, err = value.InvokableRun(context.Background(), gameInput)
	if err != nil {
		t.Fatal(err)
	}
	var partial teamModifyOutput
	if err := json.Unmarshal([]byte(output), &partial); err != nil {
		t.Fatal(err)
	}
	if partial.Status != "stopped" || partial.StoppedAt == nil || partial.Results[0].Status != "failed" || !strings.Contains(partial.Results[0].Error, "write play failed") {
		t.Fatalf("partial output=%#v", partial)
	}
}
