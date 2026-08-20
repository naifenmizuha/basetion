package game

import (
	"testing"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
)

func TestExpandCompactGameRecordDerivesPitchCountAndSituation(t *testing.T) {
	draft := CompactGameRecordDraft{
		HomeTeamName: "主队", AwayTeamName: "客队", ScheduledAt: "2026-09-01T19:00:00+08:00",
		HomeLineup: CompactLineupDraft{Name: "主", Entries: []CompactLineupEntryDraft{{JerseyNumber: 1, BattingOrder: 1, Position: player.PositionPitcher}}},
		AwayLineup: CompactLineupDraft{Name: "客", Entries: []CompactLineupEntryDraft{{JerseyNumber: 2, BattingOrder: 1, Position: player.PositionFirstBase}}},
		Plays:      []CompactPlayDraft{{Inning: 1, Half: Top, BattingOrder: 1, BatterJerseyNumber: 2, PitcherJerseyNumber: 1, BattingResult: BattingStrikeout, ResultDescription: "三振", Pitches: []CompactPitchDraft{{Result: PitchCalledStrike}, {Result: PitchFoul}, {Result: PitchSwingingStrike}}}},
	}
	expanded, err := ExpandCompactGameRecord(draft)
	if err != nil {
		t.Fatal(err)
	}
	play := expanded.Plays[0]
	if play.Before.Outs != 0 || play.After.Outs != 1 || play.Pitches[2].StrikesBefore != 2 || play.Pitches[2].StrikesAfter != 3 {
		t.Fatalf("derived play=%#v", play)
	}
}

func TestExpandCompactGameRecordRejectsEarlyHalfChange(t *testing.T) {
	_, err := ExpandCompactGameRecord(CompactGameRecordDraft{HomeTeamName: "主", AwayTeamName: "客", ScheduledAt: "2026-09-01T19:00:00+08:00", Plays: []CompactPlayDraft{{Inning: 1, Half: Bottom}}})
	if err == nil {
		t.Fatal("accepted an invalid first half")
	}
}
