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
	players  *player.QueryService
	game     *game.QueryService
	training *training.QueryService
}

type Option func(*LuaExecutor) error

func WithTeamPlayerServices(teams *team.QueryService, players *player.QueryService) Option {
	return func(executor *LuaExecutor) error {
		if teams == nil || players == nil {
			return errors.New("team and player query services are required")
		}
		executor.teams, executor.players = teams, players
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
	if e.teams != nil {
		result = append(result, "team")
	}
	if e.players != nil {
		result = append(result, "player")
	}
	if e.game != nil {
		result = append(result, "game")
	}
	return result
}

func (e *LuaExecutor) AvailableTopics() []string {
	var result []string
	if e.teams != nil {
		result = append(result, "team.list")
	}
	if e.players != nil {
		result = append(result, "player.list")
	}
	if e.game != nil {
		result = append(result, "game.list", "game.summaries", "game.records", "game.lineups", "game.performances")
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
		case "team":
			if e.teams == nil {
				return nil, errors.New("team module is unavailable")
			}
			backing.RawSetString("team", newTeamModule(ctx, state, e.teams, explicitArrays))
		case "player":
			if e.players == nil {
				return nil, errors.New("player module is unavailable")
			}
			backing.RawSetString("player", newPlayerModule(ctx, state, e.players, explicitArrays))
		case "game":
			if e.game == nil {
				return nil, errors.New("game module is unavailable")
			}
			backing.RawSetString("game", newGameModule(ctx, state, e.game, explicitArrays))
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

func newRosterProxy(ctx context.Context, state *lua.LState, teams *team.QueryService, service *player.QueryService, explicitArrays map[*lua.LTable]struct{}) *lua.LUserData {
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
		filter := player.PlayerFilter{TeamName: string(lua.LVAsString(filterTable.RawGetString("team_name")))}
		if value := filterTable.RawGetString("jersey_number"); value != lua.LNil {
			jersey := uint8(lua.LVAsNumber(value))
			filter.JerseyNumber = &jersey
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
		players, err := service.List(ctx, filter)
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

func rosterPlayerToLua(state *lua.LState, current player.PlayerView, explicitArrays map[*lua.LTable]struct{}) *lua.LTable {
	entry := state.NewTable()
	entry.RawSetString("name", lua.LString(current.Name))
	entry.RawSetString("team_name", lua.LString(current.TeamName))
	entry.RawSetString("jersey_number", lua.LNumber(current.JerseyNumber))
	entry.RawSetString("positions", stringsToLua(state, positionFlagNames(current.Positions), explicitArrays))
	entry.RawSetString("active", lua.LBool(current.Active))
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
			if value := table.RawGetString("team_name"); value != lua.LNil {
				filter.ParticipantNames = []string{value.String()}
			}
			if value := table.RawGetString("date_from"); value != lua.LNil {
				parsed, err := parseQueryTime(value.String(), false)
				if err != nil {
					state.RaiseError("invalid game date_from: %v", err)
					return 0
				}
				filter.ScheduledFrom = &parsed
			}
			if value := table.RawGetString("date_to"); value != lua.LNil {
				parsed, err := parseQueryTime(value.String(), true)
				if err != nil {
					state.RaiseError("invalid game date_to: %v", err)
					return 0
				}
				filter.ScheduledTo = &parsed
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
			result.Append(matchViewToLua(state, value))
		}
		state.Push(result)
		return 1
	}))
	return readOnlyProxy(state, "game", backing)
}
func newLineupProxy(_ context.Context, state *lua.LState, _ *game.QueryService, _ map[*lua.LTable]struct{}) *lua.LUserData {
	return readOnlyProxy(state, "lineup", state.NewTable())
}
func matchViewToLua(state *lua.LState, value game.MatchView) *lua.LTable {
	result := state.NewTable()
	result.RawSetString("scheduled_at", lua.LString(value.ScheduledAt.Format(time.RFC3339)))
	result.RawSetString("home_team_name", lua.LString(value.HomeTeamName))
	result.RawSetString("away_team_name", lua.LString(value.AwayTeamName))
	result.RawSetString("location", lua.LString(value.Location))
	result.RawSetString("status", lua.LNumber(value.Status))
	return result
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
func battingResultName(value game.BattingResult) string {
	names := []string{"", "single", "double", "triple", "home_run", "walk", "intentional_walk", "hit_by_pitch", "strikeout", "ground_out", "fly_out", "line_out", "fielders_choice", "reached_on_error", "sacrifice_bunt", "sacrifice_fly", "interference", "other"}
	if int(value) >= len(names) {
		return ""
	}
	return names[value]
}
func playToLua(state *lua.LState, value game.Play, explicitArrays map[*lua.LTable]struct{}) *lua.LTable {
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
	result.RawSetString("starting_pitcher_id", lua.LString(value.StartingPitcherID()))
	result.RawSetString("batting_result", lua.LString(battingResultName(value.BattingResult())))
	result.RawSetString("result_description", lua.LString(value.ResultDescription()))
	for name, situation := range map[string]game.Situation{"before": value.Before(), "after": value.After()} {
		x := state.NewTable()
		x.RawSetString("outs", lua.LNumber(situation.Outs))
		x.RawSetString("home_score", lua.LNumber(situation.HomeScore))
		x.RawSetString("away_score", lua.LNumber(situation.AwayScore))
		result.RawSetString(name, x)
	}
	pitches := state.NewTable()
	explicitArrays[pitches] = struct{}{}
	for _, p := range value.Pitches() {
		x := state.NewTable()
		x.RawSetString("sequence", lua.LNumber(p.Sequence))
		x.RawSetString("pitcher_id", lua.LString(p.PitcherID))
		x.RawSetString("batter_id", lua.LString(p.BatterID))
		x.RawSetString("result", lua.LNumber(p.Result))
		pitches.Append(x)
	}
	result.RawSetString("pitches", pitches)
	runners := state.NewTable()
	explicitArrays[runners] = struct{}{}
	for _, v := range value.RunnerOutcomes() {
		x := state.NewTable()
		x.RawSetString("runner_id", lua.LString(v.RunnerID))
		x.RawSetString("result", lua.LNumber(v.Result))
		x.RawSetString("scored", lua.LBool(v.Scored))
		runners.Append(x)
	}
	result.RawSetString("runner_results", runners)
	fielding := state.NewTable()
	explicitArrays[fielding] = struct{}{}
	for _, v := range value.FieldingOutcomes() {
		x := state.NewTable()
		x.RawSetString("fielder_id", lua.LString(v.FielderID))
		x.RawSetString("result", lua.LNumber(v.Result))
		fielding.Append(x)
	}
	result.RawSetString("fielding_results", fielding)
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
