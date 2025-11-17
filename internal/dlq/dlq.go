package dlq

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

var (
	ErrDLQClosed = errors.New("DLQ is closed")
	ErrDLQFull   = errors.New("DLQ is full")
)

// DLQConfig holds configuration for the Dead Letter Queue
type DLQConfig struct {
	Dir         string
	MaxSize     int64 // Maximum number of events
	MaxAge      time.Duration
	FlushInterval time.Duration
}

// DeadLetterQueue stores failed events for later retry or inspection
type DeadLetterQueue struct {
	config DLQConfig

	mu       sync.RWMutex
	entries  []*DLQEntry
	file     *os.File
	closed   bool
	closeCh  chan struct{}

	// Metrics
	enqueued uint64
	dequeued uint64
	dropped  uint64
}

// DLQEntry represents an entry in the dead letter queue
type DLQEntry struct {
	Event     *types.LogEvent `json:"event"`
	Error     string          `json:"error"`
	Timestamp time.Time       `json:"timestamp"`
	Retries   int             `json:"retries"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(config DLQConfig) (*DeadLetterQueue, error) {
	if config.Dir == "" {
		return nil, fmt.Errorf("DLQ directory is required")
	}

	if config.MaxSize == 0 {
		config.MaxSize = 10000 // Default max size
	}

	if config.MaxAge == 0 {
		config.MaxAge = 24 * time.Hour // Default max age
	}

	if config.FlushInterval == 0 {
		config.FlushInterval = 5 * time.Second
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(config.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create DLQ directory: %w", err)
	}

	dlq := &DeadLetterQueue{
		config:  config,
		entries: make([]*DLQEntry, 0),
		closeCh: make(chan struct{}),
	}

	// Load existing entries
	if err := dlq.load(); err != nil {
		return nil, fmt.Errorf("failed to load DLQ: %w", err)
	}

	// Start background flush
	go dlq.flushLoop()

	// Start background cleanup
	go dlq.cleanupLoop()

	return dlq, nil
}

// Enqueue adds a failed event to the DLQ
func (dlq *DeadLetterQueue) Enqueue(event *types.LogEvent, err error, metadata map[string]string) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if dlq.closed {
		return ErrDLQClosed
	}

	// Check if DLQ is full
	if int64(len(dlq.entries)) >= dlq.config.MaxSize {
		atomic.AddUint64(&dlq.dropped, 1)
		return ErrDLQFull
	}

	entry := &DLQEntry{
		Event:     event,
		Error:     err.Error(),
		Timestamp: time.Now(),
		Retries:   0,
		Metadata:  metadata,
	}

	dlq.entries = append(dlq.entries, entry)
	atomic.AddUint64(&dlq.enqueued, 1)

	return nil
}

// Dequeue removes and returns the oldest entry from the DLQ
func (dlq *DeadLetterQueue) Dequeue() (*DLQEntry, error) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if dlq.closed {
		return nil, ErrDLQClosed
	}

	if len(dlq.entries) == 0 {
		return nil, nil
	}

	entry := dlq.entries[0]
	dlq.entries = dlq.entries[1:]
	atomic.AddUint64(&dlq.dequeued, 1)

	return entry, nil
}

// Peek returns the oldest entry without removing it
func (dlq *DeadLetterQueue) Peek() (*DLQEntry, error) {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	if dlq.closed {
		return nil, ErrDLQClosed
	}

	if len(dlq.entries) == 0 {
		return nil, nil
	}

	return dlq.entries[0], nil
}

// GetAll returns all entries in the DLQ
func (dlq *DeadLetterQueue) GetAll() ([]*DLQEntry, error) {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	if dlq.closed {
		return nil, ErrDLQClosed
	}

	// Return a copy
	entries := make([]*DLQEntry, len(dlq.entries))
	copy(entries, dlq.entries)

	return entries, nil
}

// Size returns the number of entries in the DLQ
func (dlq *DeadLetterQueue) Size() int {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	return len(dlq.entries)
}

// Clear removes all entries from the DLQ
func (dlq *DeadLetterQueue) Clear() error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if dlq.closed {
		return ErrDLQClosed
	}

	dlq.entries = make([]*DLQEntry, 0)
	return dlq.flush()
}

// Flush persists all entries to disk
func (dlq *DeadLetterQueue) Flush() error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	return dlq.flush()
}

// Close closes the DLQ and flushes remaining entries
func (dlq *DeadLetterQueue) Close() error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if dlq.closed {
		return ErrDLQClosed
	}

	dlq.closed = true
	close(dlq.closeCh)

	// Flush remaining entries
	if err := dlq.flush(); err != nil {
		return err
	}

	if dlq.file != nil {
		return dlq.file.Close()
	}

	return nil
}

// Metrics returns DLQ statistics
func (dlq *DeadLetterQueue) Metrics() DLQMetrics {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	return DLQMetrics{
		Enqueued:    atomic.LoadUint64(&dlq.enqueued),
		Dequeued:    atomic.LoadUint64(&dlq.dequeued),
		Dropped:     atomic.LoadUint64(&dlq.dropped),
		CurrentSize: len(dlq.entries),
		MaxSize:     dlq.config.MaxSize,
	}
}

// flush persists entries to disk (must be called with lock held)
func (dlq *DeadLetterQueue) flush() error {
	filename := filepath.Join(dlq.config.Dir, "dlq.json")

	// Write to temp file first
	tempFile := filename + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	encoder := json.NewEncoder(file)
	for _, entry := range dlq.entries {
		if err := encoder.Encode(entry); err != nil {
			file.Close()
			os.Remove(tempFile)
			return fmt.Errorf("failed to encode entry: %w", err)
		}
	}

	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(tempFile)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	file.Close()

	// Atomically rename temp file
	if err := os.Rename(tempFile, filename); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// load loads entries from disk
func (dlq *DeadLetterQueue) load() error {
	filename := filepath.Join(dlq.config.Dir, "dlq.json")

	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's okay
		}
		return fmt.Errorf("failed to open DLQ file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	for {
		var entry DLQEntry
		err := decoder.Decode(&entry)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("failed to decode entry: %w", err)
		}
		dlq.entries = append(dlq.entries, &entry)
	}

	return nil
}

// flushLoop periodically flushes entries to disk
func (dlq *DeadLetterQueue) flushLoop() {
	ticker := time.NewTicker(dlq.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dlq.mu.Lock()
			_ = dlq.flush()
			dlq.mu.Unlock()
		case <-dlq.closeCh:
			return
		}
	}
}

// cleanupLoop periodically removes old entries
func (dlq *DeadLetterQueue) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dlq.cleanup()
		case <-dlq.closeCh:
			return
		}
	}
}

// cleanup removes entries older than MaxAge
func (dlq *DeadLetterQueue) cleanup() {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if dlq.closed {
		return
	}

	cutoff := time.Now().Add(-dlq.config.MaxAge)
	var remaining []*DLQEntry

	for _, entry := range dlq.entries {
		if entry.Timestamp.After(cutoff) {
			remaining = append(remaining, entry)
		}
	}

	dlq.entries = remaining
}

// Retry increments the retry count for an entry and re-enqueues it
func (dlq *DeadLetterQueue) Retry(entry *DLQEntry) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if dlq.closed {
		return ErrDLQClosed
	}

	entry.Retries++
	entry.Timestamp = time.Now()

	dlq.entries = append(dlq.entries, entry)
	return nil
}

// DLQMetrics holds DLQ statistics
type DLQMetrics struct {
	Enqueued    uint64
	Dequeued    uint64
	Dropped     uint64
	CurrentSize int
	MaxSize     int64
}

// Utilization returns the DLQ utilization percentage (0-100)
func (m DLQMetrics) Utilization() float64 {
	if m.MaxSize == 0 {
		return 0
	}
	return (float64(m.CurrentSize) / float64(m.MaxSize)) * 100.0
}
