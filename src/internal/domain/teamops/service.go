package teamops

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const MaxProgramBytes = 32 * 1024

var (
	ErrUnknownModule     = errors.New("unknown teamops module")
	ErrModuleUnavailable = errors.New("teamops module is not available")
)

type Query struct {
	Modules []string
	Program string
}

// Executor runs a read-only TeamOps program in an isolated runtime.
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

type ModuleDescription struct {
	Name        string `json:"name"`
	Available   bool   `json:"available"`
	Description string `json:"description"`
}

type Description struct {
	Runtime RuntimeDescription  `json:"runtime"`
	Modules []ModuleDescription `json:"modules"`
}

// Service owns the stable model-facing TeamOps query contract. Baseball data
// modules remain unavailable until a public TeamOps SDK is connected.
type Service struct {
	executor Executor
	modules  []ModuleDescription
}

func NewService(executor Executor) (*Service, error) {
	if executor == nil {
		return nil, errors.New("teamops executor is required")
	}
	available := make(map[string]struct{})
	for _, name := range executor.AvailableModules() {
		available[name] = struct{}{}
	}
	modules := []ModuleDescription{
		{Name: "roster", Description: "球队、球员与名单读取；roster.teams({active=true}) 列出球队，roster.players({team_id=..., on_date='YYYY-MM-DD', position_any={'pitcher'}}) 读取指定球队名单。"},
		{Name: "game", Description: "比赛、比分与比赛事件读取。"},
		{Name: "lineup", Description: "比赛阵容与候选阵容读取。"},
		{Name: "training", Description: "训练报告与训练推荐读取。"},
		{Name: "analysis", Description: "比赛表现与跨周期分析读取。"},
	}
	for index := range modules {
		_, modules[index].Available = available[modules[index].Name]
	}
	return &Service{
		executor: executor,
		modules:  modules,
	}, nil
}

func (s *Service) Describe(ctx context.Context, requested []string) (Description, error) {
	if err := ctx.Err(); err != nil {
		return Description{}, err
	}
	modules, err := s.selectModules(requested, true)
	if err != nil {
		return Description{}, err
	}
	return Description{
		Runtime: RuntimeDescription{
			Language:     "lua",
			Version:      "5.1",
			Entrypoint:   "main(teamops)",
			ReadOnly:     true,
			Libraries:    []string{"safe-base", "table", "string", "math"},
			SourceLimit:  MaxProgramBytes,
			ResultLimit:  256 * 1024,
			ExecutionTip: "先 describe 所需模块，再提交只读 Lua 查询；roster.players 必须指定 team_id。",
		},
		Modules: modules,
	}, nil
}

func (s *Service) Query(ctx context.Context, modules []string, program string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(program) > MaxProgramBytes {
		return nil, fmt.Errorf("teamops query program exceeds %d bytes", MaxProgramBytes)
	}
	program = strings.TrimSpace(program)
	if program == "" {
		return nil, errors.New("teamops query program is required")
	}
	selected, err := s.selectModules(modules, false)
	if err != nil {
		return nil, err
	}
	for _, module := range selected {
		if !module.Available {
			return nil, fmt.Errorf("%w: %s", ErrModuleUnavailable, module.Name)
		}
	}
	selectedNames := make([]string, len(selected))
	for index, module := range selected {
		selectedNames[index] = module.Name
	}
	result, err := s.executor.Execute(ctx, Query{Modules: selectedNames, Program: program})
	if err != nil {
		return nil, fmt.Errorf("execute teamops query: %w", err)
	}
	return result, nil
}

func (s *Service) selectModules(requested []string, defaultAll bool) ([]ModuleDescription, error) {
	if len(requested) == 0 {
		if !defaultAll {
			return nil, nil
		}
		modules := make([]ModuleDescription, len(s.modules))
		copy(modules, s.modules)
		return modules, nil
	}

	byName := make(map[string]ModuleDescription, len(s.modules))
	for _, module := range s.modules {
		byName[module.Name] = module
	}
	selected := make([]ModuleDescription, 0, len(requested))
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
