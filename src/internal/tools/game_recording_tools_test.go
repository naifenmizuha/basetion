package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/naifenmizuha/basetion/src/internal/domain/game"
)

func TestGameRecordingToolsAccumulateAndFinalize(t *testing.T) {
	service := &teamModifyTestService{gameProgress: game.GameCreateProgress{Status: "succeeded", CompletedPlays: 1}}
	intake := NewGameRecordingIntake("amber-otter")
	values, err := NewGameRecordingTools(service, intake)
	if err != nil {
		t.Fatal(err)
	}
	// Invoke through the concrete tool interface without depending on its order.
	lookup := func(name string) tool.InvokableTool {
		for _, value := range values {
			info, e := value.Info(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			if info.Name == name {
				return value
			}
		}
		t.Fatalf("missing %s", name)
		return nil
	}
	beginJSON, err := lookup(TeamGameBeginToolName).InvokableRun(context.Background(), `{"home_team_name":"主队","away_team_name":"客队","scheduled_at":"2026-09-01T19:00:00+08:00","home_lineup":{"name":"主队","entries":[{"jersey_number":1,"batting_order":1,"position":"pitcher"}]},"away_lineup":{"name":"客队","entries":[{"jersey_number":2,"batting_order":1,"position":"outfielder"}]}}`)
	if err != nil {
		t.Fatal(err)
	}
	var receipt gameBeginOutput
	if err := json.Unmarshal([]byte(beginJSON), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.IntakeID != "amber-otter" {
		t.Fatalf("id=%q", receipt.IntakeID)
	}
	_, err = lookup(TeamGameAppendPlaysToolName).InvokableRun(context.Background(), `{"intake_id":"amber-otter","plays":[{"inning":1,"half":"top","batting_order":1,"batter_jersey_number":2,"pitcher_jersey_number":1,"batting_result":"strikeout","result_description":"三振","pitches":[{"result":"swinging_strike"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := lookup(TeamGameFinalizeToolName).InvokableRun(context.Background(), `{"intake_id":"amber-otter"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !containsJSONStatus(result, "succeeded") {
		t.Fatalf("result=%s", result)
	}
	if _, ok := intake.Progress(); !ok {
		t.Fatal("final progress was not retained")
	}
}

func TestGameRecordingAppendRejectsUnknownAndMissingFields(t *testing.T) {
	for _, value := range []string{
		`{"intake_id":"amber-otter","plays":[{"inning":1,"half":"top","batting_order":1,"batter_jersey_number":2,"pitcher_jersey_number":1,"batting_result":"single","result_description":"安打","pitches":[{"type":"in_play"}]}]}`,
		`{"intake_id":"amber-otter","plays":[{"half":"top","batting_order":1,"batter_jersey_number":2,"pitcher_jersey_number":1,"batting_result":"single","result_description":"安打","pitches":[{"result":"in_play"}]}]}`,
	} {
		if err := validateAppendJSON(value); err == nil {
			t.Fatalf("invalid append accepted: %s", value)
		}
	}
}

func containsJSONStatus(value, want string) bool {
	var decoded struct {
		Status string `json:"status"`
	}
	return json.Unmarshal([]byte(value), &decoded) == nil && decoded.Status == want
}
