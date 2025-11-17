package parser

import (
	"fmt"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// Parser defines the interface for log parsers
type Parser interface {
	// Parse parses a raw log line into a structured LogEvent
	Parse(line string, source string) (*types.LogEvent, error)

	// Name returns the parser name
	Name() string
}

// ParserType represents different parser types
type ParserType string

const (
	ParserTypeRegex     ParserType = "regex"
	ParserTypeJSON      ParserType = "json"
	ParserTypeGrok      ParserType = "grok"
	ParserTypeMultiline ParserType = "multiline"
)

// ParserConfig holds parser configuration
type ParserConfig struct {
	Type         ParserType        `yaml:"type"`
	Pattern      string            `yaml:"pattern,omitempty"`       // For regex/grok parsers
	GrokPattern  string            `yaml:"grok_pattern,omitempty"`  // Named grok pattern
	TimeFormat   string            `yaml:"time_format,omitempty"`   // Time parsing format
	TimeField    string            `yaml:"time_field,omitempty"`    // Field containing timestamp
	LevelField   string            `yaml:"level_field,omitempty"`   // Field containing log level
	MessageField string            `yaml:"message_field,omitempty"` // Field containing message
	Multiline    *MultilineConfig  `yaml:"multiline,omitempty"`     // Multiline configuration
	CustomFields map[string]string `yaml:"custom_fields,omitempty"` // Custom fields to add
}

// MultilineConfig holds configuration for multi-line log handling
type MultilineConfig struct {
	Pattern string `yaml:"pattern"`        // Regex pattern to match continuation lines
	Negate  bool   `yaml:"negate"`         // Whether to negate the pattern match
	Match   string `yaml:"match"`          // "after" or "before" - where to append
	MaxLines int   `yaml:"max_lines"`      // Maximum lines to buffer
	Timeout  string `yaml:"timeout"`       // Timeout for incomplete multi-line events
}

// New creates a new parser based on the configuration
func New(cfg *ParserConfig) (Parser, error) {
	if cfg == nil {
		return nil, fmt.Errorf("parser configuration is nil")
	}

	switch cfg.Type {
	case ParserTypeRegex:
		return NewRegexParser(cfg)
	case ParserTypeJSON:
		return NewJSONParser(cfg)
	case ParserTypeGrok:
		return NewGrokParser(cfg)
	case ParserTypeMultiline:
		return NewMultilineParser(cfg)
	default:
		return nil, fmt.Errorf("unknown parser type: %s", cfg.Type)
	}
}

// DefaultParserConfig returns a default parser configuration
func DefaultParserConfig() *ParserConfig {
	return &ParserConfig{
		Type:         ParserTypeRegex,
		Pattern:      "^(?P<timestamp>\\S+)\\s+(?P<level>\\S+)\\s+(?P<message>.*)$",
		TimeFormat:   time.RFC3339,
		TimeField:    "timestamp",
		LevelField:   "level",
		MessageField: "message",
	}
}

// ParseTimestamp attempts to parse a timestamp from a string using multiple formats
func ParseTimestamp(ts string, formats ...string) (time.Time, error) {
	if len(formats) == 0 {
		formats = DefaultTimeFormats()
	}

	for _, format := range formats {
		if t, err := time.Parse(format, ts); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse timestamp: %s", ts)
}

// DefaultTimeFormats returns common timestamp formats
func DefaultTimeFormats() []string {
	return []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
		"2006/01/02 15:04:05",
		"Jan 02 15:04:05",
		"Jan 02, 2006 15:04:05",
		"02/Jan/2006:15:04:05 -0700",
	}
}

// NormalizeLogLevel normalizes log level strings to standard values
func NormalizeLogLevel(level string) string {
	switch level {
	case "DEBUG", "debug", "TRACE", "trace":
		return "debug"
	case "INFO", "info", "information", "INFORMATION":
		return "info"
	case "WARN", "warn", "WARNING", "warning":
		return "warn"
	case "ERROR", "error", "ERR", "err":
		return "error"
	case "FATAL", "fatal", "CRITICAL", "critical", "PANIC", "panic":
		return "fatal"
	default:
		return level
	}
}
