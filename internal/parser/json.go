package parser

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// JSONParser parses JSON-formatted log lines
type JSONParser struct {
	timeField    string
	timeFormat   string
	levelField   string
	messageField string
	customFields map[string]string
}

// NewJSONParser creates a new JSON parser
func NewJSONParser(cfg *ParserConfig) (*JSONParser, error) {
	return &JSONParser{
		timeField:    cfg.TimeField,
		timeFormat:   cfg.TimeFormat,
		levelField:   cfg.LevelField,
		messageField: cfg.MessageField,
		customFields: cfg.CustomFields,
	}, nil
}

// Parse parses a JSON log line
func (p *JSONParser) Parse(line string, source string) (*types.LogEvent, error) {
	if line == "" {
		return nil, fmt.Errorf("empty log line")
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		// If not valid JSON, return as plain message
		return &types.LogEvent{
			Timestamp: time.Now(),
			Message:   line,
			Source:    source,
			Fields:    make(map[string]string),
		}, nil
	}

	event := &types.LogEvent{
		Source: source,
		Fields: make(map[string]string),
	}

	// Extract timestamp
	timestamp := time.Now()
	if p.timeField != "" {
		if tsVal, ok := data[p.timeField]; ok {
			if tsStr, ok := tsVal.(string); ok {
				var err error
				if p.timeFormat != "" {
					timestamp, err = time.Parse(p.timeFormat, tsStr)
				} else {
					timestamp, err = ParseTimestamp(tsStr)
				}
				if err == nil {
					delete(data, p.timeField)
				}
			}
		}
	}
	event.Timestamp = timestamp

	// Extract log level
	if p.levelField != "" {
		if levelVal, ok := data[p.levelField]; ok {
			if levelStr, ok := levelVal.(string); ok {
				event.Level = NormalizeLogLevel(levelStr)
				delete(data, p.levelField)
			}
		}
	} else {
		// Try common level field names if not specified
		for _, field := range []string{"level", "severity", "loglevel", "log_level"} {
			if levelVal, ok := data[field]; ok {
				if levelStr, ok := levelVal.(string); ok {
					event.Level = NormalizeLogLevel(levelStr)
					delete(data, field)
					break
				}
			}
		}
	}

	// Extract message
	if p.messageField != "" {
		if msgVal, ok := data[p.messageField]; ok {
			if msgStr, ok := msgVal.(string); ok {
				event.Message = msgStr
				delete(data, p.messageField)
			}
		}
	}

	// If no message was extracted, try common field names
	if event.Message == "" {
		for _, field := range []string{"msg", "message", "text", "log"} {
			if msgVal, ok := data[field]; ok {
				if msgStr, ok := msgVal.(string); ok {
					event.Message = msgStr
					delete(data, field)
					break
				}
			}
		}
	}

	// If still no message, use the entire line
	if event.Message == "" {
		event.Message = line
	}

	// Convert remaining fields to strings
	for key, value := range data {
		event.Fields[key] = fmt.Sprintf("%v", value)
	}

	// Add custom fields
	for key, value := range p.customFields {
		event.Fields[key] = value
	}

	return event, nil
}

// Name returns the parser name
func (p *JSONParser) Name() string {
	return "json"
}
