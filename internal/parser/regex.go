package parser

import (
	"fmt"
	"regexp"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// RegexParser parses log lines using regular expressions
type RegexParser struct {
	pattern      *regexp.Regexp
	timeFormat   string
	timeField    string
	levelField   string
	messageField string
	customFields map[string]string
}

// NewRegexParser creates a new regex parser
func NewRegexParser(cfg *ParserConfig) (*RegexParser, error) {
	if cfg.Pattern == "" {
		return nil, fmt.Errorf("regex pattern is required")
	}

	pattern, err := regexp.Compile(cfg.Pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex pattern: %w", err)
	}

	return &RegexParser{
		pattern:      pattern,
		timeFormat:   cfg.TimeFormat,
		timeField:    cfg.TimeField,
		levelField:   cfg.LevelField,
		messageField: cfg.MessageField,
		customFields: cfg.CustomFields,
	}, nil
}

// Parse parses a log line using regex pattern matching
func (p *RegexParser) Parse(line string, source string) (*types.LogEvent, error) {
	if line == "" {
		return nil, fmt.Errorf("empty log line")
	}

	match := p.pattern.FindStringSubmatch(line)
	if match == nil {
		// If no match, return the raw line as message
		return &types.LogEvent{
			Timestamp: time.Now(),
			Message:   line,
			Source:    source,
			Fields:    make(map[string]string),
		}, nil
	}

	// Extract named groups
	fields := make(map[string]string)
	for i, name := range p.pattern.SubexpNames() {
		if i != 0 && name != "" && i < len(match) {
			fields[name] = match[i]
		}
	}

	// Build log event
	event := &types.LogEvent{
		Source: source,
		Fields: fields,
	}

	// Extract timestamp
	if p.timeField != "" {
		if tsStr, ok := fields[p.timeField]; ok {
			var ts time.Time
			var err error

			if p.timeFormat != "" {
				ts, err = time.Parse(p.timeFormat, tsStr)
			} else {
				ts, err = ParseTimestamp(tsStr)
			}

			if err == nil {
				event.Timestamp = ts
				delete(fields, p.timeField) // Remove from fields to avoid duplication
			} else {
				event.Timestamp = time.Now()
			}
		} else {
			event.Timestamp = time.Now()
		}
	} else {
		event.Timestamp = time.Now()
	}

	// Extract log level
	if p.levelField != "" {
		if level, ok := fields[p.levelField]; ok {
			event.Level = NormalizeLogLevel(level)
			delete(fields, p.levelField) // Remove from fields to avoid duplication
		}
	}

	// Extract message
	if p.messageField != "" {
		if msg, ok := fields[p.messageField]; ok {
			event.Message = msg
			delete(fields, p.messageField) // Remove from fields to avoid duplication
		}
	} else {
		event.Message = line
	}

	// Add custom fields
	for key, value := range p.customFields {
		fields[key] = value
	}

	return event, nil
}

// Name returns the parser name
func (p *RegexParser) Name() string {
	return "regex"
}
