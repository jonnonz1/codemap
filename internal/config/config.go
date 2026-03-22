// Package config handles codemap project configuration stored in .codemap.yaml.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const FileName = ".codemap.yaml"

// Config holds project-level codemap settings.
type Config struct {
	// LLM summarizer configuration.
	LLM LLMConfig `yaml:"llm"`

	// Cache directory relative to repo root.
	CacheDir string `yaml:"cache_dir"`

	// File scanning settings.
	Scan ScanConfig `yaml:"scan"`
}

// LLMConfig holds settings for the LLM summarizer.
type LLMConfig struct {
	// Provider name: "mock", "anthropic", "openai", "google".
	Provider string `yaml:"provider"`

	// Model identifier (e.g. "claude-haiku-4-5-20251001", "gpt-4o-mini").
	Model string `yaml:"model"`

	// API key stored directly in config. Takes precedence over APIKeyEnv.
	APIKey string `yaml:"api_key,omitempty"`

	// Environment variable name that holds the API key (fallback if api_key is empty).
	APIKeyEnv string `yaml:"api_key_env,omitempty"`

	// Max concurrent API requests (default 10).
	Workers int `yaml:"workers,omitempty"`

	// Max requests per minute (default 50). Set to 0 for no limit.
	RateLimit int `yaml:"rate_limit,omitempty"`
}

// ResolveAPIKey returns the API key, checking the config value first,
// then falling back to the environment variable.
func (c *LLMConfig) ResolveAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if c.APIKeyEnv != "" {
		return os.Getenv(c.APIKeyEnv)
	}
	return ""
}

// DefaultIgnorePatterns are file glob patterns excluded from builds by default.
// These target non-code files that rarely benefit from semantic indexing.
// Override by setting scan.ignore_patterns in .codemap.yaml.
var DefaultIgnorePatterns = []string{
	"*.md",
	"*.json",
	"*.yaml",
	"*.yml",
	"*.toml",
}

// ScanConfig holds file scanning preferences.
type ScanConfig struct {
	// Additional directories to ignore beyond the built-in list.
	IgnoreDirs []string `yaml:"ignore_dirs,omitempty"`

	// File glob patterns to ignore. Defaults to DefaultIgnorePatterns if empty.
	IgnorePatterns []string `yaml:"ignore_patterns,omitempty"`

	// Set to true to disable all default ignore patterns.
	NoDefaults bool `yaml:"no_defaults,omitempty"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		LLM:      LLMConfig{Provider: "mock"},
		CacheDir: ".claude/cache",
		Scan:     ScanConfig{},
	}
}

// Load reads the config from the given path. Returns Default() if the file
// does not exist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return nil, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config to the given path.
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	header := []byte("# codemap configuration\n# See: https://github.com/jonnonz1/codemap\n\n")
	return os.WriteFile(path, append(header, data...), 0o644)
}
