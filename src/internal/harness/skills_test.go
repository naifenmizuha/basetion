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
	writeTestSkill(t, root, "teamops", "manage-teamops", "team operations", "teamops body")

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
	if err == nil || !strings.Contains(err.Error(), `required Skill "manage-teamops" is missing`) {
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
	teamops, err := backend.Get(context.Background(), "manage-teamops")
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"describe", "available", "只读", "不得生成模拟数据", "尚未接入"} {
		if !strings.Contains(teamops.Content, phrase) {
			t.Errorf("TeamOps Skill missing %q", phrase)
		}
	}
}
