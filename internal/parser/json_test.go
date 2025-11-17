package parser

import (
	"testing"
)

func TestJSONParser_Parse(t *testing.T) {
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
			name: "basic JSON log",
			config: &ParserConfig{
				Type:         ParserTypeJSON,
				TimeField:    "timestamp",
				TimeFormat:   "2006-01-02T15:04:05Z",
				LevelField:   "level",
				MessageField: "message",
			},
			input:       `{"timestamp":"2024-01-15T10:30:00Z","level":"INFO","message":"Application started","user":"admin"}`,
			source:      "/var/log/app.log",
			wantMessage: "Application started",
			wantLevel:   "info",
			wantFields: map[string]string{
				"user": "admin",
			},
		},
		{
			name: "JSON with nested fields",
			config: &ParserConfig{
				Type:         ParserTypeJSON,
				TimeField:    "ts",
				LevelField:   "severity",
				MessageField: "msg",
			},
			input:       `{"ts":"2024-01-15T10:30:00Z","severity":"ERROR","msg":"Connection failed","error":{"code":500,"message":"Internal error"}}`,
			source:      "/var/log/app.log",
			wantMessage: "Connection failed",
			wantLevel:   "error",
			wantFields: map[string]string{
				"error": "map[code:500 message:Internal error]",
			},
		},
		{
			name: "JSON with common field names",
			config: &ParserConfig{
				Type: ParserTypeJSON,
			},
			input:       `{"timestamp":"2024-01-15T10:30:00Z","level":"WARN","msg":"Warning message","request_id":"123"}`,
			source:      "/var/log/app.log",
			wantMessage: "Warning message",
			wantLevel:   "warn",
			wantFields: map[string]string{
				"request_id": "123",
			},
		},
		{
			name: "JSON with custom fields",
			config: &ParserConfig{
				Type:         ParserTypeJSON,
				MessageField: "message",
				CustomFields: map[string]string{
					"environment": "production",
				},
			},
			input:       `{"message":"Test message","user":"alice"}`,
			source:      "/var/log/app.log",
			wantMessage: "Test message",
			wantFields: map[string]string{
				"user":        "alice",
				"environment": "production",
			},
		},
		{
			name: "invalid JSON returns as plain text",
			config: &ParserConfig{
				Type: ParserTypeJSON,
			},
			input:       `This is not JSON`,
			source:      "/var/log/app.log",
			wantMessage: "This is not JSON",
			wantFields:  map[string]string{},
		},
		{
			name: "empty line",
			config: &ParserConfig{
				Type: ParserTypeJSON,
			},
			input:   "",
			source:  "/var/log/app.log",
			wantErr: true,
		},
		{
			name: "JSON with numeric and boolean values",
			config: &ParserConfig{
				Type:         ParserTypeJSON,
				MessageField: "message",
			},
			input:       `{"message":"Test","count":42,"enabled":true,"ratio":3.14}`,
			source:      "/var/log/app.log",
			wantMessage: "Test",
			wantFields: map[string]string{
				"count":   "42",
				"enabled": "true",
				"ratio":   "3.14",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewJSONParser(tt.config)
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

func TestJSONParser_Name(t *testing.T) {
	parser := &JSONParser{}
	if parser.Name() != "json" {
		t.Errorf("Name() = %v, want %v", parser.Name(), "json")
	}
}
