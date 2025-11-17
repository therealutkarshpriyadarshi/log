package parser

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *ParserConfig
		wantErr bool
	}{
		{
			name: "create regex parser",
			config: &ParserConfig{
				Type:    ParserTypeRegex,
				Pattern: `^(?P<message>.*)$`,
			},
			wantErr: false,
		},
		{
			name: "create json parser",
			config: &ParserConfig{
				Type: ParserTypeJSON,
			},
			wantErr: false,
		},
		{
			name: "create grok parser",
			config: &ParserConfig{
				Type:        ParserTypeGrok,
				GrokPattern: "syslog",
			},
			wantErr: false,
		},
		{
			name: "nil config",
			config: nil,
			wantErr: true,
		},
		{
			name: "unknown parser type",
			config: &ParserConfig{
				Type: "unknown",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		formats []string
		wantErr bool
	}{
		{
			name:    "RFC3339 format",
			input:   "2024-01-15T10:30:00Z",
			formats: []string{time.RFC3339},
			wantErr: false,
		},
		{
			name:    "RFC3339Nano format",
			input:   "2024-01-15T10:30:00.123456789Z",
			formats: []string{time.RFC3339Nano},
			wantErr: false,
		},
		{
			name:    "custom format",
			input:   "2024-01-15 10:30:00",
			formats: []string{"2006-01-02 15:04:05"},
			wantErr: false,
		},
		{
			name:    "default formats - RFC3339",
			input:   "2024-01-15T10:30:00Z",
			formats: nil,
			wantErr: false,
		},
		{
			name:    "default formats - apache log",
			input:   "15/Jan/2024:10:30:00 -0700",
			formats: nil,
			wantErr: false,
		},
		{
			name:    "invalid timestamp",
			input:   "invalid-timestamp",
			formats: []string{time.RFC3339},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseTimestamp(tt.input, tt.formats...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimestamp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"DEBUG", "debug"},
		{"debug", "debug"},
		{"TRACE", "debug"},
		{"trace", "debug"},
		{"INFO", "info"},
		{"info", "info"},
		{"information", "info"},
		{"WARN", "warn"},
		{"warn", "warn"},
		{"WARNING", "warn"},
		{"warning", "warn"},
		{"ERROR", "error"},
		{"error", "error"},
		{"ERR", "error"},
		{"err", "error"},
		{"FATAL", "fatal"},
		{"fatal", "fatal"},
		{"CRITICAL", "fatal"},
		{"critical", "fatal"},
		{"PANIC", "fatal"},
		{"panic", "fatal"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeLogLevel(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultParserConfig(t *testing.T) {
	config := DefaultParserConfig()

	if config.Type != ParserTypeRegex {
		t.Errorf("Default parser type = %v, want %v", config.Type, ParserTypeRegex)
	}

	if config.Pattern == "" {
		t.Error("Default pattern should not be empty")
	}

	if config.TimeFormat != time.RFC3339 {
		t.Errorf("Default time format = %v, want %v", config.TimeFormat, time.RFC3339)
	}
}

func TestDefaultTimeFormats(t *testing.T) {
	formats := DefaultTimeFormats()

	if len(formats) == 0 {
		t.Error("DefaultTimeFormats() should return at least one format")
	}

	// Check that common formats are included
	expectedFormats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
	}

	formatMap := make(map[string]bool)
	for _, f := range formats {
		formatMap[f] = true
	}

	for _, expected := range expectedFormats {
		if !formatMap[expected] {
			t.Errorf("Expected format %s not found in default formats", expected)
		}
	}
}
