package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration
type Config struct {
	Inputs  InputsConfig  `yaml:"inputs"`
	Logging LoggingConfig `yaml:"logging"`
	Output  OutputConfig  `yaml:"output"`
}

// InputsConfig defines input sources
type InputsConfig struct {
	Files []FileInputConfig `yaml:"files"`
}

// FileInputConfig defines file input configuration
type FileInputConfig struct {
	Paths            []string      `yaml:"paths"`
	CheckpointPath   string        `yaml:"checkpoint_path"`
	CheckpointInterval time.Duration `yaml:"checkpoint_interval"`
}

// LoggingConfig defines logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // json or console
}

// OutputConfig defines output configuration
type OutputConfig struct {
	Type string `yaml:"type"` // stdout, file, kafka, etc.
	Path string `yaml:"path,omitempty"`
}

// Default values
const (
	DefaultCheckpointPath     = "/var/lib/logaggregator/checkpoints"
	DefaultCheckpointInterval = 5 * time.Second
	DefaultLogLevel           = "info"
	DefaultLogFormat          = "json"
)

// Load loads configuration from a YAML file with environment variable overrides
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables in the YAML content
	expandedData := []byte(os.ExpandEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(expandedData, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// applyDefaults sets default values for unspecified configuration
func (c *Config) applyDefaults() {
	if c.Logging.Level == "" {
		c.Logging.Level = DefaultLogLevel
	}
	if c.Logging.Format == "" {
		c.Logging.Format = DefaultLogFormat
	}
	if c.Output.Type == "" {
		c.Output.Type = "stdout"
	}

	for i := range c.Inputs.Files {
		if c.Inputs.Files[i].CheckpointPath == "" {
			c.Inputs.Files[i].CheckpointPath = DefaultCheckpointPath
		}
		if c.Inputs.Files[i].CheckpointInterval == 0 {
			c.Inputs.Files[i].CheckpointInterval = DefaultCheckpointInterval
		}
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if len(c.Inputs.Files) == 0 {
		return fmt.Errorf("at least one file input must be configured")
	}

	for i, fileInput := range c.Inputs.Files {
		if len(fileInput.Paths) == 0 {
			return fmt.Errorf("file input %d has no paths configured", i)
		}
	}

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	validLogFormats := map[string]bool{
		"json": true, "console": true,
	}
	if !validLogFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	return nil
}

// LoadOrDefault loads configuration from file or returns a default configuration
func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		return DefaultConfig()
	}
	return cfg
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	cfg := &Config{
		Inputs: InputsConfig{
			Files: []FileInputConfig{
				{
					Paths:              []string{"/var/log/app.log"},
					CheckpointPath:     DefaultCheckpointPath,
					CheckpointInterval: DefaultCheckpointInterval,
				},
			},
		},
		Logging: LoggingConfig{
			Level:  DefaultLogLevel,
			Format: DefaultLogFormat,
		},
		Output: OutputConfig{
			Type: "stdout",
		},
	}
	return cfg
}
