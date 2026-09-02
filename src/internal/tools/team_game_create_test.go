package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
)

func TestTeamGameCreateWritesOneCompleteRecord(t *testing.T) {
	value, err := newTeamGameCreateTool(gameRecordingRunnerStub{progress: game.GameCreateProgress{Status: "succeeded", HomeTeamName: "主队", AwayTeamName: "客队", CompletedLineups: 2, CompletedPlays: 1}})
	if err != nil {
		t.Fatal(err)
	}
	output, err := value.InvokableRun(context.Background(), `{"confirmed":true}`)
	if err != nil {
		t.Fatal(err)
	}
	var decoded gameCreateOutput
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != "succeeded" || decoded.ContextReceipt == nil || decoded.Progress.CompletedPlays != 1 {
		t.Fatalf("output=%#v", decoded)
	}
}

func TestTeamGameCreateRequiresConfirmation(t *testing.T) {
	value, err := newTeamGameCreateTool(gameRecordingRunnerStub{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := value.InvokableRun(context.Background(), `{"confirmed":false}`); err == nil {
		t.Fatal("unconfirmed create accepted")
	}
}

type gameRecordingRunnerStub struct{ progress game.GameCreateProgress }

func (s gameRecordingRunnerStub) Run(context.Context) (game.GameCreateProgress, error) {
	return s.progress, nil
}
