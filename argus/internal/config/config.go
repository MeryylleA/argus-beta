package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the top-level user configuration.
type Config struct {
	Keys     APIKeys  `toml:"keys"`
	Defaults Defaults `toml:"defaults"`
}

// APIKeys holds API keys for each supported provider.
type APIKeys struct {
	Anthropic string `toml:"anthropic"`
	OpenAI    string `toml:"openai"`
	GLM       string `toml:"glm"`
	Kimi      string `toml:"kimi"`
	MiniMax   string `toml:"minimax"`
}

// Defaults holds default session settings.
type Defaults struct {
	Model string `toml:"model"` // default: "claude-opus-4-6"
	Mode  string `toml:"mode"`  // "single" | "collaborative"
}

// configDir returns the path to ~/.config/argus/
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "argus"), nil
}

// configPath returns the full path to the config file.
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads config from ~/.config/argus/config.toml.
// Returns error if file doesn't exist (user needs to run `argus init`).
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Defaults: Defaults{
			Model: "claude-opus-4-6",
			Mode:  "single",
		},
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config: no config file found at %s — run 'argus init' to create one", path)
		}
		return nil, fmt.Errorf("config: failed to parse %s: %w", path, err)
	}

	return cfg, nil
}

// Save writes config to ~/.config/argus/config.toml.
// Creates the directory if it doesn't exist.
func Save(cfg *Config) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config: failed to create directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, "config.toml")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("config: failed to create %s: %w", path, err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("config: failed to write %s: %w", path, err)
	}

	return nil
}

// ToAPIKeysMap converts the Keys struct to the map format providers.NewProvider expects.
func (c *Config) ToAPIKeysMap() map[string]string {
	return map[string]string{
		"anthropic": c.Keys.Anthropic,
		"openai":    c.Keys.OpenAI,
		"glm":       c.Keys.GLM,
		"kimi":      c.Keys.Kimi,
		"minimax":   c.Keys.MiniMax,
	}
}

// ValidateForModel checks that the required API key is present for the given model.
func (c *Config) ValidateForModel(modelID string) error {
	keyMap := map[string]string{
		"claude-opus-4-6": c.Keys.Anthropic,
		"gpt-5.2":         c.Keys.OpenAI,
		"glm-5":           c.Keys.GLM,
		"kimi-k2.5":       c.Keys.Kimi,
		"minimax-m2.5":    c.Keys.MiniMax,
	}

	providerNames := map[string]string{
		"claude-opus-4-6": "anthropic",
		"gpt-5.2":         "openai",
		"glm-5":           "glm",
		"kimi-k2.5":       "kimi",
		"minimax-m2.5":    "minimax",
	}

	key, known := keyMap[modelID]
	if !known {
		return fmt.Errorf("config: unknown model %q", modelID)
	}

	if key == "" {
		return fmt.Errorf("config: API key for %q is not set — run 'argus config set-key %s <key>'",
			modelID, providerNames[modelID])
	}

	return nil
}
