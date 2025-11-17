package parser

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// MultilineParser handles multi-line log entries (e.g., stack traces)
type MultilineParser struct {
	baseParser   Parser
	pattern      *regexp.Regexp
	negate       bool
	match        string // "after" or "before"
	maxLines     int
	timeout      time.Duration
	buffer       []string
	bufferSource string
	lastUpdate   time.Time
	mu           sync.Mutex
}

// NewMultilineParser creates a new multiline parser
func NewMultilineParser(cfg *ParserConfig) (*MultilineParser, error) {
	if cfg.Multiline == nil {
		return nil, fmt.Errorf("multiline configuration is required")
	}

	if cfg.Multiline.Pattern == "" {
		return nil, fmt.Errorf("multiline pattern is required")
	}

	pattern, err := regexp.Compile(cfg.Multiline.Pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to compile multiline pattern: %w", err)
	}

	// Parse timeout
	timeout := 5 * time.Second
	if cfg.Multiline.Timeout != "" {
		timeout, err = time.ParseDuration(cfg.Multiline.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid multiline timeout: %w", err)
		}
	}

	// Set max lines default
	maxLines := cfg.Multiline.MaxLines
	if maxLines == 0 {
		maxLines = 500
	}

	// Create base parser for parsing the combined multi-line event
	baseParserCfg := *cfg
	baseParserCfg.Type = ParserTypeRegex
	baseParserCfg.Multiline = nil

	var baseParser Parser
	if cfg.Pattern != "" {
		baseParser, err = NewRegexParser(&baseParserCfg)
	} else {
		// Default to simple parser that returns the message as-is
		baseParser = &simpleParser{}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create base parser: %w", err)
	}

	return &MultilineParser{
		baseParser: baseParser,
		pattern:    pattern,
		negate:     cfg.Multiline.Negate,
		match:      cfg.Multiline.Match,
		maxLines:   maxLines,
		timeout:    timeout,
		buffer:     make([]string, 0),
	}, nil
}

// Parse processes a log line and handles multi-line buffering
func (p *MultilineParser) Parse(line string, source string) (*types.LogEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if line == "" {
		return nil, fmt.Errorf("empty log line")
	}

	// Check if we should flush the buffer due to timeout
	if len(p.buffer) > 0 && time.Since(p.lastUpdate) > p.timeout {
		event := p.flushBuffer()
		p.buffer = []string{line}
		p.bufferSource = source
		p.lastUpdate = time.Now()
		return event, nil
	}

	// Check if line matches the pattern
	matches := p.pattern.MatchString(line)
	if p.negate {
		matches = !matches
	}

	// Determine if this is a start of a new multi-line event
	isStart := matches

	if isStart {
		// This is the start of a new multi-line event
		var event *types.LogEvent
		if len(p.buffer) > 0 {
			// Flush the previous buffer
			event = p.flushBuffer()
		}

		// Start new buffer
		p.buffer = []string{line}
		p.bufferSource = source
		p.lastUpdate = time.Now()

		return event, nil
	} else {
		// This is a continuation line
		if len(p.buffer) == 0 {
			// No buffer yet, treat as standalone event
			p.buffer = []string{line}
			p.bufferSource = source
			p.lastUpdate = time.Now()
			return nil, nil
		}

		// Append to buffer
		if p.match == "before" {
			p.buffer = append([]string{line}, p.buffer...)
		} else {
			p.buffer = append(p.buffer, line)
		}
		p.lastUpdate = time.Now()

		// Check if buffer is full
		if len(p.buffer) >= p.maxLines {
			return p.flushBuffer(), nil
		}

		return nil, nil
	}
}

// Flush forces the parser to flush any buffered lines
func (p *MultilineParser) Flush() *types.LogEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.buffer) == 0 {
		return nil
	}

	return p.flushBuffer()
}

// flushBuffer combines buffered lines and parses them
func (p *MultilineParser) flushBuffer() *types.LogEvent {
	if len(p.buffer) == 0 {
		return nil
	}

	// Combine lines
	combined := ""
	for i, line := range p.buffer {
		if i > 0 {
			combined += "\n"
		}
		combined += line
	}

	// Parse combined line
	event, err := p.baseParser.Parse(combined, p.bufferSource)
	if err != nil {
		// Fallback to simple event
		event = &types.LogEvent{
			Timestamp: time.Now(),
			Message:   combined,
			Source:    p.bufferSource,
			Fields:    make(map[string]string),
		}
	}

	// Clear buffer
	p.buffer = make([]string, 0)

	return event
}

// Name returns the parser name
func (p *MultilineParser) Name() string {
	return "multiline"
}

// simpleParser is a fallback parser that just returns the message
type simpleParser struct{}

func (p *simpleParser) Parse(line string, source string) (*types.LogEvent, error) {
	return &types.LogEvent{
		Timestamp: time.Now(),
		Message:   line,
		Source:    source,
		Fields:    make(map[string]string),
	}, nil
}

func (p *simpleParser) Name() string {
	return "simple"
}
