package wal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/therealutkarshpriyadarshi/log/pkg/types"
)

func TestNewWAL(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:          dir,
		SegmentSize:  1024,
		MaxSegments:  10,
		SyncInterval: 100 * time.Millisecond,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal.Close()

	if wal.currentSegment == nil {
		t.Errorf("current segment should not be nil")
	}

	if len(wal.segments) == 0 {
		t.Errorf("segments should not be empty")
	}
}

func TestWAL_WriteAndRead(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 1024,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal.Close()

	// Write an event
	event := &types.LogEvent{
		Timestamp: time.Now(),
		Message:   "test message",
		Source:    "test",
	}

	offset, err := wal.Write(event)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Sync to ensure write is persisted
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// Read the entry
	entries, err := wal.Read(offset, 1)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Event.Message != event.Message {
		t.Errorf("message = %s, want %s", entries[0].Event.Message, event.Message)
	}
}

func TestWAL_MultipleSegments(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 512, // Small size to force multiple segments
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal.Close()

	// Write multiple events to create multiple segments
	numEvents := 10
	for i := 0; i < numEvents; i++ {
		event := &types.LogEvent{
			Message: "test message with some content to increase size",
			Source:  "test",
		}
		if _, err := wal.Write(event); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	// Sync
	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// Check that multiple segments were created
	if len(wal.segments) <= 1 {
		t.Errorf("expected multiple segments, got %d", len(wal.segments))
	}

	// Read all entries
	entries, err := wal.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(entries) != numEvents {
		t.Errorf("expected %d entries, got %d", numEvents, len(entries))
	}
}

func TestWAL_Recovery(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 1024,
	}

	// Create WAL and write some events
	wal1, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		event := &types.LogEvent{
			Message: "test message",
			Source:  "test",
		}
		if _, err := wal1.Write(event); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if err := wal1.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	wal1.Close()

	// Recover from same directory
	wal2, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal2.Close()

	// Read all entries
	entries, err := wal2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("expected 5 recovered entries, got %d", len(entries))
	}
}

func TestWAL_Compact(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:          dir,
		SegmentSize:  512,
		MaxSegments:  3,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal.Close()

	// Write enough events to create multiple segments
	for i := 0; i < 20; i++ {
		event := &types.LogEvent{
			Message: "test message with content",
			Source:  "test",
		}
		if _, err := wal.Write(event); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	// Force compaction
	if err := wal.Compact(); err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	// Check segment count
	if len(wal.segments) > config.MaxSegments {
		t.Errorf("expected <= %d segments after compaction, got %d", config.MaxSegments, len(wal.segments))
	}

	metrics := wal.Metrics()
	if metrics.Compactions == 0 {
		t.Errorf("expected compaction count > 0")
	}
}

func TestWAL_Truncate(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 512,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal.Close()

	// Write some events
	var offsets []uint64
	for i := 0; i < 10; i++ {
		event := &types.LogEvent{
			Message: "test message",
			Source:  "test",
		}
		offset, err := wal.Write(event)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		offsets = append(offsets, offset)
	}

	if err := wal.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	initialSegments := len(wal.segments)

	// Truncate at offset 5
	if err := wal.Truncate(offsets[5]); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}

	// Segments should be reduced (depending on segment boundaries)
	if len(wal.segments) > initialSegments {
		t.Errorf("expected segments to be reduced after truncate")
	}
}

func TestWAL_Metrics(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 1024,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal.Close()

	// Write some events
	for i := 0; i < 5; i++ {
		event := &types.LogEvent{
			Message: "test message",
			Source:  "test",
		}
		if _, err := wal.Write(event); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	metrics := wal.Metrics()

	if metrics.EntriesWritten != 5 {
		t.Errorf("EntriesWritten = %d, want 5", metrics.EntriesWritten)
	}

	if metrics.SegmentsCreated == 0 {
		t.Errorf("SegmentsCreated should be > 0")
	}

	if metrics.SegmentsCurrent == 0 {
		t.Errorf("SegmentsCurrent should be > 0")
	}
}

func TestWAL_Close(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 1024,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}

	// Close the WAL
	if err := wal.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to write after close
	event := &types.LogEvent{Message: "test"}
	_, err = wal.Write(event)
	if err != ErrWALClosed {
		t.Errorf("expected ErrWALClosed, got %v", err)
	}

	// Try to close again
	err = wal.Close()
	if err != ErrWALClosed {
		t.Errorf("expected ErrWALClosed on second close, got %v", err)
	}
}

func TestWAL_Persistence(t *testing.T) {
	dir := t.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 1024,
	}

	// Write events
	wal1, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		event := &types.LogEvent{
			Message: "persistent message",
			Source:  "test",
		}
		if _, err := wal1.Write(event); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	if err := wal1.Sync(); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	wal1.Close()

	// Verify files exist
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	if len(files) == 0 {
		t.Errorf("expected WAL segment files to exist")
	}

	// Reopen and verify
	wal2, err := NewWAL(config)
	if err != nil {
		t.Fatalf("NewWAL() error = %v", err)
	}
	defer wal2.Close()

	entries, err := wal2.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestSegment_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-segment.log")

	seg, err := newSegment(1, path, 1024, false)
	if err != nil {
		t.Fatalf("newSegment() error = %v", err)
	}
	defer seg.close()

	// Write an entry
	entry := &WALEntry{
		Offset:    0,
		Timestamp: time.Now(),
		Event: &types.LogEvent{
			Message: "test",
			Source:  "test",
		},
	}

	if err := seg.writeEntry(entry); err != nil {
		t.Fatalf("writeEntry() error = %v", err)
	}

	if err := seg.sync(); err != nil {
		t.Fatalf("sync() error = %v", err)
	}

	// Read entries
	entries, err := seg.readAllEntries()
	if err != nil {
		t.Fatalf("readAllEntries() error = %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Event.Message != entry.Event.Message {
		t.Errorf("message = %s, want %s", entries[0].Event.Message, entry.Event.Message)
	}
}

func BenchmarkWAL_Write(b *testing.B) {
	dir := b.TempDir()

	config := WALConfig{
		Dir:          dir,
		SegmentSize:  64 * 1024 * 1024,
		SyncInterval: 10 * time.Second,
	}

	wal, _ := NewWAL(config)
	defer wal.Close()

	event := &types.LogEvent{
		Message: "benchmark message",
		Source:  "bench",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wal.Write(event)
	}
}

func BenchmarkWAL_Read(b *testing.B) {
	dir := b.TempDir()

	config := WALConfig{
		Dir:         dir,
		SegmentSize: 64 * 1024 * 1024,
	}

	wal, _ := NewWAL(config)
	defer wal.Close()

	// Pre-fill with events
	for i := 0; i < 1000; i++ {
		event := &types.LogEvent{
			Message: "benchmark message",
			Source:  "bench",
		}
		_, _ = wal.Write(event)
	}
	_ = wal.Sync()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wal.Read(0, 100)
	}
}
