package config

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a time.Duration that unmarshals from a human-readable YAML string
// such as "250ms" or "30s".
type Duration time.Duration

// UnmarshalYAML parses a duration string into d.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// fileConfig mirrors the structure of config.yaml. It is pre-populated with
// defaults before unmarshalling, so only keys present in the file are overridden.
type fileConfig struct {
	Worker struct {
		PoolSize     int      `yaml:"pool_size"`
		ClaimBatch   int      `yaml:"claim_batch_size"`
		PollInterval Duration `yaml:"poll_interval"`
	} `yaml:"worker"`

	Retry struct {
		MaxAttempts int      `yaml:"max_attempts"`
		BaseBackoff Duration `yaml:"base_backoff"`
		MaxBackoff  Duration `yaml:"max_backoff"`
	} `yaml:"retry"`

	Validation struct {
		MaxBatchSize int `yaml:"max_batch_size"`
		MaxPromptLen int `yaml:"max_prompt_len"`
	} `yaml:"validation"`

	Inference struct {
		URL            string   `yaml:"url"`
		RequestTimeout Duration `yaml:"request_timeout"`
	} `yaml:"inference"`

	Mock struct {
		RatePerSec float64  `yaml:"rate_per_sec"`
		Burst      int      `yaml:"burst"`
		FailRate   float64  `yaml:"fail_rate"`
		MaxLatency Duration `yaml:"max_latency"`
	} `yaml:"mock"`

	Logging struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`

	Lifecycle struct {
		ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	} `yaml:"lifecycle"`
}

// defaultFileConfig returns the built-in defaults used when the YAML file (or an
// individual key) is absent.
func defaultFileConfig() fileConfig {
	var fc fileConfig
	fc.Worker.PoolSize = 8
	fc.Worker.ClaimBatch = 8
	fc.Worker.PollInterval = Duration(500 * time.Millisecond)

	fc.Retry.MaxAttempts = 5
	fc.Retry.BaseBackoff = Duration(250 * time.Millisecond)
	fc.Retry.MaxBackoff = Duration(30 * time.Second)

	fc.Validation.MaxBatchSize = 10000
	fc.Validation.MaxPromptLen = 8192

	fc.Inference.URL = "http://localhost:8080/mock/infer"
	fc.Inference.RequestTimeout = Duration(10 * time.Second)

	fc.Mock.RatePerSec = 20
	fc.Mock.Burst = 10
	fc.Mock.FailRate = 0.05
	fc.Mock.MaxLatency = Duration(100 * time.Millisecond)

	fc.Logging.Level = "info"
	fc.Logging.Format = "json"

	fc.Lifecycle.ShutdownTimeout = Duration(20 * time.Second)
	return fc
}

// applyTo copies the file-config values into the flat runtime Config.
func (fc fileConfig) applyTo(c *Config) {
	c.WorkerPoolSize = fc.Worker.PoolSize
	c.ClaimBatchSize = fc.Worker.ClaimBatch
	c.PollInterval = time.Duration(fc.Worker.PollInterval)

	c.MaxAttempts = fc.Retry.MaxAttempts
	c.BaseBackoff = time.Duration(fc.Retry.BaseBackoff)
	c.MaxBackoff = time.Duration(fc.Retry.MaxBackoff)

	c.MaxBatchSize = fc.Validation.MaxBatchSize
	c.MaxPromptLen = fc.Validation.MaxPromptLen

	c.InferenceURL = fc.Inference.URL
	c.RequestTimeout = time.Duration(fc.Inference.RequestTimeout)

	c.MockRatePerSec = fc.Mock.RatePerSec
	c.MockBurst = fc.Mock.Burst
	c.MockFailRate = fc.Mock.FailRate
	c.MockMaxLatency = time.Duration(fc.Mock.MaxLatency)

	c.LogLevel = fc.Logging.Level
	c.LogFormat = fc.Logging.Format

	c.ShutdownTimeout = time.Duration(fc.Lifecycle.ShutdownTimeout)
}
