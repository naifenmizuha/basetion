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
api_key_env = "TEST_OPENAI_API_KEY"
base_url = "https://file.invalid/v1"

[database.run]
mode = "fixed"
url = "postgres://file.invalid/basetion"

[database.dev]
mode = "temporary"
admin_url = "postgres://file.invalid/postgres"
temporary_prefix = "basetion_test"

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

func writeDotEnv(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadWithoutDotEnv(t *testing.T, contents string) (Config, error) {
	t.Helper()
	return loadFiles(writeConfig(t, contents), filepath.Join(t.TempDir(), "missing.env"))
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OPENAI_MODEL",
		"OPENAI_API_KEY",
		"OPENAI_API_KEY_ENV",
		"OPENAI_BASE_URL",
		"BASETION_SESSION_DIR",
		"BASETION_DATABASE_URL",
		"BASETION_MAX_ITERATIONS",
		"BASETION_UNSAFE_DEBUG_DATA",
		"TEST_OPENAI_API_KEY",
		"ENV_OPENAI_API_KEY",
		"DOTENV_OPENAI_API_KEY",
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
	t.Setenv("TEST_OPENAI_API_KEY", "file-secret")

	cfg, err := loadWithoutDotEnv(t, validTOML)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if cfg.OpenAI.Model != "gpt-from-file" || cfg.OpenAI.APIKeyEnv != "TEST_OPENAI_API_KEY" || cfg.OpenAI.APIKey != "file-secret" || cfg.OpenAI.BaseURL != "https://file.invalid/v1" {
		t.Fatalf("unexpected OpenAI config: %#v", cfg.OpenAI)
	}
	if cfg.Database.Run.URL != "postgres://file.invalid/basetion" || cfg.Database.Dev.Mode != DatabaseModeTemporary || cfg.Database.Dev.TemporaryPrefix != "basetion_test" || cfg.Session.Dir != "/tmp/file-sessions" || cfg.Agent.MaxIterations != 7 || !cfg.Agent.UnsafeDebugData {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if fields := cfg.DiagnosticFields(); fields["api_key"] != nil || strings.Contains(fmt.Sprint(fields), cfg.OpenAI.APIKey) {
		t.Fatalf("diagnostics leaked API key: %#v", fields)
	}
}

func TestLoadFileDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TEST_OPENAI_API_KEY", "secret")
	contents := `[openai]
model = "gpt-defaults"
api_key_env = "TEST_OPENAI_API_KEY"

[database.run]
mode = "fixed"
url = "postgres://defaults.invalid/basetion"

[database.dev]
mode = "fixed"
url = "postgres://defaults.invalid/basetion_dev"
`

	cfg, err := loadWithoutDotEnv(t, contents)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if cfg.Session.Dir != defaultSessionDir || cfg.Agent.MaxIterations != defaultMaxIterations || cfg.Agent.UnsafeDebugData {
		t.Fatalf("defaults not applied: %#v", cfg)
	}
}

func TestLoadDatabaseFileDoesNotRequireAPIKey(t *testing.T) {
	clearConfigEnvironment(t)
	database, err := LoadDatabaseFile(writeConfig(t, validTOML))
	if err != nil {
		t.Fatal(err)
	}
	if database.Run.Mode != DatabaseModeFixed || database.Dev.Mode != DatabaseModeTemporary {
		t.Fatalf("database=%#v", database)
	}
}

func TestNonSecretEnvironmentDoesNotOverrideFile(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("OPENAI_MODEL", "gpt-from-env")
	t.Setenv("OPENAI_API_KEY_ENV", "ENV_OPENAI_API_KEY")
	t.Setenv("ENV_OPENAI_API_KEY", "env-secret")
	t.Setenv("TEST_OPENAI_API_KEY", "file-secret")
	t.Setenv("OPENAI_BASE_URL", "https://env.invalid/v1")
	t.Setenv("BASETION_SESSION_DIR", "/tmp/env-sessions")
	t.Setenv("BASETION_DATABASE_URL", "postgres://env.invalid/basetion")
	t.Setenv("BASETION_MAX_ITERATIONS", "11")
	t.Setenv("BASETION_UNSAFE_DEBUG_DATA", "false")

	cfg, err := loadWithoutDotEnv(t, validTOML)
	if err != nil {
		t.Fatalf("loadFile() error = %v", err)
	}
	if cfg.OpenAI.Model != "gpt-from-file" || cfg.OpenAI.APIKeyEnv != "TEST_OPENAI_API_KEY" || cfg.OpenAI.APIKey != "file-secret" || cfg.OpenAI.BaseURL != "https://file.invalid/v1" {
		t.Fatalf("environment overrode TOML OpenAI config: %#v", cfg.OpenAI)
	}
	if cfg.Database.Run.URL != "postgres://file.invalid/basetion" || cfg.Session.Dir != "/tmp/file-sessions" || cfg.Agent.MaxIterations != 7 || !cfg.Agent.UnsafeDebugData {
		t.Fatalf("environment overrode TOML config: %#v", cfg)
	}
}

func TestDotEnvProvidesOnlyReferencedAPIKey(t *testing.T) {
	clearConfigEnvironment(t)
	dotenvPath := writeDotEnv(t, `OPENAI_MODEL=gpt-from-dotenv
TEST_OPENAI_API_KEY=dotenv-secret
OPENAI_BASE_URL=https://dotenv.invalid/v1
BASETION_DATABASE_URL=postgres://dotenv.invalid/basetion
BASETION_SESSION_DIR=/tmp/dotenv-sessions
BASETION_MAX_ITERATIONS=13
BASETION_UNSAFE_DEBUG_DATA=true
`)

	cfg, err := loadFiles(writeConfig(t, validTOML), dotenvPath)
	if err != nil {
		t.Fatalf("loadFiles() error = %v", err)
	}
	if cfg.OpenAI.Model != "gpt-from-file" || cfg.OpenAI.APIKeyEnv != "TEST_OPENAI_API_KEY" || cfg.OpenAI.APIKey != "dotenv-secret" || cfg.OpenAI.BaseURL != "https://file.invalid/v1" {
		t.Fatalf(".env did not exclusively provide API key: %#v", cfg.OpenAI)
	}
	if cfg.Database.Run.URL != "postgres://file.invalid/basetion" || cfg.Session.Dir != "/tmp/file-sessions" || cfg.Agent.MaxIterations != 7 || !cfg.Agent.UnsafeDebugData {
		t.Fatalf(".env overrode TOML config: %#v", cfg)
	}
}

func TestProcessAPIKeyOverridesDotEnv(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TEST_OPENAI_API_KEY", "process-secret")
	dotenvPath := writeDotEnv(t, "TEST_OPENAI_API_KEY=dotenv-secret\n")

	cfg, err := loadFiles(writeConfig(t, validTOML), dotenvPath)
	if err != nil {
		t.Fatalf("loadFiles() error = %v", err)
	}
	if cfg.OpenAI.Model != "gpt-from-file" || cfg.OpenAI.APIKeyEnv != "TEST_OPENAI_API_KEY" || cfg.OpenAI.APIKey != "process-secret" {
		t.Fatalf("process environment did not override .env: %#v", cfg.OpenAI)
	}
}

func TestDotEnvIsOptionalAndMalformedFileFails(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TEST_OPENAI_API_KEY", "process-secret")
	if _, err := loadWithoutDotEnv(t, validTOML); err != nil {
		t.Fatalf("missing .env error = %v", err)
	}

	malformed := writeDotEnv(t, "not valid dotenv syntax\n")
	if _, err := loadFiles(writeConfig(t, validTOML), malformed); err == nil || !strings.Contains(err.Error(), "read environment file") {
		t.Fatalf("malformed .env error = %v", err)
	}
	if _, err := loadFiles(writeConfig(t, validTOML), t.TempDir()); err == nil || !strings.Contains(err.Error(), "read environment file") {
		t.Fatalf("unreadable .env error = %v", err)
	}
}

func TestLoadFileRejectsMissingAndInvalidFiles(t *testing.T) {
	clearConfigEnvironment(t)

	missingEnv := filepath.Join(t.TempDir(), "missing.env")
	if _, err := loadFiles(filepath.Join(t.TempDir(), "missing.toml"), missingEnv); err == nil || !strings.Contains(err.Error(), "read configuration file") {
		t.Fatalf("missing file error = %v", err)
	}
	if _, err := loadWithoutDotEnv(t, "[openai\nmodel ="); err == nil || !strings.Contains(err.Error(), "read configuration file") {
		t.Fatalf("invalid TOML error = %v", err)
	}
	invalidInteger := strings.Replace(validTOML, "max_iterations = 7", `max_iterations = "nope"`, 1)
	if _, err := loadWithoutDotEnv(t, invalidInteger); err == nil || !strings.Contains(err.Error(), "decode configuration file") {
		t.Fatalf("invalid integer error = %v", err)
	}
	invalidBoolean := strings.Replace(validTOML, "unsafe_debug_data = true", `unsafe_debug_data = "nope"`, 1)
	if _, err := loadWithoutDotEnv(t, invalidBoolean); err == nil || !strings.Contains(err.Error(), "decode configuration file") {
		t.Fatalf("invalid boolean error = %v", err)
	}
}

func TestLoadFileRejectsMissingRequiredValues(t *testing.T) {
	clearConfigEnvironment(t)
	contents := `[openai]
model = ""
api_key_env = ""

[database.run]
mode = "fixed"
url = ""

[database.dev]
mode = "temporary"
admin_url = ""
temporary_prefix = "Bad-Prefix"
`

	_, err := loadWithoutDotEnv(t, contents)
	if err == nil || !strings.Contains(err.Error(), "openai.model") || !strings.Contains(err.Error(), "openai.api_key_env") || !strings.Contains(err.Error(), "database.run.url") || !strings.Contains(err.Error(), "database.dev.admin_url") || !strings.Contains(err.Error(), "temporary_prefix") {
		t.Fatalf("required values error = %v", err)
	}
}

func TestDatabaseProfileValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  DatabaseConfig
		message string
	}{
		{
			name: "run must be fixed",
			config: DatabaseConfig{
				Run: DatabaseProfileConfig{Mode: DatabaseModeTemporary, AdminURL: "postgres://admin/postgres", TemporaryPrefix: "test"},
				Dev: DatabaseProfileConfig{Mode: DatabaseModeFixed, URL: "postgres://dev/db"},
			},
			message: "database.run.mode must be fixed",
		},
		{
			name: "unknown dev mode",
			config: DatabaseConfig{
				Run: DatabaseProfileConfig{Mode: DatabaseModeFixed, URL: "postgres://run/db"},
				Dev: DatabaseProfileConfig{Mode: "other"},
			},
			message: "database.dev.mode must be fixed or temporary",
		},
		{
			name: "fixed fields are exclusive",
			config: DatabaseConfig{
				Run: DatabaseProfileConfig{Mode: DatabaseModeFixed, URL: "postgres://run/db"},
				Dev: DatabaseProfileConfig{Mode: DatabaseModeFixed, URL: "postgres://dev/db", TemporaryPrefix: "test"},
			},
			message: "only valid in temporary mode",
		},
		{
			name: "temporary fields are exclusive",
			config: DatabaseConfig{
				Run: DatabaseProfileConfig{Mode: DatabaseModeFixed, URL: "postgres://run/db"},
				Dev: DatabaseProfileConfig{Mode: DatabaseModeTemporary, URL: "postgres://dev/db", AdminURL: "postgres://admin/postgres", TemporaryPrefix: "test"},
			},
			message: "url is only valid in fixed mode",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Validate() error=%v, want %q", err, test.message)
			}
		})
	}
}

func TestLoadFileRejectsMissingAPIKeyEnvironmentValue(t *testing.T) {
	clearConfigEnvironment(t)

	_, err := loadWithoutDotEnv(t, validTOML)
	if err == nil || !strings.Contains(err.Error(), "TEST_OPENAI_API_KEY") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("missing API key environment error = %v", err)
	}

	t.Setenv("TEST_OPENAI_API_KEY", "   ")
	_, err = loadWithoutDotEnv(t, validTOML)
	if err == nil || !strings.Contains(err.Error(), "TEST_OPENAI_API_KEY") {
		t.Fatalf("blank API key environment error = %v", err)
	}
}

func TestLoadFileRejectsLegacyAPIKeyField(t *testing.T) {
	clearConfigEnvironment(t)
	contents := strings.Replace(validTOML, `api_key_env = "TEST_OPENAI_API_KEY"`, `api_key = "legacy-secret"`, 1)

	_, err := loadWithoutDotEnv(t, contents)
	if err == nil || !strings.Contains(err.Error(), "decode configuration file") || !strings.Contains(err.Error(), "api_key") || strings.Contains(err.Error(), "legacy-secret") {
		t.Fatalf("legacy api_key error = %v", err)
	}
}

func TestInitFilePublishesImmutableSnapshotOnce(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("TEST_OPENAI_API_KEY", "file-secret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(validTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(workingDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

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
