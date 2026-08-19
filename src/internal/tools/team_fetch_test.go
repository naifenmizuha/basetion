package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
)

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
	value, err := NewTeamFetch(service)
	if err != nil {
		t.Fatal(err)
	}
	return repository, value
}

func TestTeamFetchDescribesAndExecutesEveryCompositeRead(t *testing.T) {
	t.Parallel()
	repository, value := newTestTeamFetchTool(t)
	description, err := value.InvokableRun(context.Background(), `{"mode":"describe","topics":["game.performances","unknown"]}`)
	if err != nil {
		t.Fatal(err)
	}
	var described teamFetchOutput
	if err := json.Unmarshal([]byte(description), &described); err != nil {
		t.Fatal(err)
	}
	if described.Mode != "describe" || len(described.Topics) != 2 || !described.Topics[0].Found || described.Topics[1].Found {
		t.Fatalf("description=%#v", described)
	}
	for _, operation := range []string{"game.summaries", "game.records", "game.lineups", "game.performances"} {
		output, err := value.InvokableRun(context.Background(), `{"mode":"fetch","operation":"`+operation+`","arguments":{"limit":1}}`)
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

func TestTeamFetchRejectsProgramsAndUnboundedLimit(t *testing.T) {
	t.Parallel()
	_, value := newTestTeamFetchTool(t)
	for _, input := range []string{
		`{"mode":"fetch","operation":"game.performances","arguments":{"limit":0}}`,
		`{"mode":"fetch","operation":"unknown"}`,
		`{"mode":"describe","operation":"game.records"}`,
	} {
		if _, err := value.InvokableRun(context.Background(), input); err == nil {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func containsInternalMatchID(value string) bool { return strings.Contains(value, "internal-match") }
