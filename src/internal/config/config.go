package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

const (
	defaultConfigFile    = "config/config.toml"
	defaultEnvFile       = ".env"
	defaultSessionDir    = ".basetion/sessions"
	defaultLogFile       = ".basetion/basetion.log"
	defaultUserName      = "Basetion 用户"
	defaultContextWindow = 128000
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
	User     UserConfig     `mapstructure:"user"`
	Log      LogConfig      `mapstructure:"log"`
}

type DatabaseConfig struct {
	Run DatabaseProfileConfig `mapstructure:"run"`
	Dev DatabaseProfileConfig `mapstructure:"dev"`
}

type DatabaseProfileConfig struct {
	URL string `mapstructure:"url"`
}

type OpenAIConfig struct {
	Model               string `mapstructure:"model"`
	APIKeyEnv           string `mapstructure:"api_key_env"`
	APIKey              string `mapstructure:"-"`
	BaseURL             string `mapstructure:"base_url"`
	ReasoningEffort     string `mapstructure:"reasoning_effort"`
	ReasoningSummary    string `mapstructure:"reasoning_summary"`
	ContextWindowTokens int    `mapstructure:"context_window_tokens"`
}

type SessionConfig struct {
	Dir string `mapstructure:"dir"`
}

type UserConfig struct {
	Name string `mapstructure:"name"`
}

type LogConfig struct {
	File string `mapstructure:"file"`
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
// database maintenance commands and infrastructure tests to use project
// configuration without requiring model credentials or publishing a snapshot.
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
	v.SetDefault("openai.context_window_tokens", defaultContextWindow)
	v.SetDefault("session.dir", defaultSessionDir)
	v.SetDefault("user.name", defaultUserName)
	v.SetDefault("log.file", defaultLogFile)
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
	cfg.User.Name = strings.TrimSpace(cfg.User.Name)
	cfg.Log.File = strings.TrimSpace(cfg.Log.File)
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
	if c.OpenAI.APIKey == "" && c.OpenAI.APIKeyEnv != "" {
		problems = append(problems, fmt.Errorf("environment variable %q referenced by openai.api_key_env is required", c.OpenAI.APIKeyEnv))
	}
	if c.OpenAI.ReasoningEffort == "" {
		problems = append(problems, errors.New("openai.reasoning_effort must not be empty"))
	}
	if c.OpenAI.ReasoningSummary == "" {
		problems = append(problems, errors.New("openai.reasoning_summary must not be empty"))
	}
	if c.OpenAI.ContextWindowTokens <= 0 {
		problems = append(problems, errors.New("openai.context_window_tokens must be positive"))
	}
	if err := c.Database.Validate(); err != nil {
		problems = append(problems, err)
	}
	if c.Session.Dir == "" {
		problems = append(problems, errors.New("session.dir must not be empty"))
	}
	if c.User.Name == "" {
		problems = append(problems, errors.New("user.name must not be empty"))
	}
	if c.Log.File == "" {
		problems = append(problems, errors.New("log.file must not be empty"))
	}
	if c.Agent.MaxIterations <= 0 {
		problems = append(problems, errors.New("agent.max_iterations must be positive"))
	}
	return errors.Join(problems...)
}

func (c *DatabaseProfileConfig) trim() {
	c.URL = strings.TrimSpace(c.URL)
}

func (c DatabaseConfig) Validate() error {
	var problems []error
	if c.Run.URL == "" {
		problems = append(problems, errors.New("database.run.url is required"))
	}
	if c.Dev.URL == "" {
		problems = append(problems, errors.New("database.dev.url is required"))
	}
	return errors.Join(problems...)
}

// DiagnosticFields returns configuration safe for structured logs. Secrets are
// intentionally omitted, and unsafe payload logging remains explicit.
func (c Config) DiagnosticFields() map[string]any {
	return map[string]any{
		"model":                 c.OpenAI.Model,
		"base_url_set":          c.OpenAI.BaseURL != "",
		"reasoning_effort":      c.OpenAI.ReasoningEffort,
		"reasoning_summary":     c.OpenAI.ReasoningSummary,
		"database_run_set":      c.Database.Run.URL != "",
		"database_dev_set":      c.Database.Dev.URL != "",
		"session_dir":           c.Session.Dir,
		"user_name":             c.User.Name,
		"log_file":              c.Log.File,
		"context_window_tokens": c.OpenAI.ContextWindowTokens,
		"max_iterations":        c.Agent.MaxIterations,
		"unsafe_debug_data":     c.Agent.UnsafeDebugData,
	}
}
