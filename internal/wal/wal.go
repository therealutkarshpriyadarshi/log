package wal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

var (
	ErrWALClosed    = errors.New("WAL is closed")
	ErrSegmentFull  = errors.New("segment is full")
	ErrInvalidEntry = errors.New("invalid WAL entry")
)

const (
	defaultSegmentSize = 64 * 1024 * 1024 // 64 MB
	defaultMaxSegments = 100
	segmentPrefix      = "wal-"
	segmentSuffix      = ".log"
)

// WALConfig holds configuration for the Write-Ahead Log
type WALConfig struct {
	Dir              string
	SegmentSize      int64
	MaxSegments      int
	SyncInterval     time.Duration
	CompactionPolicy CompactionPolicy
}

// CompactionPolicy defines when to compact WAL segments
type CompactionPolicy string

const (
	CompactOnSize  CompactionPolicy = "size"  // Compact when segments exceed count
	CompactOnTime  CompactionPolicy = "time"  // Compact segments older than duration
	CompactManual  CompactionPolicy = "manual" // Only compact on explicit call
)

// WAL is a Write-Ahead Log for durable event storage
type WAL struct {
	config WALConfig

	mu              sync.RWMutex
	currentSegment  *segment
	segments        []*segment
	lastSegmentID   uint64
	writePos        uint64

	closeCh         chan struct{}
	closed          bool

	// Metrics
	bytesWritten    uint64
	entriesWritten  uint64
	segmentsCreated uint64
	compactions     uint64
}

// segment represents a single WAL segment file
type segment struct {
	id       uint64
	path     string
	file     *os.File
	writer   *bufio.Writer
	size     int64
	maxSize  int64
	readOnly bool
	mu       sync.Mutex
}

// WALEntry represents a single entry in the WAL
type WALEntry struct {
	Offset    uint64           `json:"offset"`
	Timestamp time.Time        `json:"timestamp"`
	Event     *types.LogEvent  `json:"event"`
}

// NewWAL creates a new Write-Ahead Log
func NewWAL(config WALConfig) (*WAL, error) {
	if config.Dir == "" {
		return nil, fmt.Errorf("WAL directory is required")
	}

	if config.SegmentSize == 0 {
		config.SegmentSize = defaultSegmentSize
	}

	if config.MaxSegments == 0 {
		config.MaxSegments = defaultMaxSegments
	}

	if config.SyncInterval == 0 {
		config.SyncInterval = 1 * time.Second
	}

	if config.CompactionPolicy == "" {
		config.CompactionPolicy = CompactOnSize
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(config.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create WAL directory: %w", err)
	}

	w := &WAL{
		config:   config,
		segments: make([]*segment, 0),
		closeCh:  make(chan struct{}),
	}

	// Load existing segments
	if err := w.loadSegments(); err != nil {
		return nil, fmt.Errorf("failed to load segments: %w", err)
	}

	// Create initial segment if none exist
	if len(w.segments) == 0 {
		if err := w.createSegment(); err != nil {
			return nil, fmt.Errorf("failed to create initial segment: %w", err)
		}
	} else {
		w.currentSegment = w.segments[len(w.segments)-1]
	}

	// Start background sync
	go w.syncLoop()

	return w, nil
}

// Write writes an event to the WAL
func (w *WAL) Write(event *types.LogEvent) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, ErrWALClosed
	}

	// Check if we need a new segment
	if w.currentSegment.size >= w.currentSegment.maxSize {
		if err := w.createSegment(); err != nil {
			return 0, fmt.Errorf("failed to create new segment: %w", err)
		}
	}

	offset := w.writePos
	w.writePos++

	entry := WALEntry{
		Offset:    offset,
		Timestamp: time.Now(),
		Event:     event,
	}

	// Write entry to segment
	if err := w.currentSegment.writeEntry(&entry); err != nil {
		return 0, fmt.Errorf("failed to write entry: %w", err)
	}

	w.bytesWritten += uint64(w.currentSegment.size)
	w.entriesWritten++

	// Check if compaction is needed
	if w.config.CompactionPolicy == CompactOnSize && len(w.segments) > w.config.MaxSegments {
		go w.Compact()
	}

	return offset, nil
}

// Read reads entries from the WAL starting at the given offset
func (w *WAL) Read(startOffset uint64, limit int) ([]*WALEntry, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return nil, ErrWALClosed
	}

	entries := make([]*WALEntry, 0, limit)
	count := 0

	for _, seg := range w.segments {
		segEntries, err := seg.readEntries(startOffset, limit-count)
		if err != nil {
			return nil, fmt.Errorf("failed to read from segment %d: %w", seg.id, err)
		}

		entries = append(entries, segEntries...)
		count += len(segEntries)

		if count >= limit {
			break
		}
	}

	return entries, nil
}

// ReadAll reads all entries from the WAL
func (w *WAL) ReadAll() ([]*WALEntry, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return nil, ErrWALClosed
	}

	var allEntries []*WALEntry

	for _, seg := range w.segments {
		entries, err := seg.readAllEntries()
		if err != nil {
			return nil, fmt.Errorf("failed to read from segment %d: %w", seg.id, err)
		}
		allEntries = append(allEntries, entries...)
	}

	return allEntries, nil
}

// Sync flushes all pending writes to disk
func (w *WAL) Sync() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return ErrWALClosed
	}

	if w.currentSegment != nil {
		return w.currentSegment.sync()
	}

	return nil
}

// Compact removes old segments and consolidates data
func (w *WAL) Compact() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWALClosed
	}

	// Keep only the most recent segments
	if len(w.segments) <= w.config.MaxSegments {
		return nil
	}

	// Calculate how many segments to remove
	toRemove := len(w.segments) - w.config.MaxSegments

	// Close and remove old segments
	for i := 0; i < toRemove; i++ {
		seg := w.segments[i]
		if err := seg.close(); err != nil {
			return fmt.Errorf("failed to close segment %d: %w", seg.id, err)
		}
		if err := os.Remove(seg.path); err != nil {
			return fmt.Errorf("failed to remove segment %d: %w", seg.id, err)
		}
	}

	w.segments = w.segments[toRemove:]
	w.compactions++

	return nil
}

// Truncate removes all entries before the given offset
func (w *WAL) Truncate(offset uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWALClosed
	}

	// Find segments that can be removed
	var toRemove []*segment
	for i, seg := range w.segments {
		// Read first entry to check offset
		entries, err := seg.readEntries(0, 1)
		if err != nil || len(entries) == 0 {
			continue
		}

		if entries[0].Offset < offset {
			toRemove = append(toRemove, seg)
		} else {
			// Keep remaining segments
			w.segments = w.segments[i:]
			break
		}
	}

	// Remove old segments
	for _, seg := range toRemove {
		if err := seg.close(); err != nil {
			return fmt.Errorf("failed to close segment: %w", err)
		}
		if err := os.Remove(seg.path); err != nil {
			return fmt.Errorf("failed to remove segment: %w", err)
		}
	}

	return nil
}

// Close closes the WAL and all open segments
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWALClosed
	}

	w.closed = true
	close(w.closeCh)

	// Close all segments
	for _, seg := range w.segments {
		if err := seg.close(); err != nil {
			return err
		}
	}

	return nil
}

// Metrics returns WAL statistics
func (w *WAL) Metrics() WALMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return WALMetrics{
		BytesWritten:    w.bytesWritten,
		EntriesWritten:  w.entriesWritten,
		SegmentsCreated: w.segmentsCreated,
		SegmentsCurrent: uint64(len(w.segments)),
		Compactions:     w.compactions,
	}
}

// createSegment creates a new WAL segment
func (w *WAL) createSegment() error {
	w.lastSegmentID++
	segmentID := w.lastSegmentID

	filename := fmt.Sprintf("%s%08d%s", segmentPrefix, segmentID, segmentSuffix)
	path := filepath.Join(w.config.Dir, filename)

	seg, err := newSegment(segmentID, path, w.config.SegmentSize, false)
	if err != nil {
		return err
	}

	// Sync previous segment before switching
	if w.currentSegment != nil {
		if err := w.currentSegment.sync(); err != nil {
			return err
		}
		w.currentSegment.readOnly = true
	}

	w.currentSegment = seg
	w.segments = append(w.segments, seg)
	w.segmentsCreated++

	return nil
}

// loadSegments loads existing WAL segments from disk
func (w *WAL) loadSegments() error {
	entries, err := os.ReadDir(w.config.Dir)
	if err != nil {
		return err
	}

	var segmentFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), segmentPrefix) && strings.HasSuffix(entry.Name(), segmentSuffix) {
			segmentFiles = append(segmentFiles, entry.Name())
		}
	}

	// Sort by segment ID
	sort.Strings(segmentFiles)

	for _, filename := range segmentFiles {
		// Extract segment ID
		idStr := strings.TrimPrefix(filename, segmentPrefix)
		idStr = strings.TrimSuffix(idStr, segmentSuffix)
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil {
			continue
		}

		path := filepath.Join(w.config.Dir, filename)

		// Open existing segment as read-only except for the last one
		readOnly := true
		seg, err := newSegment(id, path, w.config.SegmentSize, readOnly)
		if err != nil {
			return err
		}

		w.segments = append(w.segments, seg)
		if id > w.lastSegmentID {
			w.lastSegmentID = id
		}
	}

	// Last segment should be writable
	if len(w.segments) > 0 {
		lastSeg := w.segments[len(w.segments)-1]
		lastSeg.readOnly = false
	}

	return nil
}

// syncLoop periodically syncs the WAL to disk
func (w *WAL) syncLoop() {
	ticker := time.NewTicker(w.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.Sync(); err != nil {
				// Log error but continue
			}
		case <-w.closeCh:
			return
		}
	}
}

// newSegment creates a new segment
func newSegment(id uint64, path string, maxSize int64, readOnly bool) (*segment, error) {
	var file *os.File
	var err error

	if readOnly {
		file, err = os.OpenFile(path, os.O_RDONLY, 0644)
	} else {
		file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open segment file: %w", err)
	}

	// Get current size
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat segment file: %w", err)
	}

	seg := &segment{
		id:       id,
		path:     path,
		file:     file,
		size:     stat.Size(),
		maxSize:  maxSize,
		readOnly: readOnly,
	}

	if !readOnly {
		seg.writer = bufio.NewWriter(file)
	}

	return seg, nil
}

// writeEntry writes an entry to the segment
func (s *segment) writeEntry(entry *WALEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly {
		return errors.New("cannot write to read-only segment")
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	// Write length prefix
	line := fmt.Sprintf("%s\n", string(data))
	n, err := s.writer.WriteString(line)
	if err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	s.size += int64(n)
	return nil
}

// readEntries reads entries from the segment
func (s *segment) readEntries(startOffset uint64, limit int) ([]*WALEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	entries := make([]*WALEntry, 0, limit)
	scanner := bufio.NewScanner(s.file)

	for scanner.Scan() && len(entries) < limit {
		var entry WALEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		if entry.Offset >= startOffset {
			entries = append(entries, &entry)
		}
	}

	return entries, scanner.Err()
}

// readAllEntries reads all entries from the segment
func (s *segment) readAllEntries() ([]*WALEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Seek to beginning
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var entries []*WALEntry
	scanner := bufio.NewScanner(s.file)

	for scanner.Scan() {
		var entry WALEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entries = append(entries, &entry)
	}

	return entries, scanner.Err()
}

// sync flushes buffered writes to disk
func (s *segment) sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.readOnly || s.writer == nil {
		return nil
	}

	if err := s.writer.Flush(); err != nil {
		return err
	}

	return s.file.Sync()
}

// close closes the segment file
func (s *segment) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return err
		}
	}

	return s.file.Close()
}

// WALMetrics holds WAL statistics
type WALMetrics struct {
	BytesWritten    uint64
	EntriesWritten  uint64
	SegmentsCreated uint64
	SegmentsCurrent uint64
	Compactions     uint64
}
