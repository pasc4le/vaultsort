package config

import "time"

// Config represents the full application configuration.
type Config struct {
	Settings   Settings       `koanf:"settings"`
	LLM        LLMConfig      `koanf:"llm"`
	WatchPaths []WatchPath    `koanf:"watch_paths"`
	Rules      []RuleConfig   `koanf:"rules"`
}

// Settings holds global daemon settings.
type Settings struct {
	PollInterval int    `koanf:"poll_interval"`
	MaxFileSize  int64  `koanf:"max_file_size"`
	LogLevel     string `koanf:"log_level"`
	LogFile      string `koanf:"log_file"`
	VaultDir     string `koanf:"vault_dir"`
}

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	Provider     string  `koanf:"provider"`
	APIKey       string  `koanf:"api_key"`
	Model        string  `koanf:"model"`
	BaseURL      string  `koanf:"base_url"`
	Organization string  `koanf:"organization"`
	Timeout      int     `koanf:"timeout"`
	MaxRetries   int     `koanf:"max_retries"`
	Temperature  float64 `koanf:"temperature"`
}

// WatchPath represents a directory to monitor.
type WatchPath struct {
	Path string `koanf:"path"`
}

// RuleConfig represents a single organization rule.
type RuleConfig struct {
	Name        string       `koanf:"name"`
	Description string       `koanf:"description"`
	Enabled     bool         `koanf:"enabled"`
	Match       MatchConfig  `koanf:"match"`
	Action      ActionConfig `koanf:"action"`
}

// MatchConfig holds the matching criteria for a rule.
type MatchConfig struct {
	Extension     []string `koanf:"extension"`
	StartsWith    string   `koanf:"startswith"`
	EndsWith      string   `koanf:"endswith"`
	CreatedAfter  string   `koanf:"created_after"`
	ModifiedAfter string  `koanf:"modified_after"`
}

// ActionConfig defines what happens when a rule matches.
type ActionConfig struct {
	SendFile       bool   `koanf:"send_file"`
	SendFilename   bool   `koanf:"send_filename"`
	Prompt         string `koanf:"prompt"`
	EndDir         string `koanf:"end_dir"`
	StartDir       string `koanf:"start_dir"`
	FallbackSubdir string `koanf:"fallback_subdir"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Settings: Settings{
			PollInterval: 30,
			MaxFileSize:  1048576, // 1MB
			LogLevel:     "info",
		},
		LLM: LLMConfig{
			Provider:    "openai",
			Model:       "gpt-4o-mini",
			Timeout:     30,
			MaxRetries:  3,
			Temperature: 0.1,
		},
	}
}

// ParseDurations parses time strings from MatchConfig into time.Time values.
// Returns parsed times and any parse errors.
func (m MatchConfig) ParseDurations() (createdAfter, modifiedAfter *time.Time, err error) {
	if m.CreatedAfter != "" {
		t, e := time.Parse(time.RFC3339, m.CreatedAfter)
		if e != nil {
			return nil, nil, e
		}
		createdAfter = &t
	}
	if m.ModifiedAfter != "" {
		t, e := time.Parse(time.RFC3339, m.ModifiedAfter)
		if e != nil {
			return nil, nil, e
		}
		modifiedAfter = &t
	}
	return
}
