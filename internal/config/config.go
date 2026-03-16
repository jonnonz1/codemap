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

	// Environment variable name that holds the API key.
	// The key itself is never stored in the config file.
	APIKeyEnv string `yaml:"api_key_env"`
}

// ScanConfig holds file scanning preferences.
type ScanConfig struct {
	// Additional directories to ignore beyond the built-in list.
	IgnoreDirs []string `yaml:"ignore_dirs,omitempty"`

	// Additional file patterns to ignore.
	IgnorePatterns []string `yaml:"ignore_patterns,omitempty"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		LLM: LLMConfig{
			Provider:  "mock",
			Model:     "",
			APIKeyEnv: "ANTHROPIC_API_KEY",
		},
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
