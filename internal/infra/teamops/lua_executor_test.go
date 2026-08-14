package teamops

import (
	"context"
	"strings"
	"testing"
	"time"
)

func testExecutor(t *testing.T) *LuaExecutor {
	t.Helper()
	limits := DefaultLimits()
	limits.Timeout = 250 * time.Millisecond
	executor, err := NewLuaExecutor(limits)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func TestLuaExecutorAggregatesAndConvertsResult(t *testing.T) {
	t.Parallel()
	executor := testExecutor(t)
	result, err := executor.Execute(context.Background(), `
function main(teamops)
    local values = {4, 1, 3, 2}
    table.sort(values)
    local sum = 0
    for _, value in ipairs(values) do
        sum = sum + value
    end
    return {
        sum = sum,
        maximum = math.max(unpack(values)),
        label = string.upper("hits"),
        values = values,
        empty_array = teamops.array(),
        null_value = teamops.null,
    }
end`)
	if err != nil {
		t.Fatal(err)
	}
	object := result.(map[string]any)
	if object["sum"] != float64(10) || object["maximum"] != float64(4) || object["label"] != "HITS" {
		t.Fatalf("unexpected aggregation: %#v", object)
	}
	values := object["values"].([]any)
	if len(values) != 4 || values[0] != float64(1) || values[3] != float64(4) {
		t.Fatalf("unexpected values: %#v", values)
	}
	if len(object["empty_array"].([]any)) != 0 || object["null_value"] != nil {
		t.Fatalf("unexpected special values: %#v", object)
	}
}

func TestLuaExecutorSandbox(t *testing.T) {
	t.Parallel()
	executor := testExecutor(t)
	result, err := executor.Execute(context.Background(), `
function main(teamops)
    return {
        os = type(os), io = type(io), require = type(require),
        load = type(load), loadfile = type(loadfile), debug = type(debug),
        coroutine = type(coroutine), random = type(math.random),
        setmetatable = type(setmetatable),
    }
end`)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range result.(map[string]any) {
		if value != "nil" {
			t.Fatalf("sandbox global %s has type %v", name, value)
		}
	}
	if _, err := executor.Execute(context.Background(), `function main(teamops) teamops.array = 1 end`); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("teamops mutation error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), `function main(teamops) table.insert(teamops, 1) end`); err == nil || !strings.Contains(err.Error(), "table expected") {
		t.Fatalf("teamops raw table mutation error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), `function main(teamops) return teamops.array({}) end`); err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("teamops.array argument error = %v", err)
	}
}

func TestLuaExecutorHonorsTimeout(t *testing.T) {
	t.Parallel()
	limits := DefaultLimits()
	limits.Timeout = 20 * time.Millisecond
	executor, _ := NewLuaExecutor(limits)
	started := time.Now()
	_, err := executor.Execute(context.Background(), `function main(teamops) while true do end end`)
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("timeout error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(started))
	}
}

func TestLuaExecutorRejectsInvalidResults(t *testing.T) {
	t.Parallel()
	executor := testExecutor(t)
	tests := []struct {
		name    string
		program string
		want    string
	}{
		{name: "missing main", program: `return {}`, want: "must define main"},
		{name: "cycle", program: `function main() local t = {}; t.self = t; return t end`, want: "cycle"},
		{name: "sparse", program: `function main() return {[1] = "a", [3] = "c"} end`, want: "sparse"},
		{name: "mixed", program: `function main() return {[1] = "a", name = "x"} end`, want: "mixes"},
		{name: "function", program: `function main() return {value = function() end} end`, want: "unsupported function"},
		{name: "nan", program: `function main() return 0 / 0 end`, want: "non-finite"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := executor.Execute(context.Background(), test.program)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLuaExecutorLimits(t *testing.T) {
	t.Parallel()
	limits := DefaultLimits()
	limits.Timeout = 250 * time.Millisecond
	limits.MaxElements = 2
	limits.MaxResultBytes = 12
	executor, _ := NewLuaExecutor(limits)
	if _, err := executor.Execute(context.Background(), `function main() return {1, 2, 3} end`); err == nil || !strings.Contains(err.Error(), "elements") {
		t.Fatalf("element limit error = %v", err)
	}
	if _, err := executor.Execute(context.Background(), `function main() return "a result larger than twelve bytes" end`); err == nil || !strings.Contains(err.Error(), "result exceeds") {
		t.Fatalf("result limit error = %v", err)
	}
}
