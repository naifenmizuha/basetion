package teamquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const MaxProgramBytes = 32 * 1024

var (
	ErrUnknownModule     = errors.New("unknown team query module")
	ErrModuleUnavailable = errors.New("team query module is not available")
)

type Query struct {
	Modules []string
	Program string
}

// Executor runs a read-only team query program in an isolated runtime.
type Executor interface {
	AvailableModules() []string
	AvailableTopics() []string
	Execute(context.Context, Query) (any, error)
}

type RuntimeDescription struct {
	Language     string   `json:"language"`
	Version      string   `json:"version"`
	Entrypoint   string   `json:"entrypoint"`
	ReadOnly     bool     `json:"read_only"`
	Libraries    []string `json:"libraries"`
	SourceLimit  int      `json:"source_limit_bytes"`
	ResultLimit  int      `json:"result_limit_bytes"`
	ExecutionTip string   `json:"execution_tip"`
}

type TopicFieldDescription struct {
	Name        string                  `json:"name"`
	Type        string                  `json:"type"`
	Required    bool                    `json:"required"`
	Description string                  `json:"description"`
	Values      []string                `json:"values,omitempty"`
	Fields      []TopicFieldDescription `json:"fields,omitempty"`
	Item        *TopicFieldDescription  `json:"item,omitempty"`
}

type TopicSummary struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Summary   string `json:"summary"`
}

type TopicDescription struct {
	Name            string                  `json:"name"`
	Kind            string                  `json:"kind"`
	Found           bool                    `json:"found"`
	Available       bool                    `json:"available"`
	Summary         string                  `json:"summary,omitempty"`
	Error           string                  `json:"error,omitempty"`
	Children        []TopicSummary          `json:"children,omitempty"`
	Call            string                  `json:"call,omitempty"`
	RequiredModules []string                `json:"required_modules,omitempty"`
	Parameters      []TopicFieldDescription `json:"parameters,omitempty"`
	ResultType      string                  `json:"result_type,omitempty"`
	Returns         []TopicFieldDescription `json:"returns,omitempty"`
	Example         string                  `json:"example,omitempty"`
	Runtime         *RuntimeDescription     `json:"runtime,omitempty"`
}

type Description struct {
	Topics []TopicDescription `json:"topics"`
}

type module struct {
	name      string
	available bool
}

type topic struct {
	name            string
	kind            string
	summary         string
	requiresModules []string
	children        []string
	call            string
	parameters      []TopicFieldDescription
	resultType      string
	returns         []TopicFieldDescription
	example         string
}

// Service owns the stable model-facing team query contract.
type Service struct {
	executor        Executor
	modules         []module
	topics          map[string]topic
	availableTopics map[string]struct{}
	rootTopics      []string
}

func NewService(executor Executor) (*Service, error) {
	if executor == nil {
		return nil, errors.New("team query executor is required")
	}
	availableModules := make(map[string]struct{})
	for _, name := range executor.AvailableModules() {
		availableModules[name] = struct{}{}
	}
	modules := []module{{name: "team"}, {name: "player"}, {name: "game"}}
	for index := range modules {
		_, modules[index].available = availableModules[modules[index].name]
	}
	availableTopics := make(map[string]struct{})
	for _, name := range executor.AvailableTopics() {
		availableTopics[name] = struct{}{}
	}
	return &Service{
		executor:        executor,
		modules:         modules,
		topics:          defaultTopics(),
		availableTopics: availableTopics,
		rootTopics:      []string{"runtime", "team", "player", "game"},
	}, nil
}

func (s *Service) Describe(ctx context.Context, requested []string) (Description, error) {
	if err := ctx.Err(); err != nil {
		return Description{}, err
	}
	if len(requested) == 0 {
		result := make([]TopicDescription, 0, len(s.rootTopics))
		for _, name := range s.rootTopics {
			result = append(result, s.describeRootTopic(name))
		}
		return Description{Topics: result}, nil
	}
	result := make([]TopicDescription, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, s.describeTopic(name))
	}
	return Description{Topics: result}, nil
}

func (s *Service) Query(ctx context.Context, modules []string, program string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(program) > MaxProgramBytes {
		return nil, fmt.Errorf("team query program exceeds %d bytes", MaxProgramBytes)
	}
	program = strings.TrimSpace(program)
	if program == "" {
		return nil, errors.New("team query program is required")
	}
	selected, err := s.selectModules(modules)
	if err != nil {
		return nil, err
	}
	result, err := s.executor.Execute(ctx, Query{Modules: selected, Program: program})
	if err != nil {
		return nil, fmt.Errorf("execute team query: %w", err)
	}
	return result, nil
}

func (s *Service) selectModules(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	known := make(map[string]module, len(s.modules))
	for _, value := range s.modules {
		known[value.name] = value
	}
	selected := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		value, exists := known[name]
		if !exists {
			return nil, fmt.Errorf("%w: %q", ErrUnknownModule, name)
		}
		if !value.available {
			return nil, fmt.Errorf("%w: %s", ErrModuleUnavailable, name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, name)
	}
	return selected, nil
}

func (s *Service) describeRootTopic(name string) TopicDescription {
	definition := s.topics[name]
	return TopicDescription{Name: definition.name, Kind: definition.kind, Found: true, Available: s.topicAvailable(definition), Summary: definition.summary}
}

func (s *Service) describeTopic(name string) TopicDescription {
	if name == "" {
		return TopicDescription{Name: name, Found: false, Error: "team query topic is required"}
	}
	definition, exists := s.topics[name]
	if !exists {
		return TopicDescription{Name: name, Found: false, Error: "unknown team query topic"}
	}
	description := TopicDescription{Name: definition.name, Kind: definition.kind, Found: true, Available: s.topicAvailable(definition), Summary: definition.summary}
	if definition.kind == "runtime" {
		runtime := defaultRuntimeDescription()
		description.Runtime = &runtime
		return description
	}
	if len(definition.children) > 0 {
		description.Children = make([]TopicSummary, 0, len(definition.children))
		for _, childName := range definition.children {
			child := s.topics[childName]
			description.Children = append(description.Children, TopicSummary{Name: child.name, Available: s.topicAvailable(child), Summary: child.summary})
		}
		return description
	}
	description.Call = definition.call
	description.RequiredModules = append([]string(nil), definition.requiresModules...)
	description.Parameters = append([]TopicFieldDescription(nil), definition.parameters...)
	description.ResultType = definition.resultType
	description.Returns = append([]TopicFieldDescription(nil), definition.returns...)
	description.Example = definition.example
	return description
}

func (s *Service) topicAvailable(definition topic) bool {
	if definition.kind == "runtime" {
		return true
	}
	if len(definition.children) > 0 {
		for _, child := range definition.children {
			if s.topicAvailable(s.topics[child]) {
				return true
			}
		}
		return false
	}
	for _, required := range definition.requiresModules {
		found := false
		for _, candidate := range s.modules {
			if candidate.name == required {
				found = candidate.available
				break
			}
		}
		if !found {
			return false
		}
	}
	_, available := s.availableTopics[definition.name]
	return available
}

func defaultRuntimeDescription() RuntimeDescription {
	return RuntimeDescription{Language: "lua", Version: "5.1", Entrypoint: "main(team)", ReadOnly: true, Libraries: []string{"safe-base", "table", "string", "math"}, SourceLimit: MaxProgramBytes, ResultLimit: 256 * 1024, ExecutionTip: "先 describe 所需 topic，再提交只读 Lua 查询。"}
}

func field(name, typ string, required bool, description string, values ...string) TopicFieldDescription {
	return TopicFieldDescription{Name: name, Type: typ, Required: required, Description: description, Values: values}
}
func objectField(name string, required bool, description string, fields ...TopicFieldDescription) TopicFieldDescription {
	return TopicFieldDescription{Name: name, Type: "object", Required: required, Description: description, Fields: fields}
}
func arrayField(name string, required bool, description string, item TopicFieldDescription) TopicFieldDescription {
	return TopicFieldDescription{Name: name, Type: "array", Required: required, Description: description, Item: &item}
}

func defaultTopics() map[string]topic {
	positions := []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}
	teamFields := []TopicFieldDescription{field("name", "string", true, "球队名称。"), field("active", "boolean", true, "球队当前是否活跃。")}
	playerFields := []TopicFieldDescription{field("name", "string", true, "球员姓名。"), field("team_name", "string", true, "当前所属球队名称。"), field("jersey_number", "number", true, "当前球衣号码。"), field("positions", "string[]", true, "守备位置。"), field("active", "boolean", true, "球员当前是否启用。")}
	matchFields := []TopicFieldDescription{field("scheduled_at", "string", true, "RFC3339 比赛时间。"), field("home_team_name", "string", true, "主队名称。"), field("away_team_name", "string", true, "客队名称。"), field("location", "string", true, "比赛地点。"), field("status", "string", true, "比赛状态。", "scheduled", "in_progress", "final", "cancelled")}
	identityFields := []TopicFieldDescription{field("name", "string", true, "球员姓名。"), field("team_name", "string", true, "球员所属球队名称。"), field("jersey_number", "number", true, "球衣号码。")}
	matchFilter := []TopicFieldDescription{arrayField("participant_names", false, "零到两个精确参与球队名称；省略或空数组时不按球队筛选。", field("item", "string", true, "精确球队名称。")), field("date_from", "string", false, "YYYY-MM-DD 或 RFC3339 下界。"), field("date_to", "string", false, "YYYY-MM-DD 或 RFC3339 上界。"), field("limit", "integer", false, "最多返回的比赛数量；0 或省略表示不限制。")}
	matchSummaryFields := append(append([]TopicFieldDescription(nil), matchFields...), field("home_score", "number|null", true, "主队比分；没有有效 Play 时为 null。"), field("away_score", "number|null", true, "客队比分；没有有效 Play 时为 null。"), field("result", "string", true, "赛果。", "pending", "home_win", "away_win", "draw", "cancelled"))
	situationFields := []TopicFieldDescription{field("outs", "number", true, "Play 结束后的出局数。"), field("home_score", "number", true, "Play 结束后的主队比分。"), field("away_score", "number", true, "Play 结束后的客队比分。")}
	eventFields := []TopicFieldDescription{field("sequence", "number", true, "比赛内 Play 顺序。"), field("inning", "number", true, "局数。"), field("half", "string", true, "上下半局。", "top", "bottom"), objectField("batter", true, "打者身份。", identityFields...), objectField("starting_pitcher", true, "开局投手身份。", identityFields...), objectField("situation", true, "Play 结束局面。", situationFields...), field("batting_result", "number", true, "打击结果枚举。"), field("result_description", "string", true, "结果说明。")}
	lineupEntryFields := []TopicFieldDescription{objectField("player", true, "球员身份。", identityFields...), field("batting_order", "number", true, "棒次。"), field("position", "string", true, "单一守备位置。")}
	lineupFields := []TopicFieldDescription{field("team_name", "string", true, "阵容所属球队。"), field("kind", "string", true, "阵容类型。", "starter", "backup"), field("variant_number", "number", true, "阵容版本编号。"), field("variant_name", "string", true, "阵容名称。"), arrayField("entries", true, "按棒次排列的阵容条目。", TopicFieldDescription{Name: "item", Type: "object", Required: true, Description: "阵容条目。", Fields: lineupEntryFields})}
	offenseFields := []TopicFieldDescription{objectField("player", true, "球员身份。", identityFields...), field("pa", "number", true, "打席数。"), field("ab", "number", true, "打数。"), field("h", "number", true, "安打数。"), field("home_runs", "number", true, "本垒打数。"), field("rbi", "number", true, "打点。"), field("runs", "number", true, "得分。"), field("avg", "number|null", true, "打击率；打数为零时为 null。"), field("obp", "number|null", true, "上垒率；分母为零时为 null。"), field("slg", "number|null", true, "长打率；打数为零时为 null。"), field("ops", "number|null", true, "OPS；无法计算时为 null。")}
	pitchingFields := []TopicFieldDescription{objectField("player", true, "投手身份。", identityFields...), field("batters_faced", "number", true, "面对打者数。"), field("pitches", "number", true, "已记录投球数。"), field("called_strikes", "number", true, "好球判定数。"), field("swinging_strikes", "number", true, "挥空好球数。"), field("hits", "number", true, "被安打数。"), field("home_runs", "number", true, "被本垒打数。"), field("walks", "number", true, "保送数。"), field("hit_by_pitch", "number", true, "触身球数。"), field("strikeouts", "number", true, "三振数。"), field("runs", "number", true, "失分。"), field("earned_runs", "number", true, "明确标记的自责分。"), field("outs", "number", true, "按 Play 局面计算的出局数。")}
	fieldingFields := []TopicFieldDescription{objectField("player", true, "守备球员身份。", identityFields...), field("putouts", "number", true, "出局处理数。"), field("assists", "number", true, "助杀数。"), field("errors", "number", true, "失误数。")}
	limitFields := []TopicFieldDescription{field("fielding_opportunities_unavailable", "boolean", true, "未记录守备机会，不能据空行推断无守备机会。"), field("earned_runs_require_explicit_mark", "boolean", true, "自责分只统计明确标记的得分。"), field("unrecorded_pitch_facts_excluded", "boolean", true, "未记录的逐球事实不进入统计。")}
	return map[string]topic{
		"runtime":           {name: "runtime", kind: "runtime", summary: "受限 Lua 5.1 运行时、入口函数与资源限制。"},
		"team":              {name: "team", kind: "module", summary: "球队读取。", requiresModules: []string{"team"}, children: []string{"team.list"}},
		"team.list":         {name: "team.list", kind: "command", summary: "列出球队，可选只返回活跃球队。", requiresModules: []string{"team"}, call: "team.team.list({active=true})", parameters: []TopicFieldDescription{field("active", "boolean", false, "true 时仅返回活跃球队。")}, resultType: "array<team>", returns: teamFields, example: "function main(team) return team.team.list({active = true}) end"},
		"player":            {name: "player", kind: "module", summary: "当前球员读取。", requiresModules: []string{"player"}, children: []string{"player.list"}},
		"player.list":       {name: "player.list", kind: "command", summary: "按球队名称、背号和任一守备位置查询当前球员。", requiresModules: []string{"player"}, call: "team.player.list({team_name=..., jersey_number=..., position_any={...}})", parameters: []TopicFieldDescription{field("team_name", "string", false, "精确球队名称。"), field("jersey_number", "integer", false, "0 到 99 的背号。"), arrayField("position_any", false, "匹配任一守备位置。", field("item", "string", true, "守备位置。", positions...))}, resultType: "array<player>", returns: playerFields, example: "function main(team) return team.player.list({team_name = \"蜀汉队\"}) end"},
		"game":              {name: "game", kind: "module", summary: "比赛目录、摘要、记录、阵容和表现读取。", requiresModules: []string{"game"}, children: []string{"game.list", "game.summaries", "game.records", "game.lineups", "game.performances"}},
		"game.list":         {name: "game.list", kind: "command", summary: "读取比赛目录。", requiresModules: []string{"game"}, call: "team.game.list({participant_names={...}, date_from=..., date_to=..., limit=...})", parameters: matchFilter, resultType: "array<match>", returns: matchFields, example: "function main(team) return team.game.list({participant_names = {\"蜀汉队\"}}) end"},
		"game.summaries":    {name: "game.summaries", kind: "command", summary: "读取包含比分和赛果的比赛摘要。", requiresModules: []string{"game"}, call: "team.game.summaries({participant_names={...}, date_from=..., date_to=..., limit=...})", parameters: matchFilter, resultType: "array<match_summary>", returns: matchSummaryFields, example: "function main(team) return team.game.summaries({}) end"},
		"game.records":      {name: "game.records", kind: "command", summary: "读取姓名化的完整比赛记录。", requiresModules: []string{"game"}, call: "team.game.records({participant_names={...}, date_from=..., date_to=..., limit=...})", parameters: matchFilter, resultType: "array<match_record>", returns: []TopicFieldDescription{objectField("summary", true, "比赛摘要。", matchSummaryFields...), arrayField("events", true, "按顺序排列的比赛事件。", TopicFieldDescription{Name: "item", Type: "object", Required: true, Description: "比赛事件。", Fields: eventFields})}, example: "function main(team) return team.game.records({}) end"},
		"game.lineups":      {name: "game.lineups", kind: "command", summary: "读取比赛阵容。", requiresModules: []string{"game"}, call: "team.game.lineups({participant_names={...}, date_from=..., date_to=..., limit=...})", parameters: matchFilter, resultType: "array<match_lineups>", returns: []TopicFieldDescription{objectField("match", true, "比赛目录。", matchFields...), arrayField("lineups", true, "比赛阵容。", TopicFieldDescription{Name: "item", Type: "object", Required: true, Description: "阵容。", Fields: lineupFields})}, example: "function main(team) return team.game.lineups({}) end"},
		"game.performances": {name: "game.performances", kind: "command", summary: "读取逐场球员表现和统计限制；每个球员身份已包含球队名，无需另查球队或球员目录来分组。", requiresModules: []string{"game"}, call: "team.game.performances({participant_names={...}, date_from=..., date_to=..., limit=...})", parameters: matchFilter, resultType: "array<match_performance>", returns: []TopicFieldDescription{objectField("match", true, "比赛目录。", matchFields...), arrayField("offense", true, "有进攻记录的球员逐场表现。", TopicFieldDescription{Name: "item", Type: "object", Required: true, Description: "进攻表现行。", Fields: offenseFields}), arrayField("pitching", true, "有投球记录的球员逐场表现。", TopicFieldDescription{Name: "item", Type: "object", Required: true, Description: "投球表现行。", Fields: pitchingFields}), arrayField("fielding", true, "有守备结果的球员逐场表现。", TopicFieldDescription{Name: "item", Type: "object", Required: true, Description: "守备表现行。", Fields: fieldingFields}), objectField("limits", true, "统计限制。", limitFields...)}, example: "function main(team) return team.game.performances({}) end"},
	}
}
