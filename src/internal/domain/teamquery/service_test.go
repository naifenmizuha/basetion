package teamquery

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubExecutor struct {
	query     Query
	available []string
	result    any
	err       error
}

func (e *stubExecutor) AvailableModules() []string { return e.available }
func (e *stubExecutor) Execute(_ context.Context, query Query) (any, error) {
	e.query = query
	return e.result, e.err
}

func TestRosterAvailabilityComesFromExecutor(t *testing.T) {
	t.Parallel()
	executor := &stubExecutor{available: []string{"roster"}, result: []any{}}
	service, err := NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	description, err := service.Describe(context.Background(), []string{"roster"})
	if err != nil {
		t.Fatal(err)
	}
	if len(description.Topics) != 1 || !description.Topics[0].Available || len(description.Topics[0].Children) != 2 {
		t.Fatalf("description=%#v", description)
	}
	program := "function main(team) return team.array() end"
	if _, err := service.Query(context.Background(), []string{"roster"}, program); err != nil {
		t.Fatal(err)
	}
	if len(executor.query.Modules) != 1 || executor.query.Modules[0] != "roster" {
		t.Fatalf("query=%#v", executor.query)
	}
}

func TestDescribeTopics(t *testing.T) {
	t.Parallel()
	service, err := NewService(&stubExecutor{})
	if err != nil {
		t.Fatal(err)
	}

	description, err := service.Describe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(description.Topics) != 6 || description.Topics[0].Name != "runtime" || description.Topics[1].Name != "roster" || len(description.Topics[1].Children) != 0 {
		t.Fatalf("unexpected description: %#v", description)
	}

	parent, err := service.Describe(context.Background(), []string{"roster"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Topics) != 1 || len(parent.Topics[0].Children) != 2 || parent.Topics[0].Children[0].Name != "roster.teams" || parent.Topics[0].Children[1].Name != "roster.players" {
		t.Fatalf("unexpected parent topic: %#v", parent.Topics)
	}

	batch, err := service.Describe(context.Background(), []string{"roster.players", "unknown", "roster.teams", "roster.players", " "})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Topics) != 4 {
		t.Fatalf("unexpected batch topics: %#v", batch.Topics)
	}
	players := batch.Topics[0]
	if players.Name != "roster.players" || !players.Found || players.Available || players.Call == "" || len(players.Parameters) != 3 || players.Parameters[0].Name != "team_id" || !players.Parameters[0].Required || len(players.Returns) == 0 || players.Example == "" {
		t.Fatalf("unexpected player topic: %#v", players)
	}
	if batch.Topics[1].Found || batch.Topics[1].Error != "unknown team query topic" || batch.Topics[2].Name != "roster.teams" || batch.Topics[3].Found || batch.Topics[3].Error != "team query topic is required" {
		t.Fatalf("unexpected partial topic results: %#v", batch.Topics)
	}

	runtime, err := service.Describe(context.Background(), []string{"runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Topics) != 1 || runtime.Topics[0].Runtime == nil || runtime.Topics[0].Runtime.Language != "lua" {
		t.Fatalf("unexpected runtime topic: %#v", runtime.Topics)
	}

	game, err := service.Describe(context.Background(), []string{"game"})
	if err != nil {
		t.Fatal(err)
	}
	if len(game.Topics) != 1 || !game.Topics[0].Found || game.Topics[0].Available {
		t.Fatalf("unexpected unavailable topic: %#v", game.Topics)
	}
}

func TestQueryValidationAndDelegation(t *testing.T) {
	t.Parallel()
	executor := &stubExecutor{result: map[string]any{"count": 3}}
	service, _ := NewService(executor)

	program := "function main(team) return {count = 3} end"
	result, err := service.Query(context.Background(), nil, program)
	if err != nil {
		t.Fatal(err)
	}
	if executor.query.Program != program || result.(map[string]any)["count"] != 3 {
		t.Fatalf("unexpected delegation: query=%#v result=%#v", executor.query, result)
	}
	if _, err := service.Query(context.Background(), []string{"game"}, program); !errors.Is(err, ErrModuleUnavailable) {
		t.Fatalf("unavailable module error = %v", err)
	}
	if _, err := service.Query(context.Background(), []string{"unknown"}, program); !errors.Is(err, ErrUnknownModule) {
		t.Fatalf("unknown module error = %v", err)
	}
	if _, err := service.Query(context.Background(), nil, " \n "); err == nil {
		t.Fatal("empty query program was accepted")
	}
	if _, err := service.Query(context.Background(), nil, strings.Repeat("x", MaxProgramBytes+1)); err == nil {
		t.Fatal("oversized query program was accepted")
	}
}

func TestNewServiceRequiresExecutor(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil); err == nil {
		t.Fatal("nil executor was accepted")
	}
}
