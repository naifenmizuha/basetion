package teamquery

import (
	"context"
	"strings"
	"testing"
)

type stubExecutor struct {
	query     Query
	available []string
	topics    []string
	result    any
	err       error
}

func (e *stubExecutor) AvailableModules() []string { return e.available }
func (e *stubExecutor) AvailableTopics() []string  { return e.topics }
func (e *stubExecutor) Execute(_ context.Context, query Query) (any, error) {
	e.query = query
	return e.result, e.err
}

func TestAvailabilityComesFromExecutor(t *testing.T) {
	t.Parallel()
	executor := &stubExecutor{available: []string{"team", "player", "game"}, topics: []string{"team.list", "player.list", "game.list", "game.plays"}, result: []any{}}
	service, err := NewService(executor)
	if err != nil {
		t.Fatal(err)
	}
	description, err := service.Describe(context.Background(), []string{"team", "game"})
	if err != nil {
		t.Fatal(err)
	}
	if len(description.Topics) != 2 || !description.Topics[0].Available || len(description.Topics[0].Children) != 1 || !description.Topics[1].Available || !description.Topics[1].Children[0].Available || !description.Topics[1].Children[1].Available {
		t.Fatalf("description=%#v", description)
	}
	program := "function main(data) return data.array() end"
	if _, err := service.Query(context.Background(), program); err != nil {
		t.Fatal(err)
	}
	if executor.query.Program != program {
		t.Fatalf("query=%#v", executor.query)
	}
}

func TestDescribeTopics(t *testing.T) {
	t.Parallel()
	service, err := NewService(&stubExecutor{available: []string{"player"}, topics: []string{"player.list"}})
	if err != nil {
		t.Fatal(err)
	}

	description, err := service.Describe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(description.Topics) != 4 || description.Topics[0].Name != "runtime" || description.Topics[1].Name != "team" || description.Topics[2].Name != "player" || !description.Topics[2].Available {
		t.Fatalf("unexpected description: %#v", description)
	}

	parent, err := service.Describe(context.Background(), []string{"player"})
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Topics) != 1 || len(parent.Topics[0].Children) != 1 || parent.Topics[0].Children[0].Name != "player.list" {
		t.Fatalf("unexpected parent topic: %#v", parent.Topics)
	}

	batch, err := service.Describe(context.Background(), []string{"player.list", "roster.players", "game.summaries", "player.list", " "})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Topics) != 4 {
		t.Fatalf("unexpected batch topics: %#v", batch.Topics)
	}
	players := batch.Topics[0]
	if players.Name != "player.list" || !players.Found || !players.Available || players.Call == "" || len(players.Parameters) != 3 || players.Parameters[0].Name != "team_name" || players.Parameters[0].Required || len(players.Returns) != 5 || players.Example == "" {
		t.Fatalf("unexpected player topic: %#v", players)
	}
	if batch.Topics[1].Found || batch.Topics[1].Error != "unknown team query topic" || batch.Topics[2].Found || batch.Topics[2].Error != "unknown team query topic" || batch.Topics[3].Found || batch.Topics[3].Error != "team query topic is required" {
		t.Fatalf("unexpected partial topic results: %#v", batch.Topics)
	}

	runtime, err := service.Describe(context.Background(), []string{"runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.Topics) != 1 || runtime.Topics[0].Runtime == nil || runtime.Topics[0].Runtime.Language != "lua" {
		t.Fatalf("unexpected runtime topic: %#v", runtime.Topics)
	}
}

func TestDescribePlaysIncludesOpaqueMatchInput(t *testing.T) {
	t.Parallel()
	service, err := NewService(&stubExecutor{available: []string{"game"}, topics: []string{"game.plays"}})
	if err != nil {
		t.Fatal(err)
	}
	description, err := service.Describe(context.Background(), []string{"game.plays"})
	if err != nil {
		t.Fatal(err)
	}
	if len(description.Topics[0].Parameters) != 1 || description.Topics[0].Parameters[0].Name != "matches" || description.Topics[0].Parameters[0].Item == nil || description.Topics[0].Parameters[0].Item.Type != "match" {
		t.Fatalf("match reference parameter is missing: %#v", description.Topics[0].Parameters)
	}
}

func TestQueryValidationAndDelegation(t *testing.T) {
	t.Parallel()
	executor := &stubExecutor{available: []string{"game"}, topics: []string{"game.list"}, result: map[string]any{"count": 3}}
	service, _ := NewService(executor)

	program := "function main(data) return {count = 3} end"
	result, err := service.Query(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	if executor.query.Program != program || result.(map[string]any)["count"] != 3 {
		t.Fatalf("unexpected delegation: query=%#v result=%#v", executor.query, result)
	}
	if _, err := service.Query(context.Background(), " \n "); err == nil {
		t.Fatal("empty query program was accepted")
	}
	if _, err := service.Query(context.Background(), strings.Repeat("x", MaxProgramBytes+1)); err == nil {
		t.Fatal("oversized query program was accepted")
	}
}

func TestNewServiceRequiresExecutor(t *testing.T) {
	t.Parallel()
	if _, err := NewService(nil); err == nil {
		t.Fatal("nil executor was accepted")
	}
}
