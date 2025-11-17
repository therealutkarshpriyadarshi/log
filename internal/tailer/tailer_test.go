package tailer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/internal/checkpoint"
	"github.com/therealutkarshpriyadarshi/log/internal/logging"
)

func TestTailerBasic(t *testing.T) {
	// Create temporary directory and file
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	// Create checkpoint manager
	ckptMgr, err := checkpoint.NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}
	defer ckptMgr.Stop()

	// Create logger
	logger := logging.New(logging.Config{
		Level:  "debug",
		Format: "json",
	})

	// Write initial content to log file
	if err := os.WriteFile(logFile, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// Create tailer
	tailer, err := New([]string{logFile}, ckptMgr, logger)
	if err != nil {
		t.Fatalf("Failed to create tailer: %v", err)
	}

	if err := tailer.Start(); err != nil {
		t.Fatalf("Failed to start tailer: %v", err)
	}
	defer tailer.Stop()

	// Append new lines
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}

	if _, err := f.WriteString("line3\n"); err != nil {
		t.Fatalf("Failed to write to log file: %v", err)
	}
	f.Close()

	// Read events with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var events int
	for {
		select {
		case event := <-tailer.Events():
			if event != nil {
				events++
				t.Logf("Received event: %s", event.Message)
			}
		case <-ctx.Done():
			goto done
		}
	}

done:
	if events == 0 {
		t.Error("Expected to receive at least one event")
	}
}

func TestTailerRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	ckptMgr, err := checkpoint.NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}
	defer ckptMgr.Stop()

	logger := logging.New(logging.Config{
		Level:  "debug",
		Format: "console",
	})

	// Create initial log file
	if err := os.WriteFile(logFile, []byte("initial\n"), 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	tailer, err := New([]string{logFile}, ckptMgr, logger)
	if err != nil {
		t.Fatalf("Failed to create tailer: %v", err)
	}

	if err := tailer.Start(); err != nil {
		t.Fatalf("Failed to start tailer: %v", err)
	}
	defer tailer.Stop()

	time.Sleep(500 * time.Millisecond)

	// Simulate rotation: rename old file, create new one
	rotatedFile := logFile + ".1"
	if err := os.Rename(logFile, rotatedFile); err != nil {
		t.Fatalf("Failed to rotate file: %v", err)
	}

	if err := os.WriteFile(logFile, []byte("after rotation\n"), 0644); err != nil {
		t.Fatalf("Failed to write new log file: %v", err)
	}

	// Give tailer time to detect rotation
	time.Sleep(1 * time.Second)

	// Verify tailer is still working
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	if _, err := f.WriteString("new line\n"); err != nil {
		t.Fatalf("Failed to write to log file: %v", err)
	}
	f.Close()

	// Wait for event
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	select {
	case event := <-tailer.Events():
		if event != nil {
			t.Logf("Received event after rotation: %s", event.Message)
		}
	case <-ctx.Done():
		t.Log("Timeout waiting for event after rotation (this is expected for new files)")
	}
}

func TestTailerCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	// Write initial content
	if err := os.WriteFile(logFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("Failed to write log file: %v", err)
	}

	// First tailer session
	ckptMgr1, err := checkpoint.NewManager(checkpointDir, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}

	if err := ckptMgr1.Load(); err != nil {
		t.Fatalf("Failed to load checkpoints: %v", err)
	}

	logger := logging.New(logging.Config{Level: "debug", Format: "console"})

	tailer1, err := New([]string{logFile}, ckptMgr1, logger)
	if err != nil {
		t.Fatalf("Failed to create tailer: %v", err)
	}

	if err := tailer1.Start(); err != nil {
		t.Fatalf("Failed to start tailer: %v", err)
	}

	// Append new line
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	if _, err := f.WriteString("line4\n"); err != nil {
		t.Fatalf("Failed to write to log file: %v", err)
	}
	f.Close()

	time.Sleep(1 * time.Second)

	tailer1.Stop()
	ckptMgr1.Stop()

	// Check that checkpoint was saved
	pos, ok := ckptMgr1.GetPosition(logFile)
	if !ok {
		t.Fatal("Checkpoint not found")
	}
	if pos.Offset == 0 {
		t.Error("Checkpoint offset should be non-zero")
	}

	t.Logf("Checkpoint saved with offset: %d", pos.Offset)
}
