package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validTOML = `[openai]
model = "gpt-from-file"
api_key = "file-secret"
base_url = "https://file.invalid/v1"

[session]
dir = "/tmp/file-sessions"

[agent]
max_iterations = 7
unsafe_debug_data = true
`

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OPENAI_MODEL",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"BASETION_SESSION_DIR",
		"BASETION_MAX_ITERATIONS",
		"BASETION_UNSAFE_DEBUG_DATA",
	} {
		value, existed := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

func TestLoadFile(t *testing.T) {
	clearConfigEnvironment(t)

	cfg, err := loadFile(writeConfig(t, validTOML))
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if cfg.OpenAI.Model != "gpt-from-file" || cfg.OpenAI.APIKey != "file-secret" || cfg.OpenAI.BaseURL != "https://file.invalid/v1" {
		t.Fatalf("unexpected OpenAI config: %#v", cfg.OpenAI)
	}
	if cfg.Session.Dir != "/tmp/file-sessions" || cfg.Agent.MaxIterations != 7 || !cfg.Agent.UnsafeDebugData {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if fields := cfg.DiagnosticFields(); fields["api_key"] != nil || strings.Contains(fmt.Sprint(fields), cfg.OpenAI.APIKey) {
		t.Fatalf("diagnostics leaked API key: %#v", fields)
	}
}

func TestLoadFileDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	path := writeConfig(t, `[openai]
model = "gpt-defaults"
api_key = "secret"
`)

	cfg, err := loadFile(path)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if cfg.Session.Dir != defaultSessionDir || cfg.Agent.MaxIterations != defaultMaxIterations || cfg.Agent.UnsafeDebugData {
		t.Fatalf("defaults not applied: %#v", cfg)
	}
}

func TestEnvironmentOverridesFile(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("OPENAI_MODEL", "gpt-from-env")
	t.Setenv("OPENAI_API_KEY", "env-secret")
	t.Setenv("OPENAI_BASE_URL", "https://env.invalid/v1")
	t.Setenv("BASETION_SESSION_DIR", "/tmp/env-sessions")
	t.Setenv("BASETION_MAX_ITERATIONS", "11")
	t.Setenv("BASETION_UNSAFE_DEBUG_DATA", "false")

	cfg, err := loadFile(writeConfig(t, validTOML))
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if cfg.OpenAI.Model != "gpt-from-env" || cfg.OpenAI.APIKey != "env-secret" || cfg.OpenAI.BaseURL != "https://env.invalid/v1" {
		t.Fatalf("environment did not override OpenAI config: %#v", cfg.OpenAI)
	}
	if cfg.Session.Dir != "/tmp/env-sessions" || cfg.Agent.MaxIterations != 11 || cfg.Agent.UnsafeDebugData {
		t.Fatalf("environment did not override config: %#v", cfg)
	}
}

func TestLoadFileRejectsMissingAndInvalidFiles(t *testing.T) {
	clearConfigEnvironment(t)

	if _, err := loadFile(filepath.Join(t.TempDir(), "missing.toml")); err == nil || !strings.Contains(err.Error(), "read configuration file") {
		t.Fatalf("missing file error = %v", err)
	}
	if _, err := loadFile(writeConfig(t, "[openai\nmodel =")); err == nil || !strings.Contains(err.Error(), "read configuration file") {
		t.Fatalf("invalid TOML error = %v", err)
	}
	invalidInteger := strings.Replace(validTOML, "max_iterations = 7", `max_iterations = "nope"`, 1)
	if _, err := loadFile(writeConfig(t, invalidInteger)); err == nil || !strings.Contains(err.Error(), "decode configuration file") {
		t.Fatalf("invalid integer error = %v", err)
	}
	invalidBoolean := strings.Replace(validTOML, "unsafe_debug_data = true", `unsafe_debug_data = "nope"`, 1)
	if _, err := loadFile(writeConfig(t, invalidBoolean)); err == nil || !strings.Contains(err.Error(), "decode configuration file") {
		t.Fatalf("invalid boolean error = %v", err)
	}
}

func TestLoadFileRejectsInvalidEnvironmentTypes(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		clearConfigEnvironment(t)
		t.Setenv("BASETION_MAX_ITERATIONS", "nope")
		if _, err := loadFile(writeConfig(t, validTOML)); err == nil || !strings.Contains(err.Error(), "decode configuration file") {
			t.Fatalf("invalid environment integer error = %v", err)
		}
	})

	t.Run("boolean", func(t *testing.T) {
		clearConfigEnvironment(t)
		t.Setenv("BASETION_UNSAFE_DEBUG_DATA", "nope")
		if _, err := loadFile(writeConfig(t, validTOML)); err == nil || !strings.Contains(err.Error(), "decode configuration file") {
			t.Fatalf("invalid environment boolean error = %v", err)
		}
	})
}

func TestLoadFileRejectsMissingRequiredValues(t *testing.T) {
	clearConfigEnvironment(t)
	path := writeConfig(t, `[openai]
model = ""
api_key = ""
`)

	_, err := loadFile(path)
	if err == nil || !strings.Contains(err.Error(), "openai.model") || !strings.Contains(err.Error(), "openai.api_key") {
		t.Fatalf("required values error = %v", err)
	}
}

func TestInitFilePublishesImmutableSnapshotOnce(t *testing.T) {
	clearConfigEnvironment(t)
	path := writeConfig(t, validTOML)

	if err := InitFile(path); err != nil {
		t.Fatalf("InitFile() error = %v", err)
	}
	cfg := Get()
	cfg.OpenAI.Model = "changed-locally"
	if got := Get().OpenAI.Model; got != "gpt-from-file" {
		t.Fatalf("global configuration was mutated through returned copy: %q", got)
	}
	if err := InitFile(path); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second InitFile() error = %v, want ErrAlreadyInitialized", err)
	}
}
