package teamquery

import (
	"context"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
)

type TeamView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type RosterPlayerFilter struct {
	TeamID      string
	OnDate      *roster.Date
	PositionAny player.PositionFlags
}

type RosterPlayerView struct {
	MembershipID  string   `json:"membership_id"`
	TeamID        string   `json:"team_id"`
	PlayerID      string   `json:"player_id"`
	Name          string   `json:"name"`
	JerseyNumber  int      `json:"jersey_number"`
	BattingHands  []string `json:"batting_hands"`
	ThrowingHands []string `json:"throwing_hands"`
	Positions     []string `json:"positions"`
	JoinedAt      string   `json:"joined_at"`
	LeftAt        *string  `json:"left_at"`
	Active        bool     `json:"active"`
}

type RosterReader interface {
	ListTeams(context.Context, bool) ([]TeamView, error)
	ListPlayers(context.Context, RosterPlayerFilter) ([]RosterPlayerView, error)
}
