package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	localbk "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/schema"
)

const defaultSkillsDir = "skills"

var requiredSkillNames = []string{"project-knowledge", "query-team-data", "manage-roster", "manage-game-setup", "record-game", "manage-training"}

func newSkillMiddleware(ctx context.Context) (adk.TypedChatModelAgentMiddleware[*schema.AgenticMessage], error) {
	backend, err := newSkillBackend(ctx, defaultSkillsDir, requiredSkillNames)
	if err != nil {
		return nil, err
	}
	middleware, err := skill.NewTyped[*schema.AgenticMessage](ctx, &skill.TypedConfig[*schema.AgenticMessage]{
		Backend: backend,
	})
	if err != nil {
		return nil, fmt.Errorf("create Skill middleware: %w", err)
	}
	return middleware, nil
}

func newSkillBackend(ctx context.Context, skillsDir string, required []string) (skill.Backend, error) {
	absDir, err := filepath.Abs(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Skills directory %q: %w", skillsDir, err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("inspect Skills directory %q: %w", absDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Skills path %q is not a directory", absDir)
	}

	filesystemBackend, err := localbk.NewBackend(ctx, &localbk.Config{})
	if err != nil {
		return nil, fmt.Errorf("create local Skill filesystem: %w", err)
	}
	backend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: filesystemBackend,
		BaseDir: absDir,
	})
	if err != nil {
		return nil, fmt.Errorf("create Skill backend: %w", err)
	}
	matters, err := backend.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load Skills from %q: %w", absDir, err)
	}

	seen := make(map[string]struct{}, len(matters))
	for _, matter := range matters {
		name := strings.TrimSpace(matter.Name)
		if name == "" {
			return nil, fmt.Errorf("Skill in %q has an empty name", absDir)
		}
		if strings.TrimSpace(matter.Description) == "" {
			return nil, fmt.Errorf("Skill %q has an empty description", name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate Skill name %q", name)
		}
		seen[name] = struct{}{}
	}
	for _, name := range required {
		if _, exists := seen[name]; !exists {
			return nil, fmt.Errorf("required Skill %q is missing from %q", name, absDir)
		}
	}
	return backend, nil
}
