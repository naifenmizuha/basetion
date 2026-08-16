package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

const (
	defaultConfigFile    = "config/config.toml"
	defaultEnvFile       = ".env"
	defaultSessionDir    = ".basetion/sessions"
	defaultMaxIterations = 20
)

// ErrAlreadyInitialized reports an attempt to replace the process snapshot.
var ErrAlreadyInitialized = errors.New("configuration is already initialized")

// Config is the immutable process configuration published by Init.
type Config struct {
	OpenAI   OpenAIConfig   `mapstructure:"openai"`
	Database DatabaseConfig `mapstructure:"database"`
	Session  SessionConfig  `mapstructure:"session"`
	Agent    AgentConfig    `mapstructure:"agent"`
}

type DatabaseConfig struct {
	Run DatabaseProfileConfig `mapstructure:"run"`
	Dev DatabaseProfileConfig `mapstructure:"dev"`
}

type DatabaseMode string

const (
	DatabaseModeFixed     DatabaseMode = "fixed"
	DatabaseModeTemporary DatabaseMode = "temporary"
)

type DatabaseProfileConfig struct {
	Mode            DatabaseMode `mapstructure:"mode"`
	URL             string       `mapstructure:"url"`
	AdminURL        string       `mapstructure:"admin_url"`
	TemporaryPrefix string       `mapstructure:"temporary_prefix"`
}

type OpenAIConfig struct {
	Model            string `mapstructure:"model"`
	APIKeyEnv        string `mapstructure:"api_key_env"`
	APIKey           string `mapstructure:"-"`
	BaseURL          string `mapstructure:"base_url"`
	ReasoningEffort  string `mapstructure:"reasoning_effort"`
	ReasoningSummary string `mapstructure:"reasoning_summary"`
}

type SessionConfig struct {
	Dir string `mapstructure:"dir"`
}

type AgentConfig struct {
	MaxIterations   int  `mapstructure:"max_iterations"`
	UnsafeDebugData bool `mapstructure:"unsafe_debug_data"`
}

var global = struct {
	sync.RWMutex
	attempted   bool
	initialized bool
	cfg         Config
	err         error
}{}

// Init loads the process configuration from config/config.toml and optionally
// reads environment values from .env in the current working directory.
func Init() error {
	return InitFile(defaultConfigFile)
}

// InitFile loads and publishes configuration from path. It exists so callers
// such as tests can initialize the process from an isolated configuration file.
// The first initialization attempt is final, whether it succeeds or fails.
func InitFile(path string) error {
	global.Lock()
	defer global.Unlock()

	if global.attempted {
		if global.initialized {
			return ErrAlreadyInitialized
		}
		return fmt.Errorf("configuration initialization already failed: %w", global.err)
	}
	global.attempted = true

	cfg, err := loadFile(path)
	if err != nil {
		global.err = err
		return err
	}
	global.cfg = cfg
	global.initialized = true
	return nil
}

// Get returns a copy of the initialized process configuration. Calling Get
// before a successful Init is a programming error.
func Get() Config {
	global.RLock()
	defer global.RUnlock()
	if !global.initialized {
		panic("config.Get called before successful config.Init")
	}
	return global.cfg
}

func loadFile(path string) (Config, error) {
	return loadFiles(path, defaultEnvFile)
}

func loadFiles(configPath, envPath string) (Config, error) {
	dotenv, err := readDotEnv(envPath)
	if err != nil {
		return Config{}, err
	}

	cfg, err := decodeConfigFile(configPath)
	if err != nil {
		return Config{}, err
	}
	if cfg.OpenAI.APIKeyEnv != "" {
		if apiKey, ok := lookupEnvironment(cfg.OpenAI.APIKeyEnv, dotenv); ok {
			cfg.OpenAI.APIKey = strings.TrimSpace(apiKey)
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadDatabaseFile reads and validates only the database profiles. It allows
// infrastructure tests to share the project database configuration without
// requiring model credentials or publishing the process-wide snapshot.
func LoadDatabaseFile(path string) (DatabaseConfig, error) {
	cfg, err := decodeConfigFile(path)
	if err != nil {
		return DatabaseConfig{}, err
	}
	if err := cfg.Database.Validate(); err != nil {
		return DatabaseConfig{}, err
	}
	return cfg.Database, nil
}

func decodeConfigFile(path string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	v.SetDefault("openai.reasoning_effort", "high")
	v.SetDefault("openai.reasoning_summary", "detailed")
	v.SetDefault("session.dir", defaultSessionDir)
	v.SetDefault("agent.max_iterations", defaultMaxIterations)
	v.SetDefault("agent.unsafe_debug_data", false)
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read configuration file %q: %w", path, err)
	}
	var cfg Config
	if err := v.UnmarshalExact(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration file %q: %w", path, err)
	}
	cfg.OpenAI.Model = strings.TrimSpace(cfg.OpenAI.Model)
	cfg.OpenAI.APIKeyEnv = strings.TrimSpace(cfg.OpenAI.APIKeyEnv)
	cfg.OpenAI.BaseURL = strings.TrimSpace(cfg.OpenAI.BaseURL)
	cfg.OpenAI.ReasoningEffort = strings.TrimSpace(cfg.OpenAI.ReasoningEffort)
	cfg.OpenAI.ReasoningSummary = strings.TrimSpace(cfg.OpenAI.ReasoningSummary)
	cfg.Session.Dir = strings.TrimSpace(cfg.Session.Dir)
	cfg.Database.Run.trim()
	cfg.Database.Dev.trim()
	return cfg, nil
}

func readDotEnv(path string) (gotenv.Env, error) {
	environment, err := gotenv.Read(path)
	if err == nil {
		return environment, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return gotenv.Env{}, nil
	}
	return nil, fmt.Errorf("read environment file %q: %w", path, err)
}

func lookupEnvironment(name string, dotenv gotenv.Env) (string, bool) {
	if value, ok := os.LookupEnv(name); ok {
		return value, true
	}
	value, ok := dotenv[name]
	return value, ok
}

// Validate verifies required model and storage configuration.
func (c Config) Validate() error {
	var problems []error
	if c.OpenAI.Model == "" {
		problems = append(problems, errors.New("openai.model is required"))
	}
	if c.OpenAI.APIKeyEnv == "" {
		problems = append(problems, errors.New("openai.api_key_env is required"))
	}
	if c.OpenAI.APIKey == "" {
		if c.OpenAI.APIKeyEnv != "" {
			problems = append(problems, fmt.Errorf("environment variable %q referenced by openai.api_key_env is required", c.OpenAI.APIKeyEnv))
		}
	}
	if c.OpenAI.ReasoningEffort == "" {
		problems = append(problems, errors.New("openai.reasoning_effort must not be empty"))
	}
	if c.OpenAI.ReasoningSummary == "" {
		problems = append(problems, errors.New("openai.reasoning_summary must not be empty"))
	}
	if err := c.Database.Validate(); err != nil {
		problems = append(problems, err)
	}
	if c.Session.Dir == "" {
		problems = append(problems, errors.New("session.dir must not be empty"))
	}
	if c.Agent.MaxIterations <= 0 {
		problems = append(problems, errors.New("agent.max_iterations must be positive"))
	}
	return errors.Join(problems...)
}

var temporaryPrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (c *DatabaseProfileConfig) trim() {
	c.Mode = DatabaseMode(strings.TrimSpace(string(c.Mode)))
	c.URL = strings.TrimSpace(c.URL)
	c.AdminURL = strings.TrimSpace(c.AdminURL)
	c.TemporaryPrefix = strings.TrimSpace(c.TemporaryPrefix)
}

func (c DatabaseConfig) Validate() error {
	var problems []error
	if c.Run.Mode != DatabaseModeFixed {
		problems = append(problems, errors.New("database.run.mode must be fixed"))
	}
	problems = append(problems, validateDatabaseProfile("database.run", c.Run)...)
	if c.Dev.Mode != DatabaseModeFixed && c.Dev.Mode != DatabaseModeTemporary {
		problems = append(problems, errors.New("database.dev.mode must be fixed or temporary"))
	}
	problems = append(problems, validateDatabaseProfile("database.dev", c.Dev)...)
	return errors.Join(problems...)
}

func validateDatabaseProfile(path string, profile DatabaseProfileConfig) []error {
	var problems []error
	switch profile.Mode {
	case DatabaseModeFixed:
		if profile.URL == "" {
			problems = append(problems, fmt.Errorf("%s.url is required in fixed mode", path))
		}
		if profile.AdminURL != "" || profile.TemporaryPrefix != "" {
			problems = append(problems, fmt.Errorf("%s.admin_url and temporary_prefix are only valid in temporary mode", path))
		}
	case DatabaseModeTemporary:
		if profile.URL != "" {
			problems = append(problems, fmt.Errorf("%s.url is only valid in fixed mode", path))
		}
		if profile.AdminURL == "" {
			problems = append(problems, fmt.Errorf("%s.admin_url is required in temporary mode", path))
		}
		if profile.TemporaryPrefix == "" {
			problems = append(problems, fmt.Errorf("%s.temporary_prefix is required in temporary mode", path))
		} else if len(profile.TemporaryPrefix) > 40 || !temporaryPrefixPattern.MatchString(profile.TemporaryPrefix) {
			problems = append(problems, fmt.Errorf("%s.temporary_prefix must match %s and be at most 40 characters", path, temporaryPrefixPattern))
		}
	}
	return problems
}

// DiagnosticFields returns configuration safe for structured logs. Secrets are
// intentionally omitted, and unsafe payload logging remains explicit.
func (c Config) DiagnosticFields() map[string]any {
	return map[string]any{
		"model":             c.OpenAI.Model,
		"base_url_set":      c.OpenAI.BaseURL != "",
		"reasoning_effort":  c.OpenAI.ReasoningEffort,
		"reasoning_summary": c.OpenAI.ReasoningSummary,
		"database_run_mode": c.Database.Run.Mode,
		"database_dev_mode": c.Database.Dev.Mode,
		"session_dir":       c.Session.Dir,
		"max_iterations":    c.Agent.MaxIterations,
		"unsafe_debug_data": c.Agent.UnsafeDebugData,
	}
}
