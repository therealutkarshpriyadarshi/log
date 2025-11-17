package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckpointManager(t *testing.T) {
	tmpDir := t.TempDir()
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	mgr, err := NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}
	defer mgr.Stop()

	// Update position
	mgr.UpdatePosition("/var/log/test.log", 1234, 5678)

	// Retrieve position
	pos, ok := mgr.GetPosition("/var/log/test.log")
	if !ok {
		t.Fatal("Position not found")
	}

	if pos.Offset != 1234 {
		t.Errorf("Expected offset 1234, got %d", pos.Offset)
	}

	if pos.Inode != 5678 {
		t.Errorf("Expected inode 5678, got %d", pos.Inode)
	}

	// Save
	if err := mgr.Save(); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Verify file exists
	checkpointFile := filepath.Join(checkpointDir, "positions.json")
	if _, err := os.Stat(checkpointFile); os.IsNotExist(err) {
		t.Fatal("Checkpoint file was not created")
	}
}

func TestCheckpointLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	// First manager
	mgr1, err := NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}

	mgr1.UpdatePosition("/var/log/app.log", 1000, 123)
	mgr1.UpdatePosition("/var/log/app2.log", 2000, 456)

	if err := mgr1.Save(); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}
	mgr1.Stop()

	// Second manager - load from disk
	mgr2, err := NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create second checkpoint manager: %v", err)
	}
	defer mgr2.Stop()

	if err := mgr2.Load(); err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Verify positions were loaded
	pos1, ok := mgr2.GetPosition("/var/log/app.log")
	if !ok {
		t.Fatal("Position 1 not found after load")
	}
	if pos1.Offset != 1000 || pos1.Inode != 123 {
		t.Errorf("Position 1 mismatch: offset=%d, inode=%d", pos1.Offset, pos1.Inode)
	}

	pos2, ok := mgr2.GetPosition("/var/log/app2.log")
	if !ok {
		t.Fatal("Position 2 not found after load")
	}
	if pos2.Offset != 2000 || pos2.Inode != 456 {
		t.Errorf("Position 2 mismatch: offset=%d, inode=%d", pos2.Offset, pos2.Inode)
	}
}

func TestCheckpointPeriodic(t *testing.T) {
	tmpDir := t.TempDir()
	checkpointDir := filepath.Join(tmpDir, "checkpoints")

	mgr, err := NewManager(checkpointDir, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}

	mgr.Start()

	mgr.UpdatePosition("/var/log/test.log", 9999, 1111)

	// Wait for periodic save
	time.Sleep(1 * time.Second)

	mgr.Stop()

	// Load in new manager
	mgr2, err := NewManager(checkpointDir, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to create second manager: %v", err)
	}
	defer mgr2.Stop()

	if err := mgr2.Load(); err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	pos, ok := mgr2.GetPosition("/var/log/test.log")
	if !ok {
		t.Fatal("Position not found after periodic save")
	}

	if pos.Offset != 9999 {
		t.Errorf("Expected offset 9999, got %d", pos.Offset)
	}
}
