package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
)

var modifyTestNow = time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)

type modifyStub struct {
	lastOperation string
	lastID        string
	operations    []string
	failOperation string
}

func (s *modifyStub) record(operation, id string) error {
	s.lastOperation, s.lastID = operation, id
	s.operations = append(s.operations, operation)
	if s.failOperation == operation {
		return errors.New("injected failure")
	}
	return nil
}

func (s *modifyStub) Create(ctx context.Context, id team.ID, name string) (team.Team, error) {
	if err := s.record("team.create", string(id)); err != nil {
		return team.Team{}, err
	}
	return team.New(id, name, modifyTestNow)
}

func (s *modifyStub) CreatePlayer(ctx context.Context, id player.ID, name string, batting, throwing player.HandFlags, positions player.PositionFlags) (player.Player, error) {
	return player.New(id, name, batting, throwing, positions, modifyTestNow)
}

type playerModifyStub struct{ state *modifyStub }

func (s playerModifyStub) Create(ctx context.Context, id player.ID, name string, batting, throwing player.HandFlags, positions player.PositionFlags) (player.Player, error) {
	if err := s.state.record("player.create", string(id)); err != nil {
		return player.Player{}, err
	}
	return player.New(id, name, batting, throwing, positions, modifyTestNow)
}
func (s playerModifyStub) Update(ctx context.Context, id player.ID, name string, batting, throwing player.HandFlags, positions player.PositionFlags) (player.Player, error) {
	if err := s.state.record("player.update", string(id)); err != nil {
		return player.Player{}, err
	}
	value, err := player.New(id, name, batting, throwing, positions, modifyTestNow)
	if err == nil {
		err = value.UpdateProfile(name, batting, throwing, positions, modifyTestNow.Add(time.Minute))
	}
	return value, err
}
func (s playerModifyStub) SetActive(ctx context.Context, id player.ID, active bool) (player.Player, error) {
	if err := s.state.record("player.set_active", string(id)); err != nil {
		return player.Player{}, err
	}
	value, err := player.New(id, "测试球员", player.HandRight, player.HandRight, player.PositionPitcher, modifyTestNow)
	if err == nil {
		err = value.SetActive(active, modifyTestNow.Add(time.Minute))
	}
	return value, err
}

type rosterModifyStub struct{ state *modifyStub }

func (s rosterModifyStub) Assign(ctx context.Context, id roster.ID, teamID team.ID, playerID player.ID, jersey int, joined roster.Date) (roster.Membership, error) {
	if err := s.state.record("roster.assign", string(id)); err != nil {
		return roster.Membership{}, err
	}
	return roster.New(id, teamID, playerID, jersey, joined, modifyTestNow)
}
func (s rosterModifyStub) ChangeJersey(ctx context.Context, teamID team.ID, id roster.ID, jersey int) (roster.Membership, error) {
	if err := s.state.record("roster.change_jersey", string(id)); err != nil {
		return roster.Membership{}, err
	}
	joined, _ := roster.ParseDate("2026-08-01")
	value, err := roster.New(id, teamID, player.ID("00000000-0000-0000-0000-000000000003"), 8, joined, modifyTestNow)
	if err == nil {
		err = value.ChangeJersey(jersey, modifyTestNow.Add(time.Minute))
	}
	return value, err
}
func (s rosterModifyStub) Leave(ctx context.Context, teamID team.ID, id roster.ID, left roster.Date) (roster.Membership, error) {
	if err := s.state.record("roster.leave", string(id)); err != nil {
		return roster.Membership{}, err
	}
	joined, _ := roster.ParseDate("2026-08-01")
	value, err := roster.New(id, teamID, player.ID("00000000-0000-0000-0000-000000000003"), 8, joined, modifyTestNow)
	if err == nil {
		err = value.Leave(left, roster.DateFromTime(modifyTestNow), modifyTestNow.Add(time.Minute))
	}
	return value, err
}

func newTestTeamModifyTool(t *testing.T) (*modifyStub, tool.InvokableTool) {
	t.Helper()
	state := &modifyStub{}
	modifyTool, err := NewTeamModify(state, playerModifyStub{state}, rosterModifyStub{state}, WithTeamModifyIDGenerator(func() string { return "00000000-0000-0000-0000-000000000001" }))
	if err != nil {
		t.Fatal(err)
	}
	return state, modifyTool
}

func TestTeamModifyDescribe(t *testing.T) {
	t.Parallel()
	_, modifyTool := newTestTeamModifyTool(t)
	rootJSON, err := modifyTool.InvokableRun(context.Background(), `{"mode":"describe"}`)
	if err != nil {
		t.Fatal(err)
	}
	var root teamModifyOutput
	if err := json.Unmarshal([]byte(rootJSON), &root); err != nil {
		t.Fatal(err)
	}
	if len(root.Topics) != 3 || root.Topics[0].Name != "team" || len(root.Topics[1].Children) != 3 || len(root.Topics[2].Children) != 3 {
		t.Fatalf("root=%#v", root)
	}
	leafJSON, err := modifyTool.InvokableRun(context.Background(), `{"mode":"describe","topics":["roster.assign","unknown","roster.assign"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var leaf teamModifyOutput
	if err := json.Unmarshal([]byte(leafJSON), &leaf); err != nil {
		t.Fatal(err)
	}
	if len(leaf.Topics) != 2 || !leaf.Topics[0].Found || len(leaf.Topics[0].Parameters) != 4 || leaf.Topics[1].Found {
		t.Fatalf("leaf=%#v", leaf)
	}
}

func TestTeamModifyExecutesAllOperations(t *testing.T) {
	t.Parallel()
	state, modifyTool := newTestTeamModifyTool(t)
	tests := []struct{ operation, arguments, wantID string }{
		{"team.create", `{"name":"一队"}`, "00000000-0000-0000-0000-000000000001"},
		{"player.create", `{"name":"甲","batting_hands":["left"],"throwing_hands":["right"],"positions":["pitcher","outfielder"]}`, "00000000-0000-0000-0000-000000000001"},
		{"player.update", `{"player_id":"00000000-0000-0000-0000-000000000003","name":"乙","batting_hands":["right"],"throwing_hands":["right"],"positions":["catcher"]}`, "00000000-0000-0000-0000-000000000003"},
		{"player.set_active", `{"player_id":"00000000-0000-0000-0000-000000000003","active":false}`, "00000000-0000-0000-0000-000000000003"},
		{"roster.assign", `{"team_id":"00000000-0000-0000-0000-000000000002","player_id":"00000000-0000-0000-0000-000000000003","jersey_number":18,"joined_at":"2026-08-01"}`, "00000000-0000-0000-0000-000000000001"},
		{"roster.change_jersey", `{"team_id":"00000000-0000-0000-0000-000000000002","membership_id":"00000000-0000-0000-0000-000000000004","jersey_number":19}`, "00000000-0000-0000-0000-000000000004"},
		{"roster.leave", `{"team_id":"00000000-0000-0000-0000-000000000002","membership_id":"00000000-0000-0000-0000-000000000004","left_at":"2026-08-16"}`, "00000000-0000-0000-0000-000000000004"},
	}
	for _, test := range tests {
		input := `{"mode":"execute","operation":"` + test.operation + `","arguments":` + test.arguments + `,"confirmed":true}`
		output, err := modifyTool.InvokableRun(context.Background(), input)
		if err != nil {
			t.Fatalf("%s: %v", test.operation, err)
		}
		if state.lastOperation != test.operation || state.lastID != test.wantID || !strings.Contains(output, `"operation":"`+test.operation+`"`) {
			t.Fatalf("%s: state=%#v output=%s", test.operation, state, output)
		}
	}
}

func TestTeamModifyExecutesBatchInOrder(t *testing.T) {
	t.Parallel()
	state, modifyTool := newTestTeamModifyTool(t)
	outputJSON, err := modifyTool.InvokableRun(context.Background(), `{
		"mode":"execute",
		"operations":[
			{"key":"create-team","operation":"team.create","arguments":{"name":"一队"}},
			{"key":"create-player","operation":"player.create","arguments":{"name":"甲","batting_hands":["left"],"throwing_hands":["right"],"positions":["pitcher"]}},
			{"key":"disable-player","operation":"player.set_active","arguments":{"player_id":"00000000-0000-0000-0000-000000000003","active":false}}
		],
		"confirmed":true
	}`)
	if err != nil {
		t.Fatal(err)
	}
	var output teamModifyOutput
	if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
		t.Fatal(err)
	}
	wantOperations := []string{"team.create", "player.create", "player.set_active"}
	if output.Status != "succeeded" || output.StoppedAt != nil || len(output.Results) != 3 {
		t.Fatalf("output=%#v", output)
	}
	if strings.Join(state.operations, ",") != strings.Join(wantOperations, ",") {
		t.Fatalf("operations=%v", state.operations)
	}
	for index, result := range output.Results {
		if result.Index != index || result.Status != "succeeded" || result.Operation != wantOperations[index] {
			t.Fatalf("result[%d]=%#v", index, result)
		}
	}
}

func TestTeamModifyBatchStopsAndReportsFailure(t *testing.T) {
	t.Parallel()
	state, modifyTool := newTestTeamModifyTool(t)
	state.failOperation = "player.set_active"
	outputJSON, err := modifyTool.InvokableRun(context.Background(), `{
		"mode":"execute",
		"operations":[
			{"key":"create-team","operation":"team.create","arguments":{"name":"一队"}},
			{"key":"disable-player","operation":"player.set_active","arguments":{"player_id":"00000000-0000-0000-0000-000000000003","active":false}},
			{"key":"leave-player","operation":"roster.leave","arguments":{"team_id":"00000000-0000-0000-0000-000000000002","membership_id":"00000000-0000-0000-0000-000000000004","left_at":"2026-08-16"}}
		],
		"confirmed":true
	}`)
	if err != nil {
		t.Fatal(err)
	}
	var output teamModifyOutput
	if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "stopped" || output.StoppedAt == nil || output.StoppedAt.Index != 1 || output.StoppedAt.Key != "disable-player" || output.StoppedAt.Reason != "injected failure" {
		t.Fatalf("output=%#v", output)
	}
	if len(output.Results) != 3 || output.Results[0].Status != "succeeded" || output.Results[1].Status != "failed" || output.Results[2].Status != "skipped" {
		t.Fatalf("results=%#v", output.Results)
	}
	if strings.Join(state.operations, ",") != "team.create,player.set_active" {
		t.Fatalf("operations=%v", state.operations)
	}
}

func TestTeamModifyBatchValidatesBeforeWriting(t *testing.T) {
	t.Parallel()
	tests := []string{
		`{"mode":"execute","operations":[],"confirmed":true}`,
		`{"mode":"execute","operation":"team.create","arguments":{"name":"一队"},"operations":[{"key":"other","operation":"team.create","arguments":{"name":"二队"}}],"confirmed":true}`,
		`{"mode":"execute","operations":[{"key":"same","operation":"team.create","arguments":{"name":"一队"}},{"key":"same","operation":"team.create","arguments":{"name":"二队"}}],"confirmed":true}`,
		`{"mode":"execute","operations":[{"key":"valid","operation":"team.create","arguments":{"name":"一队"}},{"key":"invalid","operation":"roster.leave","arguments":{"team_id":"t","membership_id":"m","left_at":"bad"}}],"confirmed":true}`,
	}
	for _, input := range tests {
		state, modifyTool := newTestTeamModifyTool(t)
		if _, err := modifyTool.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid batch accepted: %s", input)
		}
		if len(state.operations) != 0 {
			t.Fatalf("batch wrote before validation completed: operations=%v input=%s", state.operations, input)
		}
	}
}

func TestTeamModifyRejectsUnsafeOrInvalidInput(t *testing.T) {
	t.Parallel()
	_, modifyTool := newTestTeamModifyTool(t)
	tests := []string{
		`{"mode":"execute","operation":"team.create","arguments":{"name":"一队"}}`,
		`{"mode":"execute","operation":"unknown","arguments":{},"confirmed":true}`,
		`{"mode":"execute","operation":"team.create","arguments":{"name":"一队","extra":1},"confirmed":true}`,
		`{"mode":"execute","operation":"player.create","arguments":{"name":"甲","batting_hands":["switch"],"throwing_hands":["right"],"positions":["pitcher"]},"confirmed":true}`,
		`{"mode":"execute","operation":"roster.assign","arguments":{"team_id":"t","player_id":"p","jersey_number":1,"joined_at":"bad"},"confirmed":true}`,
		`{"mode":"describe","operation":"team.create"}`,
	}
	for _, input := range tests {
		if _, err := modifyTool.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func TestNewTeamModifyRequiresDependencies(t *testing.T) {
	t.Parallel()
	if _, err := NewTeamModify(nil, nil, nil); err == nil {
		t.Fatal("nil services accepted")
	}
}
