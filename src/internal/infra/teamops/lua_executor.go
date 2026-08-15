package teamops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	luadomain "github.com/naifenmizuha/basetion/src/internal/domain/teamops"
	lua "github.com/yuin/gopher-lua"
)

const (
	defaultTimeout       = 2 * time.Second
	defaultMaxDepth      = 32
	defaultMaxElements   = 10_000
	defaultMaxResultSize = 256 * 1024
)

type Limits struct {
	Timeout        time.Duration
	MaxSourceBytes int
	MaxDepth       int
	MaxElements    int
	MaxResultBytes int
	CallStackSize  int
	RegistrySize   int
	RegistryMax    int
}

func DefaultLimits() Limits {
	return Limits{
		Timeout:        defaultTimeout,
		MaxSourceBytes: luadomain.MaxProgramBytes,
		MaxDepth:       defaultMaxDepth,
		MaxElements:    defaultMaxElements,
		MaxResultBytes: defaultMaxResultSize,
		CallStackSize:  128,
		RegistrySize:   1024,
		RegistryMax:    8192,
	}
}

type LuaExecutor struct {
	limits Limits
	roster luadomain.RosterReader
}

type Option func(*LuaExecutor) error

func WithRosterReader(reader luadomain.RosterReader) Option {
	return func(executor *LuaExecutor) error {
		if reader == nil {
			return errors.New("roster reader is required")
		}
		executor.roster = reader
		return nil
	}
}

func NewLuaExecutor(limits Limits, options ...Option) (*LuaExecutor, error) {
	if limits.Timeout <= 0 {
		return nil, errors.New("lua timeout must be positive")
	}
	if limits.MaxSourceBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxElements <= 0 || limits.MaxResultBytes <= 0 {
		return nil, errors.New("lua source, depth, element, and result limits must be positive")
	}
	if limits.CallStackSize <= 0 || limits.RegistrySize < 128 || limits.RegistryMax < limits.RegistrySize {
		return nil, errors.New("invalid lua stack or registry limits")
	}
	executor := &LuaExecutor{limits: limits}
	for _, option := range options {
		if err := option(executor); err != nil {
			return nil, err
		}
	}
	return executor, nil
}

func (e *LuaExecutor) AvailableModules() []string {
	if e.roster == nil {
		return nil
	}
	return []string{"roster"}
}

type nullValue struct{}

type converter struct {
	limits         Limits
	active         map[*lua.LTable]struct{}
	explicitArrays map[*lua.LTable]struct{}
	elements       int
}

func (e *LuaExecutor) Execute(ctx context.Context, query luadomain.Query) (any, error) {
	program := query.Program
	if len(program) > e.limits.MaxSourceBytes {
		return nil, fmt.Errorf("lua source exceeds %d bytes", e.limits.MaxSourceBytes)
	}
	runCtx, cancel := context.WithTimeout(ctx, e.limits.Timeout)
	defer cancel()

	state := lua.NewState(lua.Options{
		CallStackSize:       e.limits.CallStackSize,
		RegistrySize:        e.limits.RegistrySize,
		RegistryMaxSize:     e.limits.RegistryMax,
		RegistryGrowStep:    256,
		SkipOpenLibs:        true,
		MinimizeStackMemory: true,
	})
	defer state.Close()
	state.SetContext(runCtx)

	if err := openSafeLibraries(state); err != nil {
		return nil, fmt.Errorf("initialize lua libraries: %w", err)
	}
	explicitArrays := make(map[*lua.LTable]struct{})
	teamops, err := e.newTeamOpsProxy(runCtx, state, explicitArrays, query.Modules)
	if err != nil {
		return nil, err
	}

	if err := state.DoString(program); err != nil {
		return nil, normalizeExecutionError(runCtx, err)
	}
	mainValue := state.GetGlobal("main")
	if mainValue == lua.LNil || mainValue.Type() != lua.LTFunction {
		return nil, errors.New("lua program must define main(teamops)")
	}
	if err := state.CallByParam(lua.P{Fn: mainValue, NRet: 1, Protect: true}, teamops); err != nil {
		return nil, normalizeExecutionError(runCtx, err)
	}
	value := state.Get(-1)
	state.Pop(1)

	converted, err := (&converter{
		limits:         e.limits,
		active:         make(map[*lua.LTable]struct{}),
		explicitArrays: explicitArrays,
	}).convert(value, 0)
	if err != nil {
		return nil, fmt.Errorf("convert lua result: %w", err)
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		return nil, fmt.Errorf("encode lua result: %w", err)
	}
	if len(encoded) > e.limits.MaxResultBytes {
		return nil, fmt.Errorf("lua result exceeds %d bytes", e.limits.MaxResultBytes)
	}
	return converted, nil
}

func normalizeExecutionError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("lua execution canceled: %w", ctxErr)
	}
	return fmt.Errorf("lua execution failed: %w", err)
}

func openSafeLibraries(state *lua.LState) error {
	libraries := []struct {
		name string
		open lua.LGFunction
	}{
		{name: lua.BaseLibName, open: lua.OpenBase},
		{name: lua.TabLibName, open: lua.OpenTable},
		{name: lua.StringLibName, open: lua.OpenString},
		{name: lua.MathLibName, open: lua.OpenMath},
	}
	for _, library := range libraries {
		if err := state.CallByParam(lua.P{Fn: state.NewFunction(library.open), NRet: 0, Protect: true}, lua.LString(library.name)); err != nil {
			return err
		}
	}

	for _, name := range []string{
		"collectgarbage", "dofile", "getfenv", "getmetatable", "load", "loadfile", "loadstring",
		"module", "newproxy", "print", "rawequal", "rawget", "rawset", "require", "setfenv",
		"setmetatable", "xpcall",
	} {
		state.SetGlobal(name, lua.LNil)
	}
	removeTableFields(state, "math", "random", "randomseed")
	removeTableFields(state, "string", "dump")
	return nil
}

func removeTableFields(state *lua.LState, global string, fields ...string) {
	table, ok := state.GetGlobal(global).(*lua.LTable)
	if !ok {
		return
	}
	for _, field := range fields {
		table.RawSetString(field, lua.LNil)
	}
}

func (e *LuaExecutor) newTeamOpsProxy(ctx context.Context, state *lua.LState, explicitArrays map[*lua.LTable]struct{}, modules []string) (*lua.LUserData, error) {
	backing := state.NewTable()
	backing.RawSetString("array", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() != 0 {
			state.RaiseError("teamops.array does not accept arguments")
			return 0
		}
		table := state.NewTable()
		explicitArrays[table] = struct{}{}
		state.Push(table)
		return 1
	}))
	null := state.NewUserData()
	null.Value = nullValue{}
	backing.RawSetString("null", null)
	for _, module := range modules {
		switch module {
		case "roster":
			if e.roster == nil {
				return nil, errors.New("roster module is unavailable")
			}
			backing.RawSetString("roster", newRosterProxy(ctx, state, e.roster, explicitArrays))
		default:
			return nil, fmt.Errorf("unsupported lua module %q", module)
		}
	}

	proxy := state.NewUserData()
	proxy.Value = struct{ name string }{name: "teamops"}
	meta := state.NewTable()
	meta.RawSetString("__index", backing)
	meta.RawSetString("__newindex", state.NewFunction(func(state *lua.LState) int {
		state.RaiseError("teamops is read-only")
		return 0
	}))
	meta.RawSetString("__metatable", lua.LFalse)
	state.SetMetatable(proxy, meta)
	return proxy, nil
}

var positionNames = map[string]player.PositionFlags{
	"pitcher":     player.PositionPitcher,
	"catcher":     player.PositionCatcher,
	"first_base":  player.PositionFirstBase,
	"second_base": player.PositionSecondBase,
	"shortstop":   player.PositionShortstop,
	"third_base":  player.PositionThirdBase,
	"outfielder":  player.PositionOutfielder,
}

func newRosterProxy(ctx context.Context, state *lua.LState, reader luadomain.RosterReader, explicitArrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("teams", state.NewFunction(func(state *lua.LState) int {
		activeOnly := false
		if state.GetTop() > 1 {
			state.RaiseError("roster.teams accepts at most one filter table")
		}
		if state.GetTop() == 1 {
			filter := state.CheckTable(1)
			if value := filter.RawGetString("active"); value != lua.LNil {
				activeOnly = bool(lua.LVAsBool(value))
			}
		}
		teams, err := reader.ListTeams(ctx, activeOnly)
		if err != nil {
			state.RaiseError("list roster teams: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, current := range teams {
			entry := state.NewTable()
			entry.RawSetString("id", lua.LString(current.ID))
			entry.RawSetString("name", lua.LString(current.Name))
			entry.RawSetString("active", lua.LBool(current.Active))
			result.Append(entry)
		}
		state.Push(result)
		return 1
	}))
	backing.RawSetString("players", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() != 1 {
			state.RaiseError("roster.players requires one filter table")
			return 0
		}
		filterTable := state.CheckTable(1)
		teamID := string(lua.LVAsString(filterTable.RawGetString("team_id")))
		if teamID == "" {
			state.RaiseError("roster.players requires team_id")
			return 0
		}
		filter := luadomain.RosterPlayerFilter{TeamID: teamID}
		if value := filterTable.RawGetString("on_date"); value != lua.LNil {
			parsed, err := roster.ParseDate(string(lua.LVAsString(value)))
			if err != nil {
				state.RaiseError("invalid roster on_date: %v", err)
				return 0
			}
			filter.OnDate = &parsed
		}
		if value := filterTable.RawGetString("position_any"); value != lua.LNil {
			positions, ok := value.(*lua.LTable)
			if !ok {
				state.RaiseError("position_any must be an array")
				return 0
			}
			positions.ForEach(func(_, value lua.LValue) {
				flag, exists := positionNames[string(lua.LVAsString(value))]
				if !exists {
					state.RaiseError("unknown roster position %q", value.String())
					return
				}
				filter.PositionAny |= flag
			})
		}
		players, err := reader.ListPlayers(ctx, filter)
		if err != nil {
			state.RaiseError("list roster players: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, current := range players {
			result.Append(rosterPlayerToLua(state, current, explicitArrays))
		}
		state.Push(result)
		return 1
	}))
	proxy := state.NewUserData()
	proxy.Value = struct{ name string }{name: "roster"}
	meta := state.NewTable()
	meta.RawSetString("__index", backing)
	meta.RawSetString("__newindex", state.NewFunction(func(state *lua.LState) int { state.RaiseError("roster is read-only"); return 0 }))
	meta.RawSetString("__metatable", lua.LFalse)
	state.SetMetatable(proxy, meta)
	return proxy
}

func rosterPlayerToLua(state *lua.LState, current luadomain.RosterPlayerView, explicitArrays map[*lua.LTable]struct{}) *lua.LTable {
	entry := state.NewTable()
	entry.RawSetString("membership_id", lua.LString(current.MembershipID))
	entry.RawSetString("team_id", lua.LString(current.TeamID))
	entry.RawSetString("player_id", lua.LString(current.PlayerID))
	entry.RawSetString("name", lua.LString(current.Name))
	entry.RawSetString("jersey_number", lua.LNumber(current.JerseyNumber))
	entry.RawSetString("batting_hands", stringsToLua(state, current.BattingHands, explicitArrays))
	entry.RawSetString("throwing_hands", stringsToLua(state, current.ThrowingHands, explicitArrays))
	entry.RawSetString("positions", stringsToLua(state, current.Positions, explicitArrays))
	entry.RawSetString("joined_at", lua.LString(current.JoinedAt))
	if current.LeftAt == nil {
		null := state.NewUserData()
		null.Value = nullValue{}
		entry.RawSetString("left_at", null)
	} else {
		entry.RawSetString("left_at", lua.LString(*current.LeftAt))
	}
	entry.RawSetString("active", lua.LBool(current.Active))
	return entry
}

func stringsToLua(state *lua.LState, values []string, explicitArrays map[*lua.LTable]struct{}) *lua.LTable {
	result := state.NewTable()
	explicitArrays[result] = struct{}{}
	for _, value := range values {
		result.Append(lua.LString(value))
	}
	return result
}

func (c *converter) convert(value lua.LValue, depth int) (any, error) {
	if depth > c.limits.MaxDepth {
		return nil, fmt.Errorf("lua result exceeds maximum depth %d", c.limits.MaxDepth)
	}
	switch value := value.(type) {
	case *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(value), nil
	case lua.LNumber:
		number := float64(value)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, errors.New("lua result contains a non-finite number")
		}
		return number, nil
	case lua.LString:
		text := string(value)
		if !utf8.ValidString(text) {
			return nil, errors.New("lua result contains invalid UTF-8")
		}
		return text, nil
	case *lua.LTable:
		return c.convertTable(value, depth)
	case *lua.LUserData:
		if _, ok := value.Value.(nullValue); ok {
			return nil, nil
		}
		return nil, errors.New("lua result contains unsupported userdata")
	default:
		return nil, fmt.Errorf("lua result contains unsupported %s", value.Type().String())
	}
}

func (c *converter) convertTable(table *lua.LTable, depth int) (result any, err error) {
	if _, ok := c.active[table]; ok {
		return nil, errors.New("lua result contains a table cycle")
	}
	c.active[table] = struct{}{}
	defer delete(c.active, table)

	type tableKind uint8
	const (
		kindEmpty tableKind = iota
		kindArray
		kindObject
	)
	kind := kindEmpty
	count, maxIndex := 0, 0
	var iterationErr error
	table.ForEach(func(key, _ lua.LValue) {
		if iterationErr != nil {
			return
		}
		count++
		c.elements++
		if c.elements > c.limits.MaxElements {
			iterationErr = fmt.Errorf("lua result exceeds %d table elements", c.limits.MaxElements)
			return
		}
		switch key := key.(type) {
		case lua.LNumber:
			index := int(key)
			if float64(key) != float64(index) || index < 1 {
				iterationErr = errors.New("lua array keys must be positive integers")
				return
			}
			if kind == kindObject {
				iterationErr = errors.New("lua result table mixes array and object keys")
				return
			}
			kind = kindArray
			if index > maxIndex {
				maxIndex = index
			}
		case lua.LString:
			if kind == kindArray {
				iterationErr = errors.New("lua result table mixes array and object keys")
				return
			}
			kind = kindObject
		default:
			iterationErr = errors.New("lua object keys must be strings")
		}
	})
	if iterationErr != nil {
		return nil, iterationErr
	}

	if count == 0 {
		if _, ok := c.explicitArrays[table]; ok {
			return []any{}, nil
		}
		return map[string]any{}, nil
	}
	if kind == kindArray {
		if maxIndex != count {
			return nil, errors.New("lua result contains a sparse array")
		}
		array := make([]any, count)
		for index := 1; index <= count; index++ {
			converted, err := c.convert(table.RawGetInt(index), depth+1)
			if err != nil {
				return nil, err
			}
			array[index-1] = converted
		}
		return array, nil
	}

	object := make(map[string]any, count)
	table.ForEach(func(key, value lua.LValue) {
		if iterationErr != nil {
			return
		}
		converted, convertErr := c.convert(value, depth+1)
		if convertErr != nil {
			iterationErr = convertErr
			return
		}
		object[string(key.(lua.LString))] = converted
	})
	if iterationErr != nil {
		return nil, iterationErr
	}
	return object, nil
}
