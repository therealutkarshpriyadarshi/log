package types

import "time"

// LogEvent represents a parsed log entry
type LogEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Level     string            `json:"level,omitempty"`
	Source    string            `json:"source"`
	Fields    map[string]string `json:"fields,omitempty"`
	Raw       string            `json:"raw,omitempty"` // Original raw log line
}

// FilePosition tracks the current position in a file
type FilePosition struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Inode  uint64 `json:"inode"`
}

// ParserStats tracks parser performance metrics
type ParserStats struct {
	Parsed  int64 `json:"parsed"`
	Failed  int64 `json:"failed"`
	Dropped int64 `json:"dropped"`
}
