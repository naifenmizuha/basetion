package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

type fetchTrainingRepository struct {
	operation string
	players   []player.ID
	limit     int
}

type fetchTrainingPlayerReader struct{}

type fetchTrainingTeamReader struct{}

func (r *fetchTrainingRepository) List(context.Context, training.Filter) ([]training.Record, error) {
	return nil, nil
}

func (r *fetchTrainingRepository) ListByPlayers(_ context.Context, playerIDs []player.ID, _, _ *training.Date, limit int) ([]training.Record, error) {
	r.players = playerIDs
	r.limit = limit
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	date, dateErr := training.ParseDate("2026-08-01")
	if dateErr != nil {
		return nil, dateErr
	}
	first, err := training.New("internal-training", playerIDs[0], date, "短打练习 40 分钟", "触击方向稳定", now)
	if err != nil {
		return nil, err
	}
	return []training.Record{first}, nil
}

func (fetchTrainingPlayerReader) ListByName(_ context.Context, _ string) ([]player.Player, error) {
	value, err := player.New("internal-player", "internal-team", 7, "张三", player.HandRight, player.HandRight, player.PositionFirstBase, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return nil, err
	}
	return []player.Player{value}, nil
}

func (fetchTrainingPlayerReader) GetByIDs(_ context.Context, ids []player.ID) ([]player.Player, error) {
	values := make([]player.Player, 0, len(ids))
	for _, id := range ids {
		value, err := player.New(id, "internal-team", 7, "张三", player.HandRight, player.HandRight, player.PositionFirstBase, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (fetchTrainingTeamReader) GetByIDs(_ context.Context, ids []team.ID) (map[team.ID]string, error) {
	result := make(map[team.ID]string, len(ids))
	for _, id := range ids {
		result[id] = "蜀汉队"
	}
	return result, nil
}

type fetchGameRepository struct{ operation string }

func fetchMatchView() game.MatchView {
	return game.MatchView{ID: "internal-match", ScheduledAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), HomeTeamName: "蜀汉队", AwayTeamName: "魏国队", Location: "主场", Status: game.MatchFinal}
}
func (r *fetchGameRepository) ListMatches(context.Context, game.MatchFilter) ([]game.MatchView, error) {
	return []game.MatchView{fetchMatchView()}, nil
}
func (r *fetchGameRepository) ListPlays(context.Context, []game.MatchID) ([]game.PlayEventView, error) {
	return nil, nil
}
func (r *fetchGameRepository) SummarizeMatches(context.Context, game.MatchFilter) ([]game.MatchSummaryView, error) {
	r.operation = "game.summaries"
	return []game.MatchSummaryView{{MatchView: fetchMatchView(), Result: game.ResultHomeWin}}, nil
}
func (r *fetchGameRepository) GetMatchRecords(context.Context, game.MatchFilter) ([]game.MatchRecordView, error) {
	r.operation = "game.records"
	return []game.MatchRecordView{{Summary: game.MatchSummaryView{MatchView: fetchMatchView()}}}, nil
}
func (r *fetchGameRepository) ListMatchLineups(context.Context, game.MatchFilter) ([]game.MatchLineupsView, error) {
	r.operation = "game.lineups"
	return []game.MatchLineupsView{{Match: fetchMatchView()}}, nil
}
func (r *fetchGameRepository) AnalyzeMatchPlayers(context.Context, game.MatchFilter) ([]game.MatchPlayerPerformanceView, error) {
	r.operation = "game.performances"
	return []game.MatchPlayerPerformanceView{{Match: fetchMatchView(), Limits: game.PerformanceLimitsView{FieldingOpportunitiesUnavailable: true}}}, nil
}

func newTestTeamFetchTool(t *testing.T) (*fetchGameRepository, tool.InvokableTool) {
	t.Helper()
	repository := &fetchGameRepository{}
	service, err := game.NewQueryService(repository)
	if err != nil {
		t.Fatal(err)
	}
	trainings, err := training.NewQueryService(&fetchTrainingRepository{}, training.WithPlayerReader(&fetchTrainingPlayerReader{}), training.WithTeamReader(&fetchTrainingTeamReader{}))
	if err != nil {
		t.Fatal(err)
	}
	value, err := NewTeamFetch(service, trainings)
	if err != nil {
		t.Fatal(err)
	}
	return repository, value
}

func TestTeamFetchExecutesEveryCompositeRead(t *testing.T) {
	t.Parallel()
	repository, value := newTestTeamFetchTool(t)
	for _, operation := range []string{"game.summaries", "game.records", "game.lineups", "game.performances"} {
		output, err := value.InvokableRun(context.Background(), `{"operation":"`+operation+`","arguments":{"limit":1}}`)
		if err != nil {
			t.Fatalf("%s: %v", operation, err)
		}
		var decoded teamFetchOutput
		if err := json.Unmarshal([]byte(output), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Operation != operation || repository.operation != operation {
			t.Fatalf("operation=%s output=%#v repository=%s", operation, decoded, repository.operation)
		}
		if string(output) == "" || containsInternalMatchID(string(output)) {
			t.Fatalf("internal id leaked in %s: %s", operation, output)
		}
	}
}

func TestTeamFetchRejectsInvalidOperationsAndUnboundedLimit(t *testing.T) {
	t.Parallel()
	_, value := newTestTeamFetchTool(t)
	for _, input := range []string{
		`{"operation":"game.performances","arguments":{"limit":0}}`,
		`{"operation":"unknown"}`,
		`{"arguments":{}}`,
	} {
		if _, err := value.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func TestTeamFetchTrainingRecords(t *testing.T) {
	t.Parallel()
	_, value := newTestTeamFetchTool(t)

	output, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"player_name":"张三","limit":50}}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "internal-training") {
		t.Fatalf("internal id leaked: %s", output)
	}
	if !strings.Contains(output, "短打练习 40 分钟") {
		t.Fatalf("missing training content: %s", output)
	}

	if _, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"unknown_key":"x"}}`); err == nil {
		t.Fatal("unknown argument accepted")
	}
	if _, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"player_name":""}}`); err == nil {
		t.Fatal("empty player_name accepted")
	}
	if _, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"limit":0}}`); err == nil {
		t.Fatal("limit 0 accepted")
	}
	if _, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"limit":501}}`); err == nil {
		t.Fatal("limit 501 accepted")
	}
	if _, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"date_from":"2026-08-10","date_to":"2026-08-01"}}`); err == nil {
		t.Fatal("reversed date range accepted")
	}
	if _, err := value.InvokableRun(context.Background(), `{"operation":"training.records","arguments":{"date_from":"not-a-date"}}`); err == nil {
		t.Fatal("invalid date accepted")
	}
}

func TestTeamFetchSchemaIsExecuteOnly(t *testing.T) {
	_, value := newTestTeamFetchTool(t)
	info, err := value.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := parameters.Properties.Get("operation"); !ok {
		t.Fatalf("missing operation: %#v", parameters)
	}
	for _, field := range []string{"mode", "topics", "program", "confirmed", "operations"} {
		if _, ok := parameters.Properties.Get(field); ok {
			t.Fatalf("unexpected %q: %#v", field, parameters)
		}
	}
}

func containsInternalMatchID(value string) bool { return strings.Contains(value, "internal-match") }
