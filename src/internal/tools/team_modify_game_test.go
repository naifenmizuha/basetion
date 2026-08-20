package tools

import (
	"strings"
	"testing"
)

func TestGameCreateDescriptionIsNestedAndNameBased(t *testing.T) {
	operation := defaultModifyOperations()["game.create"]
	if operation.group != "game" || operation.resultType != "game_create_progress" || len(operation.parameters) != 7 {
		t.Fatalf("operation=%#v", operation)
	}
	if len(operation.conventions) == 0 || len(operation.invariants) == 0 {
		t.Fatalf("game.create lacks semantic guidance: %#v", operation)
	}
	for _, field := range operation.parameters {
		if field.Name == "home_team_id" || field.Name == "away_team_id" {
			t.Fatalf("game.create exposes ID field: %#v", field)
		}
		if (field.Name == "home_lineup" || field.Name == "away_lineup") && len(field.Fields) == 0 {
			t.Fatalf("lineup field is not described recursively: %#v", field)
		}
		if field.Name == "plays" && (field.Item == nil || len(field.Item.Fields) == 0) {
			t.Fatalf("plays field is not described recursively: %#v", field)
		}
	}
}

func TestGameCreateDescriptionExplainsAmbiguousPlayFields(t *testing.T) {
	handler := &teamModifyHandler{operations: defaultModifyOperations()}
	topics := handler.describe([]string{"game.create"})
	if len(topics) != 1 || len(topics[0].Conventions) == 0 || len(topics[0].Invariants) == 0 {
		t.Fatalf("topics=%#v", topics)
	}
	operation := handler.operations["game.create"]
	plays := findModifyField(operation.parameters, "plays")
	if plays == nil || plays.Item == nil {
		t.Fatalf("plays=%#v", plays)
	}
	runner := findModifyField(plays.Item.Fields, "runner_outcomes")
	if runner == nil || runner.Item == nil {
		t.Fatalf("runner_outcomes=%#v", runner)
	}
	fromBase := findModifyField(runner.Item.Fields, "from_base")
	if fromBase == nil || !strings.Contains(fromBase.Description, "0 表示打者") {
		t.Fatalf("from_base=%#v", fromBase)
	}
	if !strings.Contains(strings.Join(operation.conventions, "\n"), "服务端") || !strings.Contains(strings.Join(operation.invariants, "\n"), "首个 Play") {
		t.Fatalf("conventions=%q invariants=%q", operation.conventions, operation.invariants)
	}
}

func findModifyField(fields []modifyFieldDescription, name string) *modifyFieldDescription {
	for index := range fields {
		if fields[index].Name == name {
			return &fields[index]
		}
	}
	return nil
}

func TestGameCreateRejectsInternalIDs(t *testing.T) {
	err := validateGameArguments("game.create", map[string]any{"home_team_id": "hidden-id"})
	if err == nil {
		t.Fatal("game.create accepted an internal ID field")
	}
}

func TestConvertNamedGameRequiresThreeRunnerSlots(t *testing.T) {
	_, err := convertNamedGame(legacyGameCreateArguments{
		HomeTeamName: "蜀汉队", AwayTeamName: "曹魏队", ScheduledAt: "2026-09-01T19:00:00+08:00",
		HomeLineup: namedLineupArguments{Name: "主队首发", Entries: []namedLineupEntryArguments{{Player: namedPlayerArguments{TeamName: "蜀汉队", JerseyNumber: intPointer(18)}, BattingOrder: intPointer(1), Position: "first_base"}}},
		AwayLineup: namedLineupArguments{Name: "客队首发", Entries: []namedLineupEntryArguments{{Player: namedPlayerArguments{TeamName: "曹魏队", JerseyNumber: intPointer(2)}, BattingOrder: intPointer(1), Position: "pitcher"}}},
		Plays:      []namedPlayArguments{{Inning: intPointer(1), BattingOrder: intPointer(1), Half: "top", Batter: namedPlayerArguments{TeamName: "蜀汉队", JerseyNumber: intPointer(18)}, StartingPitcher: namedPlayerArguments{TeamName: "曹魏队", JerseyNumber: intPointer(2)}, Before: namedSituationArguments{Outs: intPointer(0), HomeScore: intPointer(0), AwayScore: intPointer(0), Runners: []*namedPlayerArguments{}}, After: namedSituationArguments{Outs: intPointer(1), HomeScore: intPointer(0), AwayScore: intPointer(0), Runners: []*namedPlayerArguments{nil, nil, nil}}, BattingResult: "strikeout", ResultDescription: "三振", Pitches: []namedPitchArguments{{Pitcher: namedPlayerArguments{TeamName: "曹魏队", JerseyNumber: intPointer(2)}, Batter: namedPlayerArguments{TeamName: "蜀汉队", JerseyNumber: intPointer(18)}, Result: "swinging_strike", BallsBefore: intPointer(0), StrikesBefore: intPointer(2), BallsAfter: intPointer(0), StrikesAfter: intPointer(3)}}}},
	})
	if err == nil {
		t.Fatal("situation with fewer than three runner slots was accepted")
	}
}

func intPointer(value int) *int { return &value }
