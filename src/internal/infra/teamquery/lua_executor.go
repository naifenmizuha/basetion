package teamquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/naifenmizuha/basetion/src/internal/domain/game"
	"github.com/naifenmizuha/basetion/src/internal/domain/player"
	"github.com/naifenmizuha/basetion/src/internal/domain/roster"
	"github.com/naifenmizuha/basetion/src/internal/domain/team"
	querydomain "github.com/naifenmizuha/basetion/src/internal/domain/teamquery"
	"github.com/naifenmizuha/basetion/src/internal/domain/training"
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
		MaxSourceBytes: querydomain.MaxProgramBytes,
		MaxDepth:       defaultMaxDepth,
		MaxElements:    defaultMaxElements,
		MaxResultBytes: defaultMaxResultSize,
		CallStackSize:  128,
		RegistrySize:   1024,
		RegistryMax:    8192,
	}
}

type LuaExecutor struct {
	limits   Limits
	teams    *team.QueryService
	roster   *roster.QueryService
	game     *game.QueryService
	training *training.QueryService
}

type Option func(*LuaExecutor) error

func WithRosterServices(teams *team.QueryService, roster *roster.QueryService) Option {
	return func(executor *LuaExecutor) error {
		if teams == nil || roster == nil {
			return errors.New("team and roster query services are required")
		}
		executor.teams, executor.roster = teams, roster
		return nil
	}
}

func WithGameService(service *game.QueryService) Option {
	return func(executor *LuaExecutor) error {
		if service == nil {
			return errors.New("game query service is required")
		}
		executor.game = service
		return nil
	}
}

func WithTrainingService(service *training.QueryService) Option {
	return func(executor *LuaExecutor) error {
		if service == nil {
			return errors.New("training query service is required")
		}
		executor.training = service
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
	var result []string
	if e.teams != nil && e.roster != nil {
		result = append(result, "roster")
	}
	if e.game != nil {
		result = append(result, "game", "lineup")
	}
	if e.training != nil {
		result = append(result, "training")
	}
	return result
}

type nullValue struct{}

type converter struct {
	limits         Limits
	active         map[*lua.LTable]struct{}
	explicitArrays map[*lua.LTable]struct{}
	elements       int
}

func (e *LuaExecutor) Execute(ctx context.Context, query querydomain.Query) (any, error) {
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
	team, err := e.newTeamProxy(runCtx, state, explicitArrays, query.Modules)
	if err != nil {
		return nil, err
	}

	if err := state.DoString(program); err != nil {
		return nil, normalizeExecutionError(runCtx, err)
	}
	mainValue := state.GetGlobal("main")
	if mainValue == lua.LNil || mainValue.Type() != lua.LTFunction {
		return nil, errors.New("lua program must define main(team)")
	}
	if err := state.CallByParam(lua.P{Fn: mainValue, NRet: 1, Protect: true}, team); err != nil {
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

func (e *LuaExecutor) newTeamProxy(ctx context.Context, state *lua.LState, explicitArrays map[*lua.LTable]struct{}, modules []string) (*lua.LUserData, error) {
	backing := state.NewTable()
	backing.RawSetString("array", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() != 0 {
			state.RaiseError("team.array does not accept arguments")
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
			if e.teams == nil || e.roster == nil {
				return nil, errors.New("roster module is unavailable")
			}
			backing.RawSetString("roster", newRosterProxy(ctx, state, e.teams, e.roster, explicitArrays))
		case "game":
			if e.game == nil {
				return nil, errors.New("game module is unavailable")
			}
			backing.RawSetString("game", newGameProxy(ctx, state, e.game, explicitArrays))
		case "lineup":
			if e.game == nil {
				return nil, errors.New("lineup module is unavailable")
			}
			backing.RawSetString("lineup", newLineupProxy(ctx, state, e.game, explicitArrays))
		case "training":
			if e.training == nil {
				return nil, errors.New("training module is unavailable")
			}
			backing.RawSetString("training", newTrainingProxy(ctx, state, e.training, explicitArrays))
		default:
			return nil, fmt.Errorf("unsupported lua module %q", module)
		}
	}

	proxy := state.NewUserData()
	proxy.Value = struct{ name string }{name: "team"}
	meta := state.NewTable()
	meta.RawSetString("__index", backing)
	meta.RawSetString("__newindex", state.NewFunction(func(state *lua.LState) int {
		state.RaiseError("team is read-only")
		return 0
	}))
	meta.RawSetString("__metatable", lua.LFalse)
	state.SetMetatable(proxy, meta)
	return proxy, nil
}

func newTrainingProxy(ctx context.Context, state *lua.LState, service *training.QueryService, explicitArrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("records", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() != 1 {
			state.RaiseError("training.records requires one filter table")
			return 0
		}
		table := state.CheckTable(1)
		playerID := string(lua.LVAsString(table.RawGetString("player_id")))
		if playerID == "" {
			state.RaiseError("training.records requires player_id")
			return 0
		}
		filter := training.Filter{PlayerID: player.ID(playerID)}
		if value := table.RawGetString("from_date"); value != lua.LNil {
			parsed, err := training.ParseDate(string(lua.LVAsString(value)))
			if err != nil {
				state.RaiseError("invalid training from_date: %v", err)
				return 0
			}
			filter.From = &parsed
		}
		if value := table.RawGetString("to_date"); value != lua.LNil {
			parsed, err := training.ParseDate(string(lua.LVAsString(value)))
			if err != nil {
				state.RaiseError("invalid training to_date: %v", err)
				return 0
			}
			filter.To = &parsed
		}
		values, err := service.List(ctx, filter)
		if err != nil {
			state.RaiseError("list training records: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, value := range values {
			entry := state.NewTable()
			entry.RawSetString("id", lua.LString(value.ID()))
			entry.RawSetString("player_id", lua.LString(value.PlayerID()))
			entry.RawSetString("training_date", lua.LString(value.TrainingDate().String()))
			entry.RawSetString("content", lua.LString(value.Content()))
			entry.RawSetString("reflection", lua.LString(value.Reflection()))
			entry.RawSetString("created_at", lua.LString(value.CreatedAt().Format(time.RFC3339Nano)))
			entry.RawSetString("updated_at", lua.LString(value.UpdatedAt().Format(time.RFC3339Nano)))
			result.Append(entry)
		}
		state.Push(result)
		return 1
	}))
	proxy := state.NewUserData()
	proxy.Value = struct{ name string }{name: "training"}
	meta := state.NewTable()
	meta.RawSetString("__index", backing)
	meta.RawSetString("__newindex", state.NewFunction(func(state *lua.LState) int { state.RaiseError("training is read-only"); return 0 }))
	meta.RawSetString("__metatable", lua.LFalse)
	state.SetMetatable(proxy, meta)
	return proxy
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

func newRosterProxy(ctx context.Context, state *lua.LState, teams *team.QueryService, service *roster.QueryService, explicitArrays map[*lua.LTable]struct{}) *lua.LUserData {
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
		values, err := teams.List(ctx, activeOnly)
		if err != nil {
			state.RaiseError("list roster teams: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, current := range values {
			entry := state.NewTable()
			entry.RawSetString("id", lua.LString(current.ID()))
			entry.RawSetString("name", lua.LString(current.Name()))
			entry.RawSetString("active", lua.LBool(current.Active()))
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
		filter := roster.PlayerFilter{TeamID: team.ID(teamID)}
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
		players, err := service.ListPlayers(ctx, filter)
		if err != nil {
			state.RaiseError("list roster players: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		activeDate := roster.DateFromTime(time.Now())
		if filter.OnDate != nil {
			activeDate = *filter.OnDate
		}
		for _, current := range players {
			result.Append(rosterPlayerToLua(state, current, activeDate, explicitArrays))
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

func rosterPlayerToLua(state *lua.LState, current roster.Member, activeDate roster.Date, explicitArrays map[*lua.LTable]struct{}) *lua.LTable {
	entry := state.NewTable()
	entry.RawSetString("membership_id", lua.LString(current.Membership.ID()))
	entry.RawSetString("team_id", lua.LString(current.Membership.TeamID()))
	entry.RawSetString("player_id", lua.LString(current.Player.ID()))
	entry.RawSetString("name", lua.LString(current.Player.Name()))
	entry.RawSetString("jersey_number", lua.LNumber(current.Membership.JerseyNumber()))
	entry.RawSetString("batting_hands", stringsToLua(state, handNames(current.Player.Batting()), explicitArrays))
	entry.RawSetString("throwing_hands", stringsToLua(state, handNames(current.Player.Throwing()), explicitArrays))
	entry.RawSetString("positions", stringsToLua(state, positionFlagNames(current.Player.Positions()), explicitArrays))
	entry.RawSetString("joined_at", lua.LString(current.Membership.JoinedAt().String()))
	if current.Membership.LeftAt() == nil {
		null := state.NewUserData()
		null.Value = nullValue{}
		entry.RawSetString("left_at", null)
	} else {
		entry.RawSetString("left_at", lua.LString(current.Membership.LeftAt().String()))
	}
	entry.RawSetString("active", lua.LBool(current.ActiveOn(activeDate)))
	return entry
}

func newGameProxy(ctx context.Context, state *lua.LState, service *game.QueryService, explicitArrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("matches", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() > 1 {
			state.RaiseError("game.matches accepts at most one filter table")
			return 0
		}
		filter := game.MatchFilter{}
		if state.GetTop() == 1 {
			table := state.CheckTable(1)
			filter.TeamID = team.ID(lua.LVAsString(table.RawGetString("team_id")))
			if value := table.RawGetString("date_from"); value != lua.LNil {
				parsed, err := parseQueryTime(value.String(), false)
				if err != nil {
					state.RaiseError("invalid game date_from: %v", err)
					return 0
				}
				filter.From = &parsed
			}
			if value := table.RawGetString("date_to"); value != lua.LNil {
				parsed, err := parseQueryTime(value.String(), true)
				if err != nil {
					state.RaiseError("invalid game date_to: %v", err)
					return 0
				}
				filter.To = &parsed
			}
			if value := table.RawGetString("status"); value != lua.LNil {
				parsed, ok := parseMatchStatus(value.String())
				if !ok {
					state.RaiseError("invalid game status %q", value.String())
					return 0
				}
				filter.Status = &parsed
			}
		}
		values, err := service.ListMatches(ctx, filter)
		if err != nil {
			state.RaiseError("list matches: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, value := range values {
			result.Append(matchToLua(state, value))
		}
		state.Push(result)
		return 1
	}))
	backing.RawSetString("match", state.NewFunction(func(state *lua.LState) int {
		id := requiredID(state, "game.match", "match_id")
		if id == "" {
			return 0
		}
		value, err := service.GetMatch(ctx, game.MatchID(id))
		if err != nil {
			state.RaiseError("get match: %v", err)
			return 0
		}
		state.Push(matchToLua(state, value))
		return 1
	}))
	backing.RawSetString("plates", state.NewFunction(func(state *lua.LState) int {
		id := requiredID(state, "game.plates", "match_id")
		if id == "" {
			return 0
		}
		values, err := service.ListPlates(ctx, game.MatchID(id))
		if err != nil {
			state.RaiseError("list plates: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, value := range values {
			result.Append(plateToLua(state, value))
		}
		state.Push(result)
		return 1
	}))
	backing.RawSetString("score", state.NewFunction(func(state *lua.LState) int {
		id := requiredID(state, "game.score", "match_id")
		if id == "" {
			return 0
		}
		value, err := service.CurrentScore(ctx, game.MatchID(id))
		if err != nil {
			state.RaiseError("get game score: %v", err)
			return 0
		}
		result := state.NewTable()
		result.RawSetString("known", lua.LBool(value.Score != nil))
		result.RawSetString("final", lua.LBool(value.Final))
		if value.Score == nil {
			setNull(state, result, "home_score")
			setNull(state, result, "away_score")
		} else {
			result.RawSetString("home_score", lua.LNumber(value.Score.Home))
			result.RawSetString("away_score", lua.LNumber(value.Score.Away))
		}
		state.Push(result)
		return 1
	}))
	return readOnlyProxy(state, "game", backing)
}

func newLineupProxy(ctx context.Context, state *lua.LState, service *game.QueryService, explicitArrays map[*lua.LTable]struct{}) *lua.LUserData {
	backing := state.NewTable()
	backing.RawSetString("list", state.NewFunction(func(state *lua.LState) int {
		if state.GetTop() != 1 {
			state.RaiseError("lineup.list requires one filter table")
			return 0
		}
		table := state.CheckTable(1)
		id := lua.LVAsString(table.RawGetString("match_id"))
		if id == "" {
			state.RaiseError("lineup.list requires match_id")
			return 0
		}
		teamID := team.ID(lua.LVAsString(table.RawGetString("team_id")))
		values, err := service.ListLineups(ctx, game.MatchID(id), teamID)
		if err != nil {
			state.RaiseError("list lineups: %v", err)
			return 0
		}
		result := state.NewTable()
		explicitArrays[result] = struct{}{}
		for _, value := range values {
			result.Append(lineupToLua(state, value, explicitArrays))
		}
		state.Push(result)
		return 1
	}))
	return readOnlyProxy(state, "lineup", backing)
}

func readOnlyProxy(state *lua.LState, name string, backing *lua.LTable) *lua.LUserData {
	proxy := state.NewUserData()
	proxy.Value = struct{ name string }{name: name}
	meta := state.NewTable()
	meta.RawSetString("__index", backing)
	meta.RawSetString("__newindex", state.NewFunction(func(state *lua.LState) int { state.RaiseError("%s is read-only", name); return 0 }))
	meta.RawSetString("__metatable", lua.LFalse)
	state.SetMetatable(proxy, meta)
	return proxy
}
func requiredID(state *lua.LState, call, field string) string {
	if state.GetTop() != 1 {
		state.RaiseError("%s requires one filter table", call)
		return ""
	}
	value := lua.LVAsString(state.CheckTable(1).RawGetString(field))
	if value == "" {
		state.RaiseError("%s requires %s", call, field)
		return ""
	}
	return value
}
func parseQueryTime(value string, end bool) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}, err
	}
	if end {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return parsed, nil
}
func parseMatchStatus(value string) (game.MatchStatus, bool) {
	statuses := map[string]game.MatchStatus{"scheduled": game.MatchScheduled, "in_progress": game.MatchInProgress, "final": game.MatchFinal, "cancelled": game.MatchCancelled}
	status, ok := statuses[value]
	return status, ok
}
func matchStatusName(value game.MatchStatus) string {
	names := map[game.MatchStatus]string{game.MatchScheduled: "scheduled", game.MatchInProgress: "in_progress", game.MatchFinal: "final", game.MatchCancelled: "cancelled"}
	return names[value]
}
func matchToLua(state *lua.LState, value game.Match) *lua.LTable {
	result := state.NewTable()
	result.RawSetString("id", lua.LString(value.ID()))
	result.RawSetString("home_team_id", lua.LString(value.HomeTeamID()))
	result.RawSetString("away_team_id", lua.LString(value.AwayTeamID()))
	result.RawSetString("scheduled_at", lua.LString(value.ScheduledAt().Format(time.RFC3339)))
	result.RawSetString("location", lua.LString(value.Location()))
	result.RawSetString("status", lua.LString(matchStatusName(value.Status())))
	return result
}
func lineupToLua(state *lua.LState, value game.Lineup, explicitArrays map[*lua.LTable]struct{}) *lua.LTable {
	result := state.NewTable()
	result.RawSetString("match_id", lua.LString(value.MatchID()))
	result.RawSetString("team_id", lua.LString(value.TeamID()))
	kind := "starter"
	if value.Kind() == game.LineupBackup {
		kind = "backup"
	}
	result.RawSetString("kind", lua.LString(kind))
	result.RawSetString("variant_number", lua.LNumber(value.VariantNumber()))
	result.RawSetString("variant_name", lua.LString(value.VariantName()))
	entries := state.NewTable()
	explicitArrays[entries] = struct{}{}
	for _, value := range value.Entries() {
		entry := state.NewTable()
		entry.RawSetString("id", lua.LString(value.ID()))
		entry.RawSetString("player_id", lua.LString(value.PlayerID()))
		entry.RawSetString("batting_order", lua.LNumber(value.BattingOrder()))
		names := positionFlagNames(value.Position())
		if len(names) > 0 {
			entry.RawSetString("position", lua.LString(names[0]))
		}
		entries.Append(entry)
	}
	result.RawSetString("entries", entries)
	return result
}
func plateTypeName(value game.PlateType) string {
	names := []string{"", "out", "single", "double", "triple", "home_run", "walk", "intentional_walk", "strikeout", "hit_by_pitch", "error", "fielders_choice", "sacrifice", "interference", "other"}
	if int(value) >= len(names) {
		return ""
	}
	return names[value]
}
func plateToLua(state *lua.LState, value game.Plate) *lua.LTable {
	result := state.NewTable()
	result.RawSetString("id", lua.LString(value.ID()))
	result.RawSetString("match_id", lua.LString(value.MatchID()))
	result.RawSetString("sequence", lua.LNumber(value.Sequence()))
	result.RawSetString("inning", lua.LNumber(value.Inning()))
	half := "top"
	if value.Half() == game.Bottom {
		half = "bottom"
	}
	result.RawSetString("half", lua.LString(half))
	result.RawSetString("batting_order", lua.LNumber(value.BattingOrder()))
	result.RawSetString("batter_id", lua.LString(value.BatterID()))
	result.RawSetString("pitcher_id", lua.LString(value.PitcherID()))
	result.RawSetString("pitch_sequence", lua.LString(value.PitchSequence()))
	result.RawSetString("plate_type", lua.LString(plateTypeName(value.Type())))
	result.RawSetString("result_description", lua.LString(value.ResultDescription()))
	runners := value.Runners()
	fields := []string{"runner_on_first_id", "runner_on_second_id", "runner_on_third_id"}
	for i, field := range fields {
		if runners[i] == nil {
			setNull(state, result, field)
		} else {
			result.RawSetString(field, lua.LString(*runners[i]))
		}
	}
	if score := value.Score(); score == nil {
		setNull(state, result, "home_score")
		setNull(state, result, "away_score")
	} else {
		result.RawSetString("home_score", lua.LNumber(score.Home))
		result.RawSetString("away_score", lua.LNumber(score.Away))
	}
	return result
}
func setNull(state *lua.LState, table *lua.LTable, field string) {
	value := state.NewUserData()
	value.Value = nullValue{}
	table.RawSetString(field, value)
}

func handNames(flags player.HandFlags) []string {
	result := []string{}
	if flags.Has(player.HandLeft) {
		result = append(result, "left")
	}
	if flags.Has(player.HandRight) {
		result = append(result, "right")
	}
	return result
}
func positionFlagNames(flags player.PositionFlags) []string {
	ordered := []struct {
		name string
		flag player.PositionFlags
	}{{"pitcher", player.PositionPitcher}, {"catcher", player.PositionCatcher}, {"first_base", player.PositionFirstBase}, {"second_base", player.PositionSecondBase}, {"shortstop", player.PositionShortstop}, {"third_base", player.PositionThirdBase}, {"outfielder", player.PositionOutfielder}}
	result := []string{}
	for _, v := range ordered {
		if flags.HasAny(v.flag) {
			result = append(result, v.name)
		}
	}
	return result
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
