// Package config loads service configuration. Only deployment-essential and
// secret values come from the environment (DATABASE_URL, PORT); all tuning
// parameters live in a config.yaml loaded at startup. Missing YAML fields fall
// back to built-in defaults, so the service runs even without a config file.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all runtime configuration for the service.
type Config struct {
	// From environment (deployment-specific / secret).
	Port        string
	DatabaseURL string

	// From config.yaml (tunable behavior).
	WorkerPoolSize int
	ClaimBatchSize int
	PollInterval   time.Duration

	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration

	MaxBatchSize int
	MaxPromptLen int

	InferenceURL   string
	RequestTimeout time.Duration

	MockRatePerSec float64
	MockBurst      int
	MockFailRate   float64
	MockMaxLatency time.Duration

	LogLevel  string
	LogFormat string

	ShutdownTimeout time.Duration
}

// Load reads env-provided essentials and merges the YAML file (from CONFIG_PATH,
// default "config.yaml") over built-in defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
	}

	fc := defaultFileConfig()
	path := getEnv("CONFIG_PATH", "config.yaml")
	if err := loadFile(path, &fc); err != nil {
		return nil, err
	}
	fc.applyTo(cfg)

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadFile overlays YAML from path onto fc. A missing file is not an error: the
// caller keeps the built-in defaults (useful for containers without a mounted
// config). A malformed file is an error.
func loadFile(path string, fc *fileConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, fc); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	return nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.WorkerPoolSize < 1 {
		return fmt.Errorf("worker.pool_size must be >= 1, got %d", c.WorkerPoolSize)
	}
	if c.ClaimBatchSize < 1 {
		return fmt.Errorf("worker.claim_batch_size must be >= 1, got %d", c.ClaimBatchSize)
	}
	if c.MaxAttempts < 1 {
		return fmt.Errorf("retry.max_attempts must be >= 1, got %d", c.MaxAttempts)
	}
	if c.MaxBatchSize < 1 {
		return fmt.Errorf("validation.max_batch_size must be >= 1, got %d", c.MaxBatchSize)
	}
	return nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
