package parser

import (
	"testing"
	"time"
)

func TestRegexParser_Parse(t *testing.T) {
	tests := []struct {
		name         string
		config       *ParserConfig
		input        string
		source       string
		wantMessage  string
		wantLevel    string
		wantFields   map[string]string
		wantErr      bool
	}{
		{
			name: "basic regex pattern",
			config: &ParserConfig{
				Type:         ParserTypeRegex,
				Pattern:      `^(?P<timestamp>\S+)\s+(?P<level>\S+)\s+(?P<message>.*)$`,
				TimeField:    "timestamp",
				TimeFormat:   "2006-01-02T15:04:05",
				LevelField:   "level",
				MessageField: "message",
			},
			input:       "2024-01-15T10:30:00 INFO Application started",
			source:      "/var/log/app.log",
			wantMessage: "Application started",
			wantLevel:   "info",
			wantFields:  map[string]string{},
		},
		{
			name: "regex with custom fields",
			config: &ParserConfig{
				Type:       ParserTypeRegex,
				Pattern:    `^(?P<level>\w+):\s+(?P<message>.*)$`,
				LevelField: "level",
				MessageField: "message",
				CustomFields: map[string]string{
					"environment": "production",
					"app":         "myapp",
				},
			},
			input:       "ERROR: Database connection failed",
			source:      "/var/log/app.log",
			wantMessage: "Database connection failed",
			wantLevel:   "error",
			wantFields: map[string]string{
				"environment": "production",
				"app":         "myapp",
			},
		},
		{
			name: "regex with additional captured fields",
			config: &ParserConfig{
				Type:         ParserTypeRegex,
				Pattern:      `^(?P<timestamp>\S+)\s+\[(?P<thread>\w+)\]\s+(?P<level>\w+)\s+(?P<logger>\S+)\s+-\s+(?P<message>.*)$`,
				TimeField:    "timestamp",
				TimeFormat:   time.RFC3339,
				LevelField:   "level",
				MessageField: "message",
			},
			input:       "2024-01-15T10:30:00Z [main] INFO com.example.App - Starting application",
			source:      "/var/log/app.log",
			wantMessage: "Starting application",
			wantLevel:   "info",
			wantFields: map[string]string{
				"thread": "main",
				"logger": "com.example.App",
			},
		},
		{
			name: "no match returns raw line",
			config: &ParserConfig{
				Type:         ParserTypeRegex,
				Pattern:      `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}.*$`,
				TimeField:    "timestamp",
				LevelField:   "level",
				MessageField: "message",
			},
			input:       "Some random log line without timestamp",
			source:      "/var/log/app.log",
			wantMessage: "Some random log line without timestamp",
			wantLevel:   "",
			wantFields:  map[string]string{},
		},
		{
			name: "empty line",
			config: &ParserConfig{
				Type:    ParserTypeRegex,
				Pattern: `^(?P<message>.*)$`,
			},
			input:   "",
			source:  "/var/log/app.log",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewRegexParser(tt.config)
			if err != nil {
				t.Fatalf("Failed to create parser: %v", err)
			}

			event, err := parser.Parse(tt.input, tt.source)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if event.Message != tt.wantMessage {
				t.Errorf("Message = %v, want %v", event.Message, tt.wantMessage)
			}

			if event.Level != tt.wantLevel {
				t.Errorf("Level = %v, want %v", event.Level, tt.wantLevel)
			}

			if event.Source != tt.source {
				t.Errorf("Source = %v, want %v", event.Source, tt.source)
			}

			for key, wantValue := range tt.wantFields {
				if gotValue, ok := event.Fields[key]; !ok {
					t.Errorf("Field %s not found in event", key)
				} else if gotValue != wantValue {
					t.Errorf("Field %s = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestRegexParser_Name(t *testing.T) {
	parser := &RegexParser{}
	if parser.Name() != "regex" {
		t.Errorf("Name() = %v, want %v", parser.Name(), "regex")
	}
}

func TestNewRegexParser_InvalidPattern(t *testing.T) {
	config := &ParserConfig{
		Type:    ParserTypeRegex,
		Pattern: `[invalid(regex`,
	}

	_, err := NewRegexParser(config)
	if err == nil {
		t.Error("Expected error for invalid regex pattern")
	}
}

func TestNewRegexParser_EmptyPattern(t *testing.T) {
	config := &ParserConfig{
		Type:    ParserTypeRegex,
		Pattern: "",
	}

	_, err := NewRegexParser(config)
	if err == nil {
		t.Error("Expected error for empty regex pattern")
	}
}
