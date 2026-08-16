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
	Execute(ctx context.Context, query Query) (any, error)
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
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description"`
	Values      []string `json:"values,omitempty"`
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
	executor   Executor
	modules    []module
	topics     map[string]topic
	rootTopics []string
}

func NewService(executor Executor) (*Service, error) {
	if executor == nil {
		return nil, errors.New("team query executor is required")
	}
	available := make(map[string]struct{})
	for _, name := range executor.AvailableModules() {
		available[name] = struct{}{}
	}
	modules := []module{
		{name: "roster"},
		{name: "game"},
		{name: "lineup"},
		{name: "training"},
		{name: "analysis"},
	}
	for index := range modules {
		_, modules[index].available = available[modules[index].name]
	}
	return &Service{
		executor:   executor,
		modules:    modules,
		topics:     defaultTopics(),
		rootTopics: []string{"runtime", "roster", "game", "lineup", "training", "analysis"},
	}, nil
}

func (s *Service) Describe(ctx context.Context, requested []string) (Description, error) {
	if err := ctx.Err(); err != nil {
		return Description{}, err
	}
	if len(requested) == 0 {
		topics := make([]TopicDescription, 0, len(s.rootTopics))
		for _, name := range s.rootTopics {
			topics = append(topics, s.describeRootTopic(name))
		}
		return Description{Topics: topics}, nil
	}

	topics := make([]TopicDescription, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		topics = append(topics, s.describeTopic(name))
	}
	return Description{Topics: topics}, nil
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
	selected, err := s.selectModules(modules, false)
	if err != nil {
		return nil, err
	}
	for _, module := range selected {
		if !module.available {
			return nil, fmt.Errorf("%w: %s", ErrModuleUnavailable, module.name)
		}
	}
	selectedNames := make([]string, len(selected))
	for index, module := range selected {
		selectedNames[index] = module.name
	}
	result, err := s.executor.Execute(ctx, Query{Modules: selectedNames, Program: program})
	if err != nil {
		return nil, fmt.Errorf("execute team query: %w", err)
	}
	return result, nil
}

func (s *Service) selectModules(requested []string, defaultAll bool) ([]module, error) {
	if len(requested) == 0 {
		if !defaultAll {
			return nil, nil
		}
		modules := make([]module, len(s.modules))
		copy(modules, s.modules)
		return modules, nil
	}

	byName := make(map[string]module, len(s.modules))
	for _, module := range s.modules {
		byName[module.name] = module
	}
	selected := make([]module, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		module, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownModule, name)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, module)
	}
	return selected, nil
}

func (s *Service) describeRootTopic(name string) TopicDescription {
	definition := s.topics[name]
	return TopicDescription{
		Name:      definition.name,
		Kind:      definition.kind,
		Found:     true,
		Available: s.topicAvailable(definition),
		Summary:   definition.summary,
	}
}

func (s *Service) describeTopic(name string) TopicDescription {
	if name == "" {
		return TopicDescription{Name: name, Found: false, Error: "team query topic is required"}
	}
	definition, exists := s.topics[name]
	if !exists {
		return TopicDescription{Name: name, Found: false, Error: "unknown team query topic"}
	}

	description := TopicDescription{
		Name:      definition.name,
		Kind:      definition.kind,
		Found:     true,
		Available: s.topicAvailable(definition),
		Summary:   definition.summary,
	}
	if definition.kind == "runtime" {
		runtime := defaultRuntimeDescription()
		description.Runtime = &runtime
		return description
	}
	if len(definition.children) > 0 {
		description.Children = make([]TopicSummary, 0, len(definition.children))
		for _, childName := range definition.children {
			child := s.topics[childName]
			description.Children = append(description.Children, TopicSummary{
				Name:      child.name,
				Available: s.topicAvailable(child),
				Summary:   child.summary,
			})
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
	for _, required := range definition.requiresModules {
		available := false
		for _, module := range s.modules {
			if module.name == required {
				available = module.available
				break
			}
		}
		if !available {
			return false
		}
	}
	return true
}

func defaultRuntimeDescription() RuntimeDescription {
	return RuntimeDescription{
		Language:     "lua",
		Version:      "5.1",
		Entrypoint:   "main(team)",
		ReadOnly:     true,
		Libraries:    []string{"safe-base", "table", "string", "math"},
		SourceLimit:  MaxProgramBytes,
		ResultLimit:  256 * 1024,
		ExecutionTip: "先 describe 所需 topic，再提交只读 Lua 查询；roster.players 必须指定 team_id。",
	}
}

func defaultTopics() map[string]topic {
	teamFields := []TopicFieldDescription{
		{Name: "id", Type: "string", Required: true, Description: "球队 ID。"},
		{Name: "name", Type: "string", Required: true, Description: "球队名称。"},
		{Name: "active", Type: "boolean", Required: true, Description: "球队当前是否活跃。"},
	}
	playerFields := []TopicFieldDescription{
		{Name: "membership_id", Type: "string", Required: true, Description: "名单记录 ID。"},
		{Name: "team_id", Type: "string", Required: true, Description: "球队 ID。"},
		{Name: "player_id", Type: "string", Required: true, Description: "球员 ID。"},
		{Name: "name", Type: "string", Required: true, Description: "球员姓名。"},
		{Name: "jersey_number", Type: "number", Required: true, Description: "球衣号码。"},
		{Name: "batting_hands", Type: "string[]", Required: true, Description: "打击手别。"},
		{Name: "throwing_hands", Type: "string[]", Required: true, Description: "投球手别。"},
		{Name: "positions", Type: "string[]", Required: true, Description: "守备位置。"},
		{Name: "joined_at", Type: "string", Required: true, Description: "加入日期，格式 YYYY-MM-DD。"},
		{Name: "left_at", Type: "string|null", Required: true, Description: "离队日期；当前效力时为 null。"},
		{Name: "active", Type: "boolean", Required: true, Description: "该名单记录当前是否有效。"},
	}
	return map[string]topic{
		"runtime": {
			name:    "runtime",
			kind:    "runtime",
			summary: "受限 Lua 5.1 运行时、入口函数与资源限制。",
		},
		"roster": {
			name:            "roster",
			kind:            "module",
			summary:         "球队、球员与名单读取。",
			requiresModules: []string{"roster"},
			children:        []string{"roster.teams", "roster.players"},
		},
		"roster.teams": {
			name:            "roster.teams",
			kind:            "command",
			summary:         "列出球队，可选只返回活跃球队。",
			requiresModules: []string{"roster"},
			call:            "team.roster.teams({active=true})",
			parameters: []TopicFieldDescription{
				{Name: "active", Type: "boolean", Required: false, Description: "可选；true 时仅返回活跃球队，省略或 false 时返回全部球队。"},
			},
			resultType: "array<team>",
			returns:    teamFields,
			example: `function main(team)
  return team.roster.teams({active = true})
end`,
		},
		"roster.players": {
			name:            "roster.players",
			kind:            "command",
			summary:         "按球队读取指定日期与任一守备位置匹配的名单球员。",
			requiresModules: []string{"roster"},
			call:            "team.roster.players({team_id=..., on_date=..., position_any={...}})",
			parameters: []TopicFieldDescription{
				{Name: "team_id", Type: "string", Required: true, Description: "必填的球队 ID。"},
				{Name: "on_date", Type: "string", Required: false, Description: "可选；查询日期，格式 YYYY-MM-DD。省略时查询当前名单。"},
				{Name: "position_any", Type: "string[]", Required: false, Description: "可选；匹配任一守备位置。", Values: []string{"pitcher", "catcher", "first_base", "second_base", "shortstop", "third_base", "outfielder"}},
			},
			resultType: "array<roster_player>",
			returns:    playerFields,
			example: `function main(team)
  return team.roster.players({
    team_id = "team-1",
    position_any = {"pitcher"},
  })
end`,
		},
		"game": {
			name:            "game",
			kind:            "module",
			summary:         "比赛、比分与比赛事件读取。",
			requiresModules: []string{"game"},
		},
		"lineup": {
			name:            "lineup",
			kind:            "module",
			summary:         "比赛阵容与候选阵容读取。",
			requiresModules: []string{"lineup"},
		},
		"training": {
			name:            "training",
			kind:            "module",
			summary:         "训练报告与训练推荐读取。",
			requiresModules: []string{"training"},
		},
		"analysis": {
			name:            "analysis",
			kind:            "module",
			summary:         "比赛表现与跨周期分析读取。",
			requiresModules: []string{"analysis"},
		},
	}
}
