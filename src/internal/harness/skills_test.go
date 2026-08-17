package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestSkill(t *testing.T, root, directory, name, description, body string) {
	t.Helper()
	dir := filepath.Join(root, directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: \"" + description + "\"\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSkillBackendDiscoversRequiredSkillsAndRejectsUnknown(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "project", "project-knowledge", "project facts", "project body")
	writeTestSkill(t, root, "team", "manage-team", "team operations", "team body")

	backend, err := newSkillBackend(context.Background(), root, requiredSkillNames)
	if err != nil {
		t.Fatal(err)
	}
	matters, err := backend.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(matters) != 2 {
		t.Fatalf("discovered %d Skills, want 2", len(matters))
	}
	if _, err := backend.Get(context.Background(), "unknown"); err == nil {
		t.Fatal("unknown Skill unexpectedly loaded")
	}
}

func TestSkillBackendFailsForMissingDirectory(t *testing.T) {
	_, err := newSkillBackend(context.Background(), filepath.Join(t.TempDir(), "missing"), nil)
	if err == nil || !strings.Contains(err.Error(), "inspect Skills directory") {
		t.Fatalf("error = %v, want missing directory error", err)
	}
}

func TestSkillBackendFailsForInvalidFrontmatter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("no frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSkillBackend(context.Background(), root, nil); err == nil {
		t.Fatal("invalid frontmatter unexpectedly loaded")
	}
}

func TestSkillBackendFailsForDuplicateNames(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "first", "duplicate", "first", "one")
	writeTestSkill(t, root, "second", "duplicate", "second", "two")
	_, err := newSkillBackend(context.Background(), root, nil)
	if err == nil || !strings.Contains(err.Error(), `duplicate Skill name "duplicate"`) {
		t.Fatalf("error = %v, want duplicate name error", err)
	}
}

func TestSkillBackendFailsWhenRequiredSkillIsMissing(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "project", "project-knowledge", "project facts", "project body")
	_, err := newSkillBackend(context.Background(), root, requiredSkillNames)
	if err == nil || !strings.Contains(err.Error(), `required Skill "manage-team" is missing`) {
		t.Fatalf("error = %v, want missing required Skill error", err)
	}
}

func TestRepositorySkillsContainRequiredGuidance(t *testing.T) {
	backend, err := newSkillBackend(context.Background(), defaultSkillsDir, requiredSkillNames)
	if err != nil {
		t.Fatal(err)
	}
	project, err := backend.Get(context.Background(), "project-knowledge")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"六个职责区域", "Session", "Checkpoint", "尚未接入"} {
		if !strings.Contains(project.Content, phrase) {
			t.Errorf("project Skill missing %q", phrase)
		}
	}
	teamSkill, err := backend.Get(context.Background(), "manage-team")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{
		"team_query", "team_modify", "confirmed=true", "不得猜测", "幂等",
		"roster.teams", "roster.players", "game.matches", "game.match", "game.plates", "game.score", "lineup.list", "training.records",
		"team.create", "player.create", "player.update", "player.set_active", "roster.assign", "roster.change_jersey", "roster.leave",
		"match.create", "match.update", "match.set_status", "match.delete", "lineup.create", "lineup.replace", "lineup.delete",
		"plate.create", "plate.update", "plate.delete", "training.create", "training.update", "training.delete",
		"一次调用 `team_query` 的 `describe`", "一次启用全部必要模块", "不得仅为取得下一次查询所需的 ID 而拆分 `query`", "根目录 `describe` 仅用于",
		"原始对象只用于脚本内部计算", "不得直接作为 `main` 的返回值", "构造面向当前任务的最小证据结构",
		"除非后续 `team_modify` 确实需要稳定 ID", "已经转换为名称或业务标签的 ID 不得同时返回", "过滤、分组、聚合、排序和压缩重复模式",
	} {
		if !strings.Contains(teamSkill.Content, phrase) {
			t.Errorf("team Skill missing %q", phrase)
		}
	}
	for _, phrase := range []string{"同一轮并行执行两者的精确 `describe`", "projected_records", "player = {name = target_player.name", "local starters = {}", "local events = {}"} {
		if !strings.Contains(teamSkill.Content, phrase) {
			t.Errorf("team Skill missing projection guidance %q", phrase)
		}
	}
	if strings.Contains(teamSkill.Content, "player = target_player") || strings.Contains(teamSkill.Content, "return team.game.plates") {
		t.Error("team Skill examples directly return raw domain objects")
	}
}
