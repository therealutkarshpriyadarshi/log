package parser

import (
	"testing"
)

func TestGrokParser_Parse(t *testing.T) {
	tests := []struct {
		name         string
		config       *ParserConfig
		input        string
		source       string
		wantMessage  string
		wantLevel    string
		wantFields   map[string]string
		skipFields   []string // Fields to skip in validation
	}{
		{
			name: "syslog pattern",
			config: &ParserConfig{
				Type:        ParserTypeGrok,
				GrokPattern: "syslog",
			},
			input:       "Jan 15 10:30:00 server1 myapp[1234]: Application started successfully",
			source:      "/var/log/syslog",
			wantMessage: "Application started successfully",
			wantFields: map[string]string{
				"program": "myapp",
			},
			skipFields: []string{"pid"}, // pid extraction may vary
		},
		{
			name: "java log pattern",
			config: &ParserConfig{
				Type:        ParserTypeGrok,
				GrokPattern: "java",
			},
			input:       "2024-01-15T10:30:00.123Z INFO [main] com.example.App - Starting application",
			source:      "/var/log/app.log",
			wantMessage: "Starting application",
			wantLevel:   "info",
			wantFields: map[string]string{
				"thread": "main",
				"logger": "com.example.App",
			},
		},
		{
			name: "go log pattern",
			config: &ParserConfig{
				Type:        ParserTypeGrok,
				GrokPattern: "go",
			},
			input:       "2024-01-15T10:30:00Z INFO Server started on port 8080",
			source:      "/var/log/app.log",
			wantMessage: "Server started on port 8080",
			wantLevel:   "info",
		},
		{
			name: "python log pattern",
			config: &ParserConfig{
				Type:        ParserTypeGrok,
				GrokPattern: "python",
			},
			input:       "2024-01-15T10:30:00.123Z - myapp.module - ERROR - Database connection failed",
			source:      "/var/log/app.log",
			wantMessage: "Database connection failed",
			wantLevel:   "error",
			wantFields: map[string]string{
				"logger": "myapp.module",
			},
		},
		{
			name: "custom grok pattern",
			config: &ParserConfig{
				Type:    ParserTypeGrok,
				Pattern: `%{IP:client_ip} - %{USER:user} \[%{DATA:timestamp}\] "%{WORD:method} %{NOTSPACE:request}" %{NUMBER:status}`,
			},
			input:  `192.168.1.1 - john [15/Jan/2024:10:30:00] "GET /api/users" 200`,
			source: "/var/log/nginx.log",
			wantFields: map[string]string{
				"client_ip": "192.168.1.1",
				"user":      "john",
				"method":    "GET",
				"request":   "/api/users",
				"status":    "200",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewGrokParser(tt.config)
			if err != nil {
				t.Fatalf("Failed to create parser: %v", err)
			}

			event, err := parser.Parse(tt.input, tt.source)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if tt.wantMessage != "" && event.Message != tt.wantMessage {
				t.Errorf("Message = %v, want %v", event.Message, tt.wantMessage)
			}

			if tt.wantLevel != "" && event.Level != tt.wantLevel {
				t.Errorf("Level = %v, want %v", event.Level, tt.wantLevel)
			}

			if event.Source != tt.source {
				t.Errorf("Source = %v, want %v", event.Source, tt.source)
			}

			// Check expected fields
			skipMap := make(map[string]bool)
			for _, field := range tt.skipFields {
				skipMap[field] = true
			}

			for key, wantValue := range tt.wantFields {
				if skipMap[key] {
					continue
				}
				if gotValue, ok := event.Fields[key]; !ok {
					t.Errorf("Field %s not found in event", key)
				} else if gotValue != wantValue {
					t.Errorf("Field %s = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestExpandGrokPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr bool
	}{
		{
			name:    "simple pattern without field name",
			pattern: "%{WORD}",
			want:    `\b\w+\b`,
		},
		{
			name:    "pattern with field name",
			pattern: "%{WORD:action}",
			want:    `(?P<action>\b\w+\b)`,
		},
		{
			name:    "multiple patterns",
			pattern: "%{WORD:action} %{INT:count}",
			want:    `(?P<action>\b\w+\b) (?P<count>(?:[+-]?(?:[0-9]+)))`,
		},
		{
			name:    "unknown pattern",
			pattern: "%{UNKNOWN}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandGrokPattern(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandGrokPattern() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("expandGrokPattern() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAvailableGrokPatterns(t *testing.T) {
	patterns := GetAvailableGrokPatterns()
	if len(patterns) == 0 {
		t.Error("Expected at least one available grok pattern")
	}

	// Check for some expected patterns
	expectedPatterns := []string{"syslog", "apache", "nginx", "java", "python", "go"}
	patternMap := make(map[string]bool)
	for _, p := range patterns {
		patternMap[p] = true
	}

	for _, expected := range expectedPatterns {
		if !patternMap[expected] {
			t.Errorf("Expected pattern %s not found in available patterns", expected)
		}
	}
}

func TestNewGrokParser_InvalidPattern(t *testing.T) {
	config := &ParserConfig{
		Type:        ParserTypeGrok,
		GrokPattern: "nonexistent",
	}

	_, err := NewGrokParser(config)
	if err == nil {
		t.Error("Expected error for invalid grok pattern")
	}
}

func TestNewGrokParser_NoPattern(t *testing.T) {
	config := &ParserConfig{
		Type: ParserTypeGrok,
	}

	_, err := NewGrokParser(config)
	if err == nil {
		t.Error("Expected error when no pattern is specified")
	}
}
