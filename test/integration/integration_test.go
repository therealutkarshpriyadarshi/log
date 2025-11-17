// +build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/checkpoint"
	"github.com/therealutkarshpriyadarshi/log/internal/config"
	"github.com/therealutkarshpriyadarshi/log/internal/input"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
	"github.com/therealutkarshpriyadarshi/log/internal/parser"
	"github.com/therealutkarshpriyadarshi/log/internal/tailer"
	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// TestFileTailerIntegration tests the complete file tailing pipeline
func TestFileTailerIntegration(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	// Create checkpoint manager
	ckptMgr, err := checkpoint.NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}
	defer ckptMgr.Stop()

	if err := ckptMgr.Load(); err != nil {
		t.Logf("No existing checkpoints: %v", err)
	}
	ckptMgr.Start()

	// Create logger
	logger := logging.New(logging.Config{Level: "info", Format: "json"})

	// Create tailer
	tailerInstance, err := tailer.New([]string{logFile}, ckptMgr, logger)
	if err != nil {
		t.Fatalf("Failed to create tailer: %v", err)
	}

	// Write some log lines
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	testLines := []string{
		"Log line 1\n",
		"Log line 2\n",
		"Log line 3\n",
	}

	for _, line := range testLines {
		if _, err := f.WriteString(line); err != nil {
			t.Fatalf("Failed to write log line: %v", err)
		}
	}
	f.Close()

	// Start tailer
	if err := tailerInstance.Start(); err != nil {
		t.Fatalf("Failed to start tailer: %v", err)
	}
	defer tailerInstance.Stop()

	// Collect events
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var receivedLines []string
	done := make(chan struct{})

	go func() {
		for {
			select {
			case event := <-tailerInstance.Events():
				receivedLines = append(receivedLines, event.Message)
				if len(receivedLines) == len(testLines) {
					close(done)
					return
				}
			case <-ctx.Done():
				close(done)
				return
			}
		}
	}()

	<-done

	// Verify received lines
	if len(receivedLines) != len(testLines) {
		t.Errorf("Expected %d lines, got %d", len(testLines), len(receivedLines))
	}

	for i, line := range receivedLines {
		if line != testLines[i] {
			t.Errorf("Line %d: expected %q, got %q", i, testLines[i], line)
		}
	}
}

// TestHTTPInputIntegration tests the HTTP input endpoint
func TestHTTPInputIntegration(t *testing.T) {
	logger := logging.New(logging.Config{Level: "info", Format: "json"})

	cfg := &input.HTTPConfig{
		Address:   "127.0.0.1:0", // Random port
		Path:      "/log",
		BatchPath: "/logs",
		APIKeys:   []string{"test-key"},
	}

	httpInput, err := input.NewHTTPInput("test-http", cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create HTTP input: %v", err)
	}

	if err := httpInput.Start(); err != nil {
		t.Fatalf("Failed to start HTTP input: %v", err)
	}
	defer httpInput.Stop()

	// Get actual listening address
	addr := httpInput.(*input.HTTPInput).Address()

	// Send a test event
	testEvent := map[string]interface{}{
		"message": "Test log message",
		"level":   "info",
		"user_id": 123,
	}

	jsonData, err := json.Marshal(testEvent)
	if err != nil {
		t.Fatalf("Failed to marshal test event: %v", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/log", addr), io.NopCloser(jsonData))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-key")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Errorf("Expected status 200 or 202, got %d", resp.StatusCode)
	}

	// Receive event
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case event := <-httpInput.Events():
		if event.Message != "Test log message" {
			t.Errorf("Expected message 'Test log message', got %q", event.Message)
		}
	case <-ctx.Done():
		t.Error("Did not receive event in time")
	}
}

// TestParserIntegration tests the parser with different formats
func TestParserIntegration(t *testing.T) {
	tests := []struct {
		name   string
		parser parser.ParserType
		config *parser.ParserConfig
		input  string
		check  func(*testing.T, *types.LogEvent)
	}{
		{
			name:   "JSON parser",
			parser: parser.ParserTypeJSON,
			config: &parser.ParserConfig{
				Type:         parser.ParserTypeJSON,
				TimeField:    "timestamp",
				LevelField:   "level",
				MessageField: "message",
			},
			input: `{"timestamp":"2024-01-01T00:00:00Z","level":"info","message":"Test message"}`,
			check: func(t *testing.T, event *types.LogEvent) {
				if event.Message != "Test message" {
					t.Errorf("Expected message 'Test message', got %q", event.Message)
				}
				if event.Level != "info" {
					t.Errorf("Expected level 'info', got %q", event.Level)
				}
			},
		},
		{
			name:   "Regex parser",
			parser: parser.ParserTypeRegex,
			config: &parser.ParserConfig{
				Type:         parser.ParserTypeRegex,
				Pattern:      `^(?P<timestamp>\S+)\s+\[(?P<level>\w+)\]\s+(?P<message>.*)$`,
				TimeField:    "timestamp",
				LevelField:   "level",
				MessageField: "message",
			},
			input: "2024-01-01T00:00:00Z [INFO] Test message",
			check: func(t *testing.T, event *types.LogEvent) {
				if event.Message != "Test message" {
					t.Errorf("Expected message 'Test message', got %q", event.Message)
				}
				if event.Level != "INFO" {
					t.Errorf("Expected level 'INFO', got %q", event.Level)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parser.New(tt.config)
			if err != nil {
				t.Fatalf("Failed to create parser: %v", err)
			}

			event, err := p.Parse(tt.input, "test-source")
			if err != nil {
				t.Fatalf("Failed to parse: %v", err)
			}

			tt.check(t, event)
		})
	}
}

// TestConfigLoadIntegration tests configuration loading with environment variables
func TestConfigLoadIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Set environment variable
	os.Setenv("TEST_LOG_LEVEL", "debug")
	defer os.Unsetenv("TEST_LOG_LEVEL")

	configYAML := `
inputs:
  files:
    - paths:
        - /var/log/test.log

logging:
  level: ${TEST_LOG_LEVEL}
  format: json

output:
  type: stdout
`

	if err := os.WriteFile(configFile, []byte(configYAML), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got %q", cfg.Logging.Level)
	}
}
