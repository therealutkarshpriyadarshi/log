package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// GrokParser parses log lines using Grok patterns
type GrokParser struct {
	pattern      *regexp.Regexp
	patternName  string
	timeFormat   string
	timeField    string
	levelField   string
	messageField string
	customFields map[string]string
}

// Common Grok patterns (subset of popular patterns)
var grokPatterns = map[string]string{
	// Base patterns
	"USERNAME":   `[a-zA-Z0-9._-]+`,
	"USER":       `%{USERNAME}`,
	"INT":        `(?:[+-]?(?:[0-9]+))`,
	"NUMBER":     `(?:%{INT})`,
	"WORD":       `\b\w+\b`,
	"NOTSPACE":   `\S+`,
	"SPACE":      `\s*`,
	"DATA":       `.*?`,
	"GREEDYDATA": `.*`,

	// Date/Time patterns
	"MONTHDAY":   `(?:(?:0[1-9])|(?:[12][0-9])|(?:3[01])|[1-9])`,
	"MONTH":      `\b(?:Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\b`,
	"YEAR":       `(\d\d){1,2}`,
	"HOUR":       `(?:2[0123]|[01]?[0-9])`,
	"MINUTE":     `(?:[0-5][0-9])`,
	"SECOND":     `(?:(?:[0-5]?[0-9]|60)(?:[:.,][0-9]+)?)`,
	"TIME":       `%{HOUR}:%{MINUTE}(?::%{SECOND})?`,
	"TIMESTAMP_ISO8601": `%{YEAR}-%{MONTHDAY}-%{MONTHDAY}[T ]%{HOUR}:?%{MINUTE}(?::?%{SECOND})?%{ISO8601_TIMEZONE}?`,
	"ISO8601_TIMEZONE": `(?:Z|[+-]%{HOUR}(?::?%{MINUTE}))`,

	// Network patterns
	"IP":         `(?:%{IPV4}|%{IPV6})`,
	"IPV4":       `(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`,
	"IPV6":       `((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))`,
	"HOSTNAME":   `\b(?:[0-9A-Za-z][0-9A-Za-z-]{0,62})(?:\.(?:[0-9A-Za-z][0-9A-Za-z-]{0,62}))*(\.?|\b)`,

	// Log level patterns
	"LOGLEVEL":   `(?:DEBUG|TRACE|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL)`,

	// Common log formats
	"SYSLOGBASE": `%{MONTH} +%{MONTHDAY} %{TIME} %{HOSTNAME} %{DATA:program}(?:\[%{POSINT:pid}\])?:`,
	"COMMONAPACHELOG": `%{IPORHOST:clientip} %{USER:ident} %{USER:auth} \[%{HTTPDATE:timestamp}\] "(?:%{WORD:verb} %{NOTSPACE:request}(?: HTTP/%{NUMBER:httpversion})?|%{DATA:rawrequest})" %{NUMBER:response} (?:%{NUMBER:bytes}|-)`,
	"HTTPDATE":   `%{MONTHDAY}/%{MONTH}/%{YEAR}:%{TIME} %{INT}`,
	"IPORHOST":   `(?:%{IP}|%{HOSTNAME})`,
	"POSINT":     `\b(?:[1-9][0-9]*)\b`,
}

// Named Grok pattern templates
var namedGrokPatterns = map[string]string{
	"syslog":       `%{SYSLOGBASE} %{GREEDYDATA:message}`,
	"apache":       `%{COMMONAPACHELOG}`,
	"nginx":        `%{IPORHOST:clientip} - %{USER:ident} \[%{HTTPDATE:timestamp}\] "(?:%{WORD:verb} %{NOTSPACE:request}(?: HTTP/%{NUMBER:httpversion})?|%{DATA:rawrequest})" %{NUMBER:response} %{NUMBER:bytes} "%{DATA:referrer}" "%{DATA:agent}"`,
	"java":         `%{TIMESTAMP_ISO8601:timestamp} %{LOGLEVEL:level} \[%{DATA:thread}\] %{DATA:logger} - %{GREEDYDATA:message}`,
	"python":       `%{TIMESTAMP_ISO8601:timestamp} - %{DATA:logger} - %{LOGLEVEL:level} - %{GREEDYDATA:message}`,
	"go":           `%{TIMESTAMP_ISO8601:timestamp} %{LOGLEVEL:level} %{GREEDYDATA:message}`,
	"json":         `\{.+\}`,
}

// NewGrokParser creates a new Grok parser
func NewGrokParser(cfg *ParserConfig) (*GrokParser, error) {
	var pattern string
	var patternName string

	if cfg.GrokPattern != "" {
		// Use named pattern
		var ok bool
		pattern, ok = namedGrokPatterns[cfg.GrokPattern]
		if !ok {
			return nil, fmt.Errorf("unknown grok pattern: %s", cfg.GrokPattern)
		}
		patternName = cfg.GrokPattern
	} else if cfg.Pattern != "" {
		// Use custom pattern
		pattern = cfg.Pattern
		patternName = "custom"
	} else {
		return nil, fmt.Errorf("grok pattern or custom pattern is required")
	}

	// Expand grok pattern to regex
	expandedPattern, err := expandGrokPattern(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to expand grok pattern: %w", err)
	}

	regex, err := regexp.Compile(expandedPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile expanded pattern: %w", err)
	}

	return &GrokParser{
		pattern:      regex,
		patternName:  patternName,
		timeFormat:   cfg.TimeFormat,
		timeField:    cfg.TimeField,
		levelField:   cfg.LevelField,
		messageField: cfg.MessageField,
		customFields: cfg.CustomFields,
	}, nil
}

// expandGrokPattern expands grok pattern syntax to regex
func expandGrokPattern(pattern string) (string, error) {
	// Pattern syntax: %{PATTERN:field_name} or %{PATTERN}
	re := regexp.MustCompile(`%\{([A-Z0-9_]+)(?::([a-z0-9_]+))?\}`)

	expanded := pattern
	maxIterations := 100 // Prevent infinite loops

	for i := 0; i < maxIterations; i++ {
		matches := re.FindAllStringSubmatch(expanded, -1)
		if len(matches) == 0 {
			break
		}

		for _, match := range matches {
			patternName := match[1]
			fieldName := match[2]

			replacement, ok := grokPatterns[patternName]
			if !ok {
				return "", fmt.Errorf("unknown grok pattern: %s", patternName)
			}

			// If field name is specified, make it a named capture group
			if fieldName != "" {
				replacement = fmt.Sprintf("(?P<%s>%s)", fieldName, replacement)
			}

			expanded = strings.Replace(expanded, match[0], replacement, 1)
		}
	}

	return expanded, nil
}

// Parse parses a log line using grok pattern
func (p *GrokParser) Parse(line string, source string) (*types.LogEvent, error) {
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
	timeField := p.timeField
	if timeField == "" {
		timeField = "timestamp" // Default field name
	}

	if tsStr, ok := fields[timeField]; ok {
		var ts time.Time
		var err error

		if p.timeFormat != "" {
			ts, err = time.Parse(p.timeFormat, tsStr)
		} else {
			ts, err = ParseTimestamp(tsStr)
		}

		if err == nil {
			event.Timestamp = ts
			delete(fields, timeField)
		} else {
			event.Timestamp = time.Now()
		}
	} else {
		event.Timestamp = time.Now()
	}

	// Extract log level
	levelField := p.levelField
	if levelField == "" {
		levelField = "level" // Default field name
	}

	if level, ok := fields[levelField]; ok {
		event.Level = NormalizeLogLevel(level)
		delete(fields, levelField)
	}

	// Extract message
	messageField := p.messageField
	if messageField == "" {
		messageField = "message" // Default field name
	}

	if msg, ok := fields[messageField]; ok {
		event.Message = msg
		delete(fields, messageField)
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
func (p *GrokParser) Name() string {
	return fmt.Sprintf("grok(%s)", p.patternName)
}

// GetAvailableGrokPatterns returns list of available named grok patterns
func GetAvailableGrokPatterns() []string {
	patterns := make([]string, 0, len(namedGrokPatterns))
	for name := range namedGrokPatterns {
		patterns = append(patterns, name)
	}
	return patterns
}
