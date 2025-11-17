package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
inputs:
  files:
    - paths:
        - /var/log/app.log
        - /var/log/app2.log
      checkpoint_path: /tmp/checkpoints
      checkpoint_interval: 10s

logging:
  level: debug
  format: json

output:
  type: stdout
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(cfg.Inputs.Files) != 1 {
		t.Errorf("Expected 1 file input, got %d", len(cfg.Inputs.Files))
	}

	if len(cfg.Inputs.Files[0].Paths) != 2 {
		t.Errorf("Expected 2 paths, got %d", len(cfg.Inputs.Files[0].Paths))
	}

	if cfg.Inputs.Files[0].CheckpointInterval != 10*time.Second {
		t.Errorf("Expected checkpoint interval 10s, got %v", cfg.Inputs.Files[0].CheckpointInterval)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level debug, got %s", cfg.Logging.Level)
	}
}

func TestLoadConfigWithEnvVars(t *testing.T) {
	// Set environment variable
	os.Setenv("LOG_LEVEL", "warn")
	defer os.Unsetenv("LOG_LEVEL")

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
inputs:
  files:
    - paths:
        - /var/log/app.log

logging:
  level: ${LOG_LEVEL}
  format: json

output:
  type: stdout
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Logging.Level != "warn" {
		t.Errorf("Expected log level warn (from env var), got %s", cfg.Logging.Level)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Inputs: InputsConfig{
					Files: []FileInputConfig{
						{Paths: []string{"/var/log/app.log"}},
					},
				},
				Logging: LoggingConfig{Level: "info", Format: "json"},
				Output:  OutputConfig{Type: "stdout"},
			},
			wantErr: false,
		},
		{
			name: "no file inputs",
			config: &Config{
				Inputs:  InputsConfig{Files: []FileInputConfig{}},
				Logging: LoggingConfig{Level: "info", Format: "json"},
				Output:  OutputConfig{Type: "stdout"},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: &Config{
				Inputs: InputsConfig{
					Files: []FileInputConfig{
						{Paths: []string{"/var/log/app.log"}},
					},
				},
				Logging: LoggingConfig{Level: "invalid", Format: "json"},
				Output:  OutputConfig{Type: "stdout"},
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			config: &Config{
				Inputs: InputsConfig{
					Files: []FileInputConfig{
						{Paths: []string{"/var/log/app.log"}},
					},
				},
				Logging: LoggingConfig{Level: "info", Format: "invalid"},
				Output:  OutputConfig{Type: "stdout"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.applyDefaults()
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if err := cfg.Validate(); err != nil {
		t.Errorf("Default config should be valid: %v", err)
	}

	if cfg.Logging.Level != DefaultLogLevel {
		t.Errorf("Expected default log level %s, got %s", DefaultLogLevel, cfg.Logging.Level)
	}

	if cfg.Output.Type != "stdout" {
		t.Errorf("Expected default output type stdout, got %s", cfg.Output.Type)
	}
}
