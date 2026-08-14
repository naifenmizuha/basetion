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
	defaultMaxIterations = 20
)

// ErrAlreadyInitialized reports an attempt to replace the process snapshot.
var ErrAlreadyInitialized = errors.New("configuration is already initialized")

// Config is the immutable process configuration published by Init.
type Config struct {
	OpenAI  OpenAIConfig  `mapstructure:"openai"`
	Session SessionConfig `mapstructure:"session"`
	Agent   AgentConfig   `mapstructure:"agent"`
}

type OpenAIConfig struct {
	Model     string `mapstructure:"model"`
	APIKeyEnv string `mapstructure:"api_key_env"`
	APIKey    string `mapstructure:"-"`
	BaseURL   string `mapstructure:"base_url"`
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

	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	v.SetDefault("session.dir", defaultSessionDir)
	v.SetDefault("agent.max_iterations", defaultMaxIterations)
	v.SetDefault("agent.unsafe_debug_data", false)

	environment := map[string]string{
		"openai.model":            "OPENAI_MODEL",
		"openai.api_key_env":      "OPENAI_API_KEY_ENV",
		"openai.base_url":         "OPENAI_BASE_URL",
		"session.dir":             "BASETION_SESSION_DIR",
		"agent.max_iterations":    "BASETION_MAX_ITERATIONS",
		"agent.unsafe_debug_data": "BASETION_UNSAFE_DEBUG_DATA",
	}
	for key, name := range environment {
		if value, ok := lookupEnvironment(name, dotenv); ok {
			v.Set(key, value)
		}
	}

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read configuration file %q: %w", configPath, err)
	}

	var cfg Config
	if err := v.UnmarshalExact(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode configuration file %q: %w", configPath, err)
	}
	cfg.OpenAI.Model = strings.TrimSpace(cfg.OpenAI.Model)
	cfg.OpenAI.APIKeyEnv = strings.TrimSpace(cfg.OpenAI.APIKeyEnv)
	cfg.OpenAI.BaseURL = strings.TrimSpace(cfg.OpenAI.BaseURL)
	cfg.Session.Dir = strings.TrimSpace(cfg.Session.Dir)
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
		problems = append(problems, errors.New("openai.model (OPENAI_MODEL) is required"))
	}
	if c.OpenAI.APIKeyEnv == "" {
		problems = append(problems, errors.New("openai.api_key_env (OPENAI_API_KEY_ENV) is required"))
	}
	if c.OpenAI.APIKey == "" {
		if c.OpenAI.APIKeyEnv != "" {
			problems = append(problems, fmt.Errorf("environment variable %q referenced by openai.api_key_env is required", c.OpenAI.APIKeyEnv))
		}
	}
	if c.Session.Dir == "" {
		problems = append(problems, errors.New("session.dir (BASETION_SESSION_DIR) must not be empty"))
	}
	if c.Agent.MaxIterations <= 0 {
		problems = append(problems, errors.New("agent.max_iterations (BASETION_MAX_ITERATIONS) must be positive"))
	}
	return errors.Join(problems...)
}

// DiagnosticFields returns configuration safe for structured logs. Secrets are
// intentionally omitted, and unsafe payload logging remains explicit.
func (c Config) DiagnosticFields() map[string]any {
	return map[string]any{
		"model":             c.OpenAI.Model,
		"base_url_set":      c.OpenAI.BaseURL != "",
		"session_dir":       c.Session.Dir,
		"max_iterations":    c.Agent.MaxIterations,
		"unsafe_debug_data": c.Agent.UnsafeDebugData,
	}
}
