package teamquery

import (
	"context"
	"testing"
	"time"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	domain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
)

type trainingQueryStub struct{ filter training.Filter }

func (s *trainingQueryStub) List(_ context.Context, filter training.Filter) ([]training.Record, error) {
	s.filter = filter
	date, _ := training.ParseDate("2026-08-17")
	value, _ := training.New("record-1", filter.PlayerID, date, "打击训练", "状态不错", time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC))
	return []training.Record{value}, nil
}

func TestTrainingProxyListsRecords(t *testing.T) {
	repository := &trainingQueryStub{}
	service, _ := training.NewQueryService(repository)
	executor, err := NewLuaExecutor(DefaultLimits(), WithTrainingService(service))
	if err != nil {
		t.Fatal(err)
	}
	result, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"training"}, Program: `function main(team) return team.training.records({player_id="player-1",from_date="2026-08-01",to_date="2026-08-31"}) end`})
	if err != nil {
		t.Fatal(err)
	}
	values := result.([]any)
	if len(values) != 1 || values[0].(map[string]any)["training_date"] != "2026-08-17" || repository.filter.PlayerID != player.ID("player-1") || repository.filter.From == nil || repository.filter.To == nil {
		t.Fatalf("result=%#v filter=%#v", result, repository.filter)
	}
	if _, err := executor.Execute(context.Background(), domain.Query{Modules: []string{"training"}, Program: `function main(team) team.training.records = 1 end`}); err == nil {
		t.Fatal("training proxy mutation accepted")
	}
}
