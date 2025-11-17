package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

// Manager manages checkpoint persistence
type Manager struct {
	mu            sync.RWMutex
	checkpointDir string
	positions     map[string]*types.FilePosition
	interval      time.Duration
	stopCh        chan struct{}
	saveCh        chan struct{}
}

// NewManager creates a new checkpoint manager
func NewManager(checkpointDir string, interval time.Duration) (*Manager, error) {
	if err := os.MkdirAll(checkpointDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	m := &Manager{
		checkpointDir: checkpointDir,
		positions:     make(map[string]*types.FilePosition),
		interval:      interval,
		stopCh:        make(chan struct{}),
		saveCh:        make(chan struct{}, 1),
	}

	return m, nil
}

// Start starts the periodic checkpoint saving
func (m *Manager) Start() {
	go m.saveLoop()
}

// Stop stops the checkpoint manager
func (m *Manager) Stop() {
	close(m.stopCh)
	m.Save() // Final save before stopping
}

// UpdatePosition updates the position for a file
func (m *Manager) UpdatePosition(path string, offset int64, inode uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.positions[path] = &types.FilePosition{
		Path:   path,
		Offset: offset,
		Inode:  inode,
	}

	// Trigger save
	select {
	case m.saveCh <- struct{}{}:
	default:
	}
}

// GetPosition retrieves the position for a file
func (m *Manager) GetPosition(path string) (*types.FilePosition, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pos, ok := m.positions[path]
	return pos, ok
}

// Load loads checkpoints from disk
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	checkpointFile := filepath.Join(m.checkpointDir, "positions.json")
	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No checkpoint file yet
		}
		return fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	var positions map[string]*types.FilePosition
	if err := json.Unmarshal(data, &positions); err != nil {
		return fmt.Errorf("failed to unmarshal checkpoint data: %w", err)
	}

	m.positions = positions
	return nil
}

// Save saves checkpoints to disk
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	checkpointFile := filepath.Join(m.checkpointDir, "positions.json")

	data, err := json.MarshalIndent(m.positions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint data: %w", err)
	}

	// Write to temporary file first, then rename for atomicity
	tmpFile := checkpointFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	if err := os.Rename(tmpFile, checkpointFile); err != nil {
		return fmt.Errorf("failed to rename checkpoint file: %w", err)
	}

	return nil
}

// saveLoop periodically saves checkpoints
func (m *Manager) saveLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.Save(); err != nil {
				// Log error but don't stop
				fmt.Fprintf(os.Stderr, "Failed to save checkpoint: %v\n", err)
			}
		case <-m.saveCh:
			// Immediate save requested
			if err := m.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to save checkpoint: %v\n", err)
			}
		case <-m.stopCh:
			return
		}
	}
}
