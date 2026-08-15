package teamops

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
	if len(description.Modules) != 1 || !description.Modules[0].Available {
		t.Fatalf("description=%#v", description)
	}
	program := "function main(teamops) return teamops.array() end"
	if _, err := service.Query(context.Background(), []string{"roster"}, program); err != nil {
		t.Fatal(err)
	}
	if len(executor.query.Modules) != 1 || executor.query.Modules[0] != "roster" {
		t.Fatalf("query=%#v", executor.query)
	}
}

func TestDescribeModules(t *testing.T) {
	t.Parallel()
	service, err := NewService(&stubExecutor{})
	if err != nil {
		t.Fatal(err)
	}

	description, err := service.Describe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if description.Runtime.Language != "lua" || !description.Runtime.ReadOnly || len(description.Modules) != 5 {
		t.Fatalf("unexpected description: %#v", description)
	}
	filtered, err := service.Describe(context.Background(), []string{"game", "game", "analysis"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Modules) != 2 || filtered.Modules[0].Name != "game" || filtered.Modules[1].Name != "analysis" {
		t.Fatalf("unexpected filtered modules: %#v", filtered.Modules)
	}
	if _, err := service.Describe(context.Background(), []string{"unknown"}); !errors.Is(err, ErrUnknownModule) {
		t.Fatalf("unknown module error = %v", err)
	}
}

func TestQueryValidationAndDelegation(t *testing.T) {
	t.Parallel()
	executor := &stubExecutor{result: map[string]any{"count": 3}}
	service, _ := NewService(executor)

	program := "function main(teamops) return {count = 3} end"
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
