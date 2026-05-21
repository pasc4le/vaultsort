package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const (
	configDir  = ".config/vaultsort"
	stateDir   = ".local/state/vaultsort"
	configName = "config.toml"
)

// Load loads configuration from TOML file with environment variable overrides.
// configPath is optional; if empty, defaults to ~/.config/vaultsort/config.toml.
func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Determine config path
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("home directory: %w", err)
		}
		configPath = filepath.Join(home, configDir, configName)
	}

	// Load TOML file if it exists
	if _, err := os.Stat(configPath); err == nil {
		parser := toml.Parser()
		if err := k.Load(file.Provider(configPath), parser); err != nil {
			return nil, fmt.Errorf("loading config %s: %w", configPath, err)
		}
	} else if configPath != "" {
		// If a specific path was given and doesn't exist, error out
		// Only skip if using default path
		home, _ := os.UserHomeDir()
		defaultPath := filepath.Join(home, configDir, configName)
		if configPath != defaultPath {
			return nil, fmt.Errorf("config file not found: %s", configPath)
		}
	}

	// Environment variable overrides
	if v := os.Getenv("WATCH_PATH"); v != "" {
		paths := strings.Split(v, ":")
		var watchPaths []WatchPath
		for _, p := range paths {
			if p != "" {
				watchPaths = append(watchPaths, WatchPath{Path: p})
			}
		}
		if err := k.Set("watch_paths", watchPaths); err != nil {
			return nil, fmt.Errorf("setting WATCH_PATH: %w", err)
		}
	}
	if v := os.Getenv("VAULTSORT_LLM_API_KEY"); v != "" {
		if err := k.Set("llm.api_key", v); err != nil {
			return nil, fmt.Errorf("setting VAULTSORT_LLM_API_KEY: %w", err)
		}
	}
	if v := os.Getenv("VAULTSORT_LLM_BASE_URL"); v != "" {
		if err := k.Set("llm.base_url", v); err != nil {
			return nil, fmt.Errorf("setting VAULTSORT_LLM_BASE_URL: %w", err)
		}
	}
	if v := os.Getenv("VAULTSORT_LLM_PROVIDER"); v != "" {
		if err := k.Set("llm.provider", v); err != nil {
			return nil, fmt.Errorf("setting VAULTSORT_LLM_PROVIDER: %w", err)
		}
	}
	if v := os.Getenv("VAULTSORT_LLM_MODEL"); v != "" {
		if err := k.Set("llm.model", v); err != nil {
			return nil, fmt.Errorf("setting VAULTSORT_LLM_MODEL: %w", err)
		}
	}

	// Unmarshal into config struct
	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Apply defaults for zero values
	applyDefaults(&cfg)

	// Validate
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

// EnsureDirs creates config and state directories if they don't exist.
func EnsureDirs() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dirs := []string{
		filepath.Join(home, configDir),
		filepath.Join(home, stateDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
}

// ConfigPath returns the default config file path.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDir, configName)
}

// StateDir returns the default state directory path.
func StateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, stateDir)
}

// StateFile returns the path to the state JSON file.
func StateFile() string {
	return filepath.Join(StateDir(), "state.json")
}

func applyDefaults(cfg *Config) {
	def := DefaultConfig()

	if cfg.Settings.PollInterval == 0 {
		cfg.Settings.PollInterval = def.Settings.PollInterval
	}
	if cfg.Settings.MaxFileSize == 0 {
		cfg.Settings.MaxFileSize = def.Settings.MaxFileSize
	}
	if cfg.Settings.LogLevel == "" {
		cfg.Settings.LogLevel = def.Settings.LogLevel
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = def.LLM.Provider
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = def.LLM.Model
	}
	if cfg.LLM.Timeout == 0 {
		cfg.LLM.Timeout = def.LLM.Timeout
	}
	if cfg.LLM.MaxRetries == 0 {
		cfg.LLM.MaxRetries = def.LLM.MaxRetries
	}
	if cfg.LLM.Temperature == 0.0 {
		cfg.LLM.Temperature = def.LLM.Temperature
	}
}

func validate(cfg *Config) error {
	// Validate watch paths
	for _, wp := range cfg.WatchPaths {
		if wp.Path == "" {
			return fmt.Errorf("watch_path entry with empty path")
		}
		// Check directory exists (warn only; defer to runtime)
	}

	// Validate LLM provider
	validProviders := map[string]bool{
		"openai": true, "anthropic": true, "ollama": true,
		"azure": true, "custom": true,
	}
	if cfg.LLM.Provider != "" && !validProviders[cfg.LLM.Provider] {
		return fmt.Errorf("unsupported LLM provider: %s", cfg.LLM.Provider)
	}

	// Validate rules
	for i, r := range cfg.Rules {
		if r.Name == "" {
			return fmt.Errorf("rule[%d] has no name", i)
		}
		if !r.Enabled {
			continue
		}
		// Parse time constraints to validate format
		_, _, err := r.Match.ParseDurations()
		if err != nil {
			return fmt.Errorf("rule[%d] %q: invalid time format: %w", i, r.Name, err)
		}
	}

	return nil
}
